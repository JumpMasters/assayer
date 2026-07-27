package capture

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/JumpMasters/assayer/internal/assay"
	"github.com/JumpMasters/assayer/internal/port"
)

// maxLine caps one transcript line. Lines carry whole tool results and run far
// past the scanner's default: the largest measured in a local store was 4.2 MiB,
// so this leaves roughly four times the headroom. A line past it is a source
// this adapter cannot read rather than a damaged one.
const maxLine = 16 * 1024 * 1024

// record is one line of a transcript, in the shape this adapter reads.
//
// Only the fields the neutral representation needs are declared. A transcript
// carries a great deal more, and anything not needed downstream stays here
// rather than travelling as an opaque payload.
type record struct {
	Type        string  `json:"type"`
	UUID        string  `json:"uuid"`
	SessionID   string  `json:"sessionId"`
	ResumedFrom string  `json:"session_id"`
	CWD         string  `json:"cwd"`
	GitBranch   string  `json:"gitBranch"`
	Version     string  `json:"version"`
	Timestamp   string  `json:"timestamp"`
	IsSidechain bool    `json:"isSidechain"`
	Message     message `json:"message"`

	// IsMeta and IsCompactSummary mark text the harness wrote into the
	// conversation itself: hook output, command results, caveat banners, and the
	// summary that replaces a prefix dropped to save context. All of it arrives
	// under the user's role and none of it was typed by a person.
	IsMeta           bool `json:"isMeta"`
	IsCompactSummary bool `json:"isCompactSummary"`

	// Subtype distinguishes kinds of system record. The boundary a compaction
	// leaves behind is the one that matters here, and it carries no message, so
	// it is visible only on the record.
	Subtype string `json:"subtype"`

	// ParentSessionID appears on the record that marks a fork. The field that
	// names a parent lives on its own record type rather than on every line.
	ParentSessionID string `json:"parentSessionId"`
}

type message struct {
	Role    string          `json:"role"`
	Model   string          `json:"model"`
	Usage   usage           `json:"usage"`
	Content json.RawMessage `json:"content"`
}

// usage is what one record cost.
//
// The two cache fields are read because they are where the input actually is.
// Over a local store of 730 project-root transcripts they carry 12,433,904,375
// tokens against 3,888,052 for input_tokens, so reading only the plain field
// reports about 0.03% of the input the harness billed for — and a change in
// caching behaviour, which is exactly the kind of drift an exam is
// re-administered to detect, is invisible in it.
type usage struct {
	InputTokens     int64 `json:"input_tokens"`
	CacheCreationIn int64 `json:"cache_creation_input_tokens"`
	CacheReadIn     int64 `json:"cache_read_input_tokens"`
	OutputTokens    int64 `json:"output_tokens"`
}

// input is every input token the record was billed for, cached or not.
func (u usage) input() int64 { return u.InputTokens + u.CacheCreationIn + u.CacheReadIn }

// block is one element of a structured message body.
type block struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`

	// Tool results arrive on a following user message rather than beside the
	// call, so they are matched back by identifier.
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

// contentTypes carry a session's substance and always declare a version.
//
// An attachment is admitted and contributes nothing: 23,908 of them in the
// project-root files of a local store, each carrying its payload on a top-level
// key this adapter does not read rather than under message, so it produces
// neither a turn nor any text. They are still content, and skipping them
// silently as bookkeeping would be the failure the list below exists to prevent.
//
// Two types are deliberately absent from both lists. started and result appear
// only in the workflow journals kept beside a session, which are a different
// file format rather than a transcript — pointing Load at one refuses the whole
// file with ErrUnsupported, which is the right answer for a source this adapter
// does not read.
func isContentType(t string) bool {
	switch t {
	case "assistant", "user", "attachment", "system":
		return true
	}
	return false
}

// bookkeepingTypes never declare a version and hold no conversation.
//
// They are listed rather than inferred from a missing version field. Roughly a
// third of all records carry no version, so treating absence as the signal
// would reject every real transcript; and inferring it the other way would let
// a genuinely new kind of content record be skipped in silence, producing a
// session that looks complete and is not.
func isBookkeepingType(t string) bool {
	switch t {
	case "queue-operation", "last-prompt", "ai-title", "mode", "pr-link",
		"file-history-snapshot", "bridge-session", "permission-mode",
		"file-history-delta", "worktree-state", "custom-title", "fork-context-ref":
		return true
	}
	return false
}

// parse reads a transcript into the neutral representation.
func parse(ctx context.Context, r io.Reader) (assay.Session, error) {
	var s assay.Session

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxLine)

	var (
		versions = map[string]bool{}
		version  string
		dirs     []string
		seenDir  = map[string]bool{}

		// A result arrives on a later record than the call it answers, so both
		// sides are collected and joined once the file has been read.
		//
		// Neither map is pruned: both are bounded by one file. A tool_use
		// identifier repeated across a resumed transcript overwrites its earlier
		// registration rather than being detected.
		sites   = map[string]callSite{}
		results = map[string]*assay.ToolResult{}

		delegated      []assay.Turn
		delegatedUsage assay.Usage
		compacted      bool
		sawContent     bool
	)

	// register indexes a record's calls where they landed. ids[i] identifies
	// Tools[i]: both come from one walk of the same record.
	register := func(turns *[]assay.Turn, ids []string) {
		for i, id := range ids {
			if id != "" {
				sites[id] = callSite{turns: turns, turn: len(*turns) - 1, tool: i}
			}
		}
	}

	for sc.Scan() {
		if err := ctx.Err(); err != nil {
			return assay.Session{}, err
		}
		// A byte order mark on the first line is an encoding preamble, not
		// damage, and refusing a whole transcript over three bytes would be a
		// failure this adapter manufactured.
		line := strings.TrimPrefix(strings.TrimSpace(sc.Text()), "\ufeff")
		if line == "" {
			continue
		}

		var rec record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return assay.Session{}, fmt.Errorf("%w: %w", port.ErrMalformed, err)
		}

		switch {
		case rec.Type == "":
			// A null line, or an object with no type, unmarshals into a zero
			// record. That is damage, and refusing it below as a record type
			// nobody has read yet would name the wrong cause.
			return assay.Session{}, fmt.Errorf("%w: a line carries no record type", port.ErrMalformed)
		case rec.Type == "fork-context-ref":
			// The one record that names a parent explicitly.
			if rec.ParentSessionID != "" {
				s.Lineage.Parent = rec.ParentSessionID
			}
			continue
		case isBookkeepingType(rec.Type):
			continue
		case !isContentType(rec.Type):
			// Refused rather than skipped: an unrecognised kind of content
			// record would otherwise vanish into a session that reads as whole.
			// The type is truncated because on a corrupted file it is unbounded
			// bytes from the transcript on their way to a terminal.
			return assay.Session{}, fmt.Errorf("%w: unknown record type %q",
				port.ErrUnsupported, truncate(rec.Type, 40))
		}

		sawContent = true
		if rec.Version != "" {
			versions[rec.Version] = true
			version = rec.Version
		}
		if rec.SessionID != "" && s.Lineage.ID == "" {
			s.Lineage.ID = rec.SessionID
		}
		// The second identifier names the session this one continues, and is
		// not a duplicate of the first. Preferring the wrong one attributes a
		// session's work to its predecessor.
		if rec.ResumedFrom != "" && rec.ResumedFrom != rec.SessionID && s.Lineage.Parent == "" {
			s.Lineage.Parent = rec.ResumedFrom
		}
		if rec.CWD != "" && !seenDir[rec.CWD] {
			seenDir[rec.CWD] = true
			dirs = append(dirs, rec.CWD)
		}
		if rec.GitBranch != "" && s.Workspace.Branch == "" {
			s.Workspace.Branch = rec.GitBranch
		}
		// Both markers are read. They arrive together in the ordinary case, and
		// the summary can be resumed into a session that never saw the boundary
		// that produced it.
		if rec.IsCompactSummary || rec.Subtype == "compact_boundary" {
			compacted = true
		}

		turn, ids := recordToTurn(&rec, results)

		// Counted whether or not the record became a turn: the harness billed
		// for it either way, and a record whose only block this adapter does not
		// map carried 114,000 tokens in the measured store.
		spent := &s.Usage
		if rec.IsSidechain {
			// Session usage is exclusive of delegated work, so a sub-agent's
			// tokens move to the delegation rather than being dropped.
			spent = &delegatedUsage
		}
		spent.InputTokens += rec.Message.Usage.input()
		spent.OutputTokens += rec.Message.Usage.OutputTokens

		if turn == nil {
			continue
		}

		if rec.IsSidechain {
			delegated = append(delegated, *turn)
			register(&delegated, ids)
			continue
		}

		s.Turns = append(s.Turns, *turn)
		register(&s.Turns, ids)
	}
	if err := sc.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			// This adapter's own limit. The source is intact and unreadable
			// here, which is a different verdict from damaged, and Assayer's
			// caps must never produce the one that reads as a regression.
			return assay.Session{}, fmt.Errorf("%w: %w", port.ErrUnsupported, err)
		}
		return assay.Session{}, fmt.Errorf("%w: %w", port.ErrMalformed, err)
	}
	if !sawContent {
		// Otherwise a truncated or zero-byte file loads as a session in which
		// nothing happened, claiming to know the complete order of that nothing.
		return assay.Session{}, fmt.Errorf("%w: no content records", port.ErrMalformed)
	}

	orphaned := false
	for id, res := range results {
		site, ok := sites[id]
		if !ok {
			// A result whose call the transcript does not hold. There is nothing
			// to attach it to, and its call going missing is a prefix going
			// missing — which is what the order claim below reports.
			orphaned = true
			continue
		}
		(*site.turns)[site.turn].Tools[site.tool].Result = res
	}

	s.Workspace.Dirs = dirs
	if len(delegated) > 0 || delegatedUsage != (assay.Usage{}) {
		// Delegated work is carried flat, as one delegation rather than one per
		// sub-agent: nothing in a transcript separates two runs, and nothing
		// links either back to the call that spawned it. So the order of the
		// whole session is not fully known and assertions about ordering must
		// say so rather than guess.
		delegatedUsage.Wall = wallOf(delegated)
		s.Delegated = []assay.Delegation{{Turns: delegated, Usage: delegatedUsage}}
	}
	// The other ways the order goes unknown: compaction drops a prefix of the
	// session, and what it dropped is gone from the transcript rather than
	// marked in it; and a result whose call is absent says the same thing from
	// the other end. Any of them makes an ordering assertion a question the
	// evidence cannot answer.
	s.OrderComplete = len(s.Delegated) == 0 && !compacted && !orphaned
	s.Usage.Wall = wallOf(s.Turns)

	// One release, not every release the session saw: a heartbeat check compares
	// the current harness version against this field, and an equality test
	// against a joined list never matches — so a bump would go unnoticed on
	// exactly the sessions that had already seen one. Verified below covers all
	// of them.
	s.Fidelity.Version = version
	s.Fidelity.Verified = allKnown(versions)
	return s, nil
}

// callSite locates a tool call by position in the slice holding it.
//
// Positions rather than a pointer into the call. A pointer survives the turn
// slice growing, because a turn's tools are their own slice and only the header
// is copied — but it is orphaned in silence the moment anything appends a call
// to a turn already stored, which merging streamed assistant messages would do.
// The result would then attach to a stale array with no error anywhere.
type callSite struct {
	turns *[]assay.Turn
	turn  int
	tool  int
}

// wallOf measures from the earliest timestamp seen to the latest, over turns
// that carry one.
//
// Not last minus first. 445 of 728 project-root transcripts in a local store
// record content timestamps out of order, which made this negative on two of
// them; and a missing or unparseable timestamp reads as the zero time, which
// saturated it to time.Duration's maximum of 292 years. It measures the span
// between the turns observed, which is neither the session's duration nor the
// time the agent worked.
func wallOf(turns []assay.Turn) time.Duration {
	var lo, hi time.Time
	for i := range turns {
		at := turns[i].At
		if at.IsZero() {
			continue
		}
		if lo.IsZero() || at.Before(lo) {
			lo = at
		}
		if at.After(hi) {
			hi = at
		}
	}
	if lo.IsZero() {
		return 0
	}
	return hi.Sub(lo)
}

// truncate bounds a value taken from a transcript before it is formatted into
// an error. Splitting a rune is harmless: %q escapes what it produces.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// recordToTurn translates one content record, returning the identifiers of the
// calls it makes in the order Turn.Tools holds them, and collecting any tool
// results it carries into results so they can be joined to the calls they
// answer.
//
// One walk of the body produces both. Walking it twice meant a second unmarshal
// of a line that reaches 4.2 MiB, and left two traversals that agreed only by
// convention.
func recordToTurn(rec *record, results map[string]*assay.ToolResult) (turn *assay.Turn, ids []string) {
	// The harness's own text is kept, because the agent read it and a rubric
	// grading what the agent was working from needs it, and its author is left
	// unclaimed, because no person wrote it. Attributing it to one inflated the
	// human turns of the measured store by 48.3%.
	role := assay.RoleUnknown
	switch {
	case rec.IsMeta || rec.IsCompactSummary:
	case rec.Message.Role == "user":
		role = assay.RoleHuman
	case rec.Message.Role == "assistant":
		role = assay.RoleAgent
	}

	t := assay.Turn{
		Role:  role,
		Model: modelOf(rec),
		At:    parseTime(rec.Timestamp),
	}

	blocks := blocksOf(rec.Message.Content)
	for i := range blocks {
		b := &blocks[i]
		switch b.Type {
		case "text":
			t.Text += b.Text
		case "thinking":
			t.Reasoning += b.Text
		case "tool_use":
			t.Tools = append(t.Tools, assay.ToolCall{
				Name:  b.Name,
				Kind:  classify(b.Name),
				Argv:  argvOf(b),
				Input: string(b.Input),
			})
			ids = append(ids, b.ID)
		case "tool_result":
			results[b.ToolUseID] = toolResult(b)
		case "fallback":
			// The harness substituting one model for another mid-session, naming
			// what was asked for and what actually served. Six in the measured
			// store, each on a record carrying no other block — so it becomes no
			// turn, and there is nowhere in the neutral representation to put an
			// event that belongs to no turn. Not carried, and named here because
			// it is the one place a substitution is visible from a transcript.
		}
	}

	// A record carrying only tool results is not a turn in the conversation.
	// The protocol returns results on a message with the user's role, so
	// admitting it would insert an empty human turn between an agent's call and
	// its answer — inflating any count of turns, and putting a human turn where
	// the person said nothing.
	if t.Text == "" && t.Reasoning == "" && len(t.Tools) == 0 {
		return nil, nil
	}
	return &t, ids
}

func toolResult(b *block) *assay.ToolResult {
	outcome := assay.OutcomeOK
	if b.IsError {
		outcome = assay.OutcomeNonZero
	}
	return &assay.ToolResult{
		Text:    textOf(b.Content),
		Outcome: outcome,
		// No exit code: transcripts record that a call errored and never the
		// code it returned. Filling this with zero would report success beside
		// an outcome saying otherwise.
		ExitCode: nil,
	}
}

// classify maps a harness's tool name onto the neutral vocabulary, so that no
// package outside an adapter has to match on a name.
func classify(name string) assay.ToolKind {
	switch name {
	case "Read", "Glob", "Grep", "NotebookRead", "WebFetch", "WebSearch":
		return assay.ToolRead
	case "Edit", "Write", "NotebookEdit", "MultiEdit":
		return assay.ToolMutate
	case "Bash", "BashOutput", "KillShell":
		return assay.ToolExec
	case "Task", "Agent":
		return assay.ToolDelegate
	case "":
		return assay.ToolUnknown
	}
	return assay.ToolOther
}

// argvOf extracts a command line when the call ran one.
func argvOf(b *block) []string {
	if classify(b.Name) != assay.ToolExec || len(b.Input) == 0 {
		return nil
	}
	var in struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(b.Input, &in); err != nil || in.Command == "" {
		return nil
	}
	// The harness records a command as one string. It is kept whole in a
	// single element rather than split here: splitting a shell command
	// correctly needs a shell, and guessing produces an argv that never ran.
	return []string{in.Command}
}

func modelOf(rec *record) assay.Model {
	if rec.Message.Model == "" {
		return assay.Model{}
	}
	// The transcript records what served the message, never the alias that was
	// asked for, so a quiet remapping of an alias cannot be seen from here.
	//
	// No provider. No record in a transcript names one: the harness routes to
	// two other backends under environment variables and writes the same
	// transcript either way, so a constant here would assert a backend that was
	// never observed — and the backend changing is one of the conditions an exam
	// is re-administered for.
	return assay.Model{Canonical: rec.Message.Model}
}

// blocksOf reads a message body, which is either a plain string or a list of
// structured blocks depending on the record.
func blocksOf(raw json.RawMessage) []block {
	if len(raw) == 0 {
		return nil
	}
	var blocks []block
	if err := json.Unmarshal(raw, &blocks); err == nil {
		return blocks
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil && text != "" {
		return []block{{Type: "text", Text: text}}
	}
	return nil
}

// textOf renders a result body, which has the same two shapes as a message.
func textOf(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var blocks []block
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var b strings.Builder
		for i := range blocks {
			b.WriteString(blocks[i].Text)
		}
		return b.String()
	}
	return ""
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func allKnown(versions map[string]bool) bool {
	if len(versions) == 0 {
		return false
	}
	for v := range versions {
		if !isKnownVersion(v) {
			return false
		}
	}
	return true
}
