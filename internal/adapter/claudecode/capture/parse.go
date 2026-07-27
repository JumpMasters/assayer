package capture

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/JumpMasters/assayer/internal/assay"
	"github.com/JumpMasters/assayer/internal/port"
)

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

type usage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

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
	// Transcript lines carry whole tool results and can be far longer than the
	// scanner's default limit.
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	var (
		versions  = map[string]bool{}
		dirs      []string
		seenDir   = map[string]bool{}
		pending   = map[string]*pendingCall{}
		delegated []assay.Turn
		compacted bool
	)

	for sc.Scan() {
		if err := ctx.Err(); err != nil {
			return assay.Session{}, err
		}
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}

		var rec record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return assay.Session{}, fmt.Errorf("%w: %w", port.ErrMalformed, err)
		}

		switch {
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
			return assay.Session{}, fmt.Errorf("%w: unknown record type %q",
				port.ErrUnsupported, rec.Type)
		}

		if rec.Version != "" {
			versions[rec.Version] = true
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

		turn, results := recordToTurn(&rec)

		// A result arrives on a later message than the call it answers.
		for id, res := range results {
			if p, ok := pending[id]; ok {
				p.call.Result = res
			}
		}

		if turn == nil {
			continue
		}

		if rec.IsSidechain {
			delegated = append(delegated, *turn)
			continue
		}

		s.Turns = append(s.Turns, *turn)
		last := &s.Turns[len(s.Turns)-1]
		// Index each call so a result arriving on a later message can be
		// attached to it. The identifiers are gathered once: recomputing them
		// per call would re-parse the whole message body each time.
		ids := toolIDs(&rec)
		for i := range last.Tools {
			if i < len(ids) && ids[i] != "" {
				pending[ids[i]] = &pendingCall{call: &last.Tools[i]}
			}
		}

		s.Usage.InputTokens += rec.Message.Usage.InputTokens
		s.Usage.OutputTokens += rec.Message.Usage.OutputTokens
	}
	if err := sc.Err(); err != nil {
		return assay.Session{}, fmt.Errorf("%w: %w", port.ErrMalformed, err)
	}

	s.Workspace.Dirs = dirs
	if len(delegated) > 0 {
		// Delegated work is carried flat. Nothing here links it back to the
		// call that spawned it, so the order of the whole session is not fully
		// known and assertions about ordering must say so rather than guess.
		s.Delegated = []assay.Delegation{{Turns: delegated}}
	}
	// The other way the order goes unknown: compaction drops a prefix of the
	// session, and what it dropped is gone from the transcript rather than
	// marked in it. Either condition makes an ordering assertion a question the
	// evidence cannot answer.
	s.OrderComplete = len(delegated) == 0 && !compacted

	if len(s.Turns) > 0 {
		s.Usage.Wall = s.Turns[len(s.Turns)-1].At.Sub(s.Turns[0].At)
	}

	s.Fidelity.Version = strings.Join(sortedKeys(versions), ",")
	s.Fidelity.Verified = allKnown(versions)
	return s, nil
}

type pendingCall struct{ call *assay.ToolCall }

// recordToTurn translates one content record, returning any tool results it
// carries so they can be matched to the calls they answer.
func recordToTurn(rec *record) (turn *assay.Turn, results map[string]*assay.ToolResult) {
	results = map[string]*assay.ToolResult{}

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
		case "tool_result":
			results[b.ToolUseID] = toolResult(b)
		}
	}

	// A record carrying only tool results is not a turn in the conversation.
	// The protocol returns results on a message with the user's role, so
	// admitting it would insert an empty human turn between an agent's call and
	// its answer — inflating any count of turns, and putting a human turn where
	// the person said nothing.
	if t.Text == "" && t.Reasoning == "" && len(t.Tools) == 0 {
		return nil, results
	}
	return &t, results
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
	return assay.Model{Canonical: rec.Message.Model, Provider: "anthropic"}
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

// toolIDs lists the identifiers of the calls a record makes, in order.
func toolIDs(rec *record) []string {
	blocks := blocksOf(rec.Message.Content)
	ids := make([]string, 0, len(blocks))
	for i := range blocks {
		if blocks[i].Type == "tool_use" {
			ids = append(ids, blocks[i].ID)
		}
	}
	return ids
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

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
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
