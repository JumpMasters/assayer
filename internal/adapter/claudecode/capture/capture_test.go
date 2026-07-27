package capture

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JumpMasters/assayer/internal/adapter/conformance"
	"github.com/JumpMasters/assayer/internal/assay"
	"github.com/JumpMasters/assayer/internal/port"
)

// Every fixture here is synthetic.
//
// No real transcript is committed, and none should be: the store holds working
// directories, file contents, commands and whatever a session happened to type,
// and the redaction pass that would make an export safe is not trustworthy yet
// — a survey found it firing on one local file in two. These fixtures reproduce
// the structure the survey measured without carrying anything from it.

const transcriptWithATool = `
{"type":"user","version":"2.1.220","sessionId":"s1","cwd":"/work","gitBranch":"main","timestamp":"2026-01-01T00:00:00Z","message":{"role":"user","content":"add a test"}}
{"type":"assistant","version":"2.1.220","sessionId":"s1","cwd":"/work","timestamp":"2026-01-01T00:00:10Z","message":{"role":"assistant","model":"claude-x-1","usage":{"input_tokens":100,"output_tokens":50},"content":[{"type":"thinking","text":"consider the options"},{"type":"text","text":"running the tests"},{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"go test ./..."}}]}}
{"type":"user","version":"2.1.220","sessionId":"s1","cwd":"/work","timestamp":"2026-01-01T00:00:20Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok","is_error":false}]}}
{"type":"queue-operation","note":"bookkeeping, no version, must be skipped"}
{"type":"assistant","version":"2.1.220","sessionId":"s1","cwd":"/other","timestamp":"2026-01-01T00:01:00Z","message":{"role":"assistant","model":"claude-x-2","usage":{"input_tokens":10,"output_tokens":5},"content":[{"type":"text","text":"done"}]}}
`

func writeStore(t *testing.T, name, body string) string {
	t.Helper()
	return writeStoreIn(t, "-work-project", name, body)
}

func writeStoreIn(t *testing.T, project, name, body string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(strings.TrimSpace(body)+"\n"), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	return root
}

func loadOnly(t *testing.T, body string) assay.Session {
	t.Helper()
	a := Adapter{Root: writeStore(t, "session.jsonl", body)}
	refs, err := a.Discover(context.Background(), port.Query{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("Discover returned %d refs, want 1", len(refs))
	}
	s, err := a.Load(context.Background(), refs[0])
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return s
}

// TestSatisfiesTheSharedContract runs the kit every capture adapter must pass.
func TestSatisfiesTheSharedContract(t *testing.T) {
	conformance.Verify(t, Adapter{Root: writeStore(t, "session.jsonl", transcriptWithATool)})
}

func TestTranslatesASession(t *testing.T) {
	s := loadOnly(t, transcriptWithATool)

	t.Run("turns keep their order and roles", func(t *testing.T) {
		if len(s.Turns) != 3 {
			t.Fatalf("got %d turns, want 3", len(s.Turns))
		}
		if s.Turns[0].Role != assay.RoleHuman {
			t.Errorf("first turn role = %v, want human", s.Turns[0].Role)
		}
		if s.Turns[0].Text != "add a test" {
			t.Errorf("first turn text = %q", s.Turns[0].Text)
		}
		if s.Turns[1].Role != assay.RoleAgent {
			t.Errorf("second turn role = %v, want agent", s.Turns[1].Role)
		}
	})

	t.Run("reasoning is kept apart from prose", func(t *testing.T) {
		if s.Turns[1].Reasoning != "consider the options" {
			t.Errorf("reasoning = %q", s.Turns[1].Reasoning)
		}
		if strings.Contains(s.Turns[1].Text, "consider the options") {
			t.Error("reasoning leaked into the turn's text; a rubric would grade a scratchpad")
		}
	})

	t.Run("a tool call carries its command and neutral kind", func(t *testing.T) {
		if len(s.Turns[1].Tools) != 1 {
			t.Fatalf("got %d tool calls, want 1", len(s.Turns[1].Tools))
		}
		c := s.Turns[1].Tools[0]
		if c.Kind != assay.ToolExec {
			t.Errorf("kind = %v, want exec", c.Kind)
		}
		if len(c.Argv) != 1 || c.Argv[0] != "go test ./..." {
			t.Errorf("argv = %v", c.Argv)
		}
		if c.Input == "" {
			t.Error("tool input was dropped; distillation needs what was put to the tool")
		}
	})

	t.Run("a result is matched to the call it answers", func(t *testing.T) {
		c := s.Turns[1].Tools[0]
		if c.Result == nil {
			t.Fatal("result is nil; it arrives on a later record and must be paired back")
		}
		if c.Result.Text != "ok" {
			t.Errorf("result text = %q", c.Result.Text)
		}
		if c.Result.Outcome != assay.OutcomeOK {
			t.Errorf("outcome = %v, want ok", c.Result.Outcome)
		}
		if c.Result.ExitCode != nil {
			t.Error("an exit code was reported; transcripts do not carry one, " +
				"and a zero here would read as success beside a failing outcome")
		}
	})

	t.Run("every working directory is kept", func(t *testing.T) {
		if len(s.Workspace.Dirs) != 2 {
			t.Fatalf("dirs = %v, want both", s.Workspace.Dirs)
		}
		if s.Workspace.Dirs[0] != "/work" || s.Workspace.Dirs[1] != "/other" {
			t.Errorf("dirs = %v, want first-use order", s.Workspace.Dirs)
		}
	})

	t.Run("the model is per turn", func(t *testing.T) {
		models := s.Models()
		if len(models) != 2 {
			t.Fatalf("Models() = %v, want two distinct", models)
		}
		if models[0].Canonical != "claude-x-1" || models[1].Canonical != "claude-x-2" {
			t.Errorf("Models() = %v", models)
		}
		if models[0].Alias != "" {
			t.Error("an alias was reported; a transcript records only what served the message")
		}
	})

	t.Run("tokens are summed and money is not invented", func(t *testing.T) {
		if s.Usage.InputTokens != 110 || s.Usage.OutputTokens != 55 {
			t.Errorf("usage = %+v", s.Usage)
		}
		if s.Usage.CostMicroUSD != 0 {
			t.Error("a price was reported; no transcript record carries one")
		}
	})

	t.Run("fidelity names the adapter and the releases read", func(t *testing.T) {
		if s.Fidelity.Adapter != "claude-code" || s.Fidelity.Tier != assay.TierNative {
			t.Errorf("fidelity = %+v", s.Fidelity)
		}
		if s.Fidelity.Version != "2.1.220" {
			t.Errorf("version = %q", s.Fidelity.Version)
		}
		if !s.Fidelity.Verified {
			t.Error("a release this adapter has been read against was marked unverified")
		}
	})
}

// TestLineageKeepsTheParentSeparate covers the misattribution a single
// identifier would cause: the second field names the session this one
// continues, not a duplicate of the first.
func TestLineageKeepsTheParentSeparate(t *testing.T) {
	s := loadOnly(t, `
{"type":"user","version":"2.1.220","sessionId":"child","session_id":"parent","cwd":"/w","timestamp":"2026-01-01T00:00:00Z","message":{"role":"user","content":"go"}}
`)
	if s.Lineage.ID != "child" {
		t.Errorf("lineage id = %q, want the session's own", s.Lineage.ID)
	}
	if s.Lineage.Parent != "parent" {
		t.Errorf("lineage parent = %q, want the session it continues", s.Lineage.Parent)
	}
}

// TestForkRecordNamesTheParent covers the record type an earlier survey
// classified as bookkeeping and skipped, which is where a fork's parent is
// actually written down.
func TestForkRecordNamesTheParent(t *testing.T) {
	s := loadOnly(t, `
{"type":"fork-context-ref","parentSessionId":"ancestor","parentLastUuid":"u1"}
{"type":"user","version":"2.1.220","sessionId":"forked","cwd":"/w","timestamp":"2026-01-01T00:00:00Z","message":{"role":"user","content":"go"}}
`)
	if s.Lineage.Parent != "ancestor" {
		t.Errorf("lineage parent = %q, want the fork record's parent", s.Lineage.Parent)
	}
}

// TestDelegatedWorkIsCarriedAndOrderIsNotClaimed covers work handed to a
// sub-agent: it is kept rather than dropped, and because nothing links it back
// to the call that spawned it, the session says its order is incomplete.
func TestDelegatedWorkIsCarriedAndOrderIsNotClaimed(t *testing.T) {
	s := loadOnly(t, `
{"type":"user","version":"2.1.220","sessionId":"s","cwd":"/w","timestamp":"2026-01-01T00:00:00Z","message":{"role":"user","content":"go"}}
{"type":"assistant","version":"2.1.220","sessionId":"s","cwd":"/w","isSidechain":true,"timestamp":"2026-01-01T00:00:05Z","message":{"role":"assistant","model":"m","content":[{"type":"text","text":"sub-task"}]}}
`)
	if len(s.Delegated) != 1 || len(s.Delegated[0].Turns) != 1 {
		t.Fatalf("delegated = %+v, want one delegation with one turn", s.Delegated)
	}
	if s.OrderComplete {
		t.Error("order was claimed complete while delegated work is unattributed")
	}
	for _, turn := range s.Turns {
		if turn.Text == "sub-task" {
			t.Error("delegated work was folded into the main turns")
		}
	}
}

// TestInjectedRecordsAreNotAttributedToAPerson guards the distinction between
// what a person asked for and what the harness wrote into the conversation on
// its own — hook output, command results, caveat banners, a compaction summary.
// All of it arrives under the user's role. Counted as human turns it was 48.3%
// of them on the store this adapter was measured against, and a distiller
// reading the session to work out what was asked would find a third of the
// asking was the harness talking to itself.
func TestInjectedRecordsAreNotAttributedToAPerson(t *testing.T) {
	s := loadOnly(t, `
{"type":"user","version":"2.1.220","sessionId":"s","cwd":"/w","timestamp":"2026-01-01T00:00:00Z","message":{"role":"user","content":"add a test"}}
{"type":"user","version":"2.1.220","sessionId":"s","cwd":"/w","isMeta":true,"timestamp":"2026-01-01T00:00:01Z","message":{"role":"user","content":"Caveat: the messages below were generated while running a local command."}}
{"type":"user","version":"2.1.220","sessionId":"s","cwd":"/w","isCompactSummary":true,"timestamp":"2026-01-01T00:00:02Z","message":{"role":"user","content":"This session is being continued from a previous conversation."}}
`)
	if len(s.Turns) != 3 {
		t.Fatalf("got %d turns, want 3; injected text is kept as evidence, only its author is not claimed", len(s.Turns))
	}
	if s.Turns[0].Role != assay.RoleHuman {
		t.Errorf("a genuine turn lost its role: got %v", s.Turns[0].Role)
	}
	for _, turn := range s.Turns[1:] {
		if turn.Role == assay.RoleHuman {
			t.Errorf("the harness's own text was attributed to a person: %q", turn.Text)
		}
	}
}

// TestCompactedSessionDoesNotClaimACompleteOrder covers the second clause of
// OrderComplete, which went unimplemented: a session whose earlier part was
// compacted away no longer knows the order of everything it did. Claiming
// otherwise turns missing evidence into a failure — the manufactured regression
// the flag exists to prevent.
func TestCompactedSessionDoesNotClaimACompleteOrder(t *testing.T) {
	t.Run("the summary that replaces a compacted prefix", func(t *testing.T) {
		s := loadOnly(t, `
{"type":"user","version":"2.1.220","sessionId":"s","cwd":"/w","isCompactSummary":true,"timestamp":"2026-01-01T00:00:00Z","message":{"role":"user","content":"This session is being continued from a previous conversation."}}
{"type":"user","version":"2.1.220","sessionId":"s","cwd":"/w","timestamp":"2026-01-01T00:00:01Z","message":{"role":"user","content":"carry on"}}
`)
		if s.OrderComplete {
			t.Error("order was claimed complete on a session that had been compacted")
		}
	})

	// The boundary marker carries no message and so becomes no turn. Reading the
	// flag off the turns alone would miss it.
	t.Run("the boundary marker on its own", func(t *testing.T) {
		s := loadOnly(t, `
{"type":"system","version":"2.1.220","sessionId":"s","cwd":"/w","subtype":"compact_boundary","timestamp":"2026-01-01T00:00:00Z","content":"Conversation compacted"}
{"type":"user","version":"2.1.220","sessionId":"s","cwd":"/w","timestamp":"2026-01-01T00:00:01Z","message":{"role":"user","content":"carry on"}}
`)
		if s.OrderComplete {
			t.Error("order was claimed complete across a compaction boundary")
		}
	})

	t.Run("an uncompacted session still claims it", func(t *testing.T) {
		if s := loadOnly(t, transcriptWithATool); !s.OrderComplete {
			t.Error("order was withheld from a session that was never compacted")
		}
	})
}

// TestUnknownReleaseIsReadButNotVerified covers the choice not to refuse a
// release nobody has checked. Refusing would break capture roughly twice a week
// at the rate releases were measured arriving.
func TestUnknownReleaseIsReadButNotVerified(t *testing.T) {
	s := loadOnly(t, `
{"type":"user","version":"9.9.999","sessionId":"s","cwd":"/w","timestamp":"2026-01-01T00:00:00Z","message":{"role":"user","content":"go"}}
`)
	if len(s.Turns) != 1 {
		t.Fatalf("an unknown release was refused; got %d turns", len(s.Turns))
	}
	if s.Fidelity.Verified {
		t.Error("a release nobody has read against was reported as verified")
	}
}

// TestUnknownRecordTypeIsRefused is the other half of that choice: a new kind
// of content record must fail loudly rather than be skipped into a session that
// looks whole and is not.
func TestUnknownRecordTypeIsRefused(t *testing.T) {
	a := Adapter{Root: writeStore(t, "session.jsonl", `
{"type":"user","version":"2.1.220","sessionId":"s","cwd":"/w","timestamp":"2026-01-01T00:00:00Z","message":{"role":"user","content":"go"}}
{"type":"a-record-type-nobody-has-seen","version":"2.1.220"}
`)}
	refs, _ := a.Discover(context.Background(), port.Query{})
	_, err := a.Load(context.Background(), refs[0])
	if !errors.Is(err, port.ErrUnsupported) {
		t.Errorf("Load returned %v, want ErrUnsupported", err)
	}
}

func TestMalformedTranscriptIsReported(t *testing.T) {
	a := Adapter{Root: writeStore(t, "session.jsonl", `{"type":"user"`+"\n")}
	refs, _ := a.Discover(context.Background(), port.Query{})
	_, err := a.Load(context.Background(), refs[0])
	if !errors.Is(err, port.ErrMalformed) {
		t.Errorf("Load returned %v, want ErrMalformed", err)
	}
}

func TestMissingSessionReportsNotFound(t *testing.T) {
	a := Adapter{Root: t.TempDir()}
	_, err := a.Load(context.Background(), port.Ref{ID: filepath.Join(t.TempDir(), "gone.jsonl")})
	if !errors.Is(err, port.ErrNotFound) {
		t.Errorf("Load of a pruned session returned %v, want ErrNotFound", err)
	}
	if _, err := a.Load(context.Background(), port.Ref{}); !errors.Is(err, port.ErrNotFound) {
		t.Errorf("Load of an empty ref returned %v, want ErrNotFound", err)
	}
}

func TestDiscoverOnAnAbsentStoreIsEmptyNotAnError(t *testing.T) {
	a := Adapter{Root: filepath.Join(t.TempDir(), "no-such-store")}
	refs, err := a.Discover(context.Background(), port.Query{})
	if err != nil {
		t.Fatalf("Discover on an absent store returned %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("got %d refs from an absent store", len(refs))
	}
}

func TestDiscoverHonoursTheQuery(t *testing.T) {
	a := Adapter{Root: writeStore(t, "session.jsonl", transcriptWithATool)}
	ctx := context.Background()

	if refs, _ := a.Discover(ctx, port.Query{Since: time.Now().Add(time.Hour)}); len(refs) != 0 {
		t.Errorf("Since in the future returned %d refs", len(refs))
	}
	if refs, _ := a.Discover(ctx, port.Query{Dir: "/nowhere/at/all"}); len(refs) != 0 {
		t.Errorf("a non-matching Dir returned %d refs", len(refs))
	}
	if refs, _ := a.Discover(ctx, port.Query{Limit: 1}); len(refs) != 1 {
		t.Errorf("Limit 1 returned %d refs", len(refs))
	}
}

// TestToolNamesMapToNeutralKinds is what lets core reason about behaviour
// without matching on the names one harness happens to use.
func TestToolNamesMapToNeutralKinds(t *testing.T) {
	for name, want := range map[string]assay.ToolKind{
		"Read":    assay.ToolRead,
		"Grep":    assay.ToolRead,
		"Edit":    assay.ToolMutate,
		"Write":   assay.ToolMutate,
		"Bash":    assay.ToolExec,
		"Task":    assay.ToolDelegate,
		"Unheard": assay.ToolOther,
		"":        assay.ToolUnknown,
	} {
		if got := classify(name); got != want {
			t.Errorf("classify(%q) = %v, want %v", name, got, want)
		}
	}
}

// TestCapabilitiesAreNotOverclaimed pins the four this adapter must not assert.
// Each was checked against the store rather than assumed.
func TestCapabilitiesAreNotOverclaimed(t *testing.T) {
	caps := Adapter{}.Capabilities()
	for _, c := range []assay.Capability{
		assay.CanSeeCost,
		assay.CanSeeWorkspacePatch,
		assay.CanSeeFileMutations,
		assay.CanSeeMutationContent,
	} {
		if caps.Has(c) {
			t.Errorf("adapter declares %v, which no transcript record supplies", c)
		}
	}

	partial := partialCapabilities()
	if !partial.Has(assay.CanSeeModelIdentity) {
		t.Error("model identity is not marked partial; the alias is never recorded")
	}
	if !partial.Has(assay.CanSeeToolResults) {
		t.Error("tool results are not marked partial; large ones are shortened in place")
	}
	if !partial.Has(assay.CanSeeDelegation) {
		t.Error("delegation is not marked partial; a scan of 601 project-root transcripts " +
			"found none recorded inline, so on the real store the answer is always empty")
	}
	if !caps.Contains(partial) {
		t.Error("a capability is marked partial that the adapter does not claim at all")
	}
	if !partial.Has(assay.CanSeeWorkspace) {
		t.Error("the workspace is not marked partial; the revision and the origin are " +
			"governed by it and are never filled, so an assertion comparing a replay's " +
			"revision would compare a recorded SHA against an empty string and fail")
	}
}

// TestProviderIsNotInvented covers a field that was filled with a constant no
// record states. The harness routes to two other backends under environment
// variables and writes the same transcript either way; the backend changing is
// one of the conditions an exam is re-administered for, and a hard-coded value
// asserts it never did.
func TestProviderIsNotInvented(t *testing.T) {
	s := loadOnly(t, `
{"type":"assistant","version":"2.1.220","sessionId":"s","cwd":"/w","timestamp":"2026-01-01T00:00:00Z","message":{"role":"assistant","model":"anthropic.claude-x-1-v1:0","content":[{"type":"text","text":"hello"}]}}
`)
	if got := s.Turns[0].Model.Provider; got != "" {
		t.Errorf("provider = %q, want empty; no transcript record names one", got)
	}
	if s.Turns[0].Model.Canonical != "anthropic.claude-x-1-v1:0" {
		t.Errorf("canonical = %q", s.Turns[0].Model.Canonical)
	}
}

// TestTokensCountEveryInputTheHarnessBilledFor covers the two cache fields that
// went unread. On the measured store they carry 12.4 billion tokens against 3.9
// million for the field that was read, so the figure an assertion ran against
// was three orders of magnitude too small.
func TestTokensCountEveryInputTheHarnessBilledFor(t *testing.T) {
	s := loadOnly(t, `
{"type":"assistant","version":"2.1.220","sessionId":"s","cwd":"/w","timestamp":"2026-01-01T00:00:00Z","message":{"role":"assistant","model":"m","usage":{"input_tokens":7,"cache_creation_input_tokens":300,"cache_read_input_tokens":5000,"output_tokens":11},"content":[{"type":"text","text":"hi"}]}}
`)
	if s.Usage.InputTokens != 5307 {
		t.Errorf("input tokens = %d, want 5307: every input token billed, cached or not", s.Usage.InputTokens)
	}
	if s.Usage.OutputTokens != 11 {
		t.Errorf("output tokens = %d", s.Usage.OutputTokens)
	}
}

// TestUsageIsKeptFromRecordsThatAreNotTurns covers a record whose only block is
// one this adapter does not map. It becomes no turn, and its tokens were
// therefore dropped — a fallback record in the measured store carried 114,000
// of them.
func TestUsageIsKeptFromRecordsThatAreNotTurns(t *testing.T) {
	s := loadOnly(t, `
{"type":"user","version":"2.1.220","sessionId":"s","cwd":"/w","timestamp":"2026-01-01T00:00:00Z","message":{"role":"user","content":"go"}}
{"type":"assistant","version":"2.1.220","sessionId":"s","cwd":"/w","timestamp":"2026-01-01T00:00:01Z","message":{"role":"assistant","model":"m","usage":{"input_tokens":2,"cache_read_input_tokens":110821,"output_tokens":0},"content":[{"type":"fallback","from":{"model":"a"},"to":{"model":"b"}}]}}
`)
	if len(s.Turns) != 1 {
		t.Fatalf("got %d turns, want 1; a record with no mapped block is not a turn", len(s.Turns))
	}
	if s.Usage.InputTokens != 110823 {
		t.Errorf("input tokens = %d, want 110823; the harness billed for the record either way", s.Usage.InputTokens)
	}
}

// TestWallIsMeasuredAcrossTheTimestampsThatExist covers a duration computed as
// last minus first with no guard on either end: out-of-order records made it
// negative on two transcripts in the measured store, and a missing first
// timestamp saturated it to 292 years.
func TestWallIsMeasuredAcrossTheTimestampsThatExist(t *testing.T) {
	t.Run("records out of order", func(t *testing.T) {
		s := loadOnly(t, `
{"type":"user","version":"2.1.220","sessionId":"s","cwd":"/w","timestamp":"2026-01-01T00:01:00Z","message":{"role":"user","content":"go"}}
{"type":"assistant","version":"2.1.220","sessionId":"s","cwd":"/w","timestamp":"2026-01-01T00:00:00Z","message":{"role":"assistant","model":"m","content":[{"type":"text","text":"done"}]}}
`)
		if s.Usage.Wall != time.Minute {
			t.Errorf("wall = %v, want 1m across the timestamps present", s.Usage.Wall)
		}
	})

	t.Run("a missing first timestamp", func(t *testing.T) {
		s := loadOnly(t, `
{"type":"user","version":"2.1.220","sessionId":"s","cwd":"/w","message":{"role":"user","content":"go"}}
{"type":"assistant","version":"2.1.220","sessionId":"s","cwd":"/w","timestamp":"2026-01-01T00:00:30Z","message":{"role":"assistant","model":"m","content":[{"type":"text","text":"done"}]}}
`)
		if s.Usage.Wall != 0 {
			t.Errorf("wall = %v, want 0; one usable timestamp measures no span", s.Usage.Wall)
		}
	})

	t.Run("no timestamp at all", func(t *testing.T) {
		s := loadOnly(t, `
{"type":"user","version":"2.1.220","sessionId":"s","cwd":"/w","message":{"role":"user","content":"go"}}
`)
		if s.Usage.Wall != 0 {
			t.Errorf("wall = %v, want 0", s.Usage.Wall)
		}
	})
}

// TestVersionNamesOneRelease covers a field a heartbeat check compares the
// current harness version against. Joining several into one string made that
// comparison unable to match on exactly the sessions that had already seen a
// bump.
func TestVersionNamesOneRelease(t *testing.T) {
	s := loadOnly(t, `
{"type":"user","version":"2.1.219","sessionId":"s","cwd":"/w","timestamp":"2026-01-01T00:00:00Z","message":{"role":"user","content":"go"}}
{"type":"assistant","version":"2.1.220","sessionId":"s","cwd":"/w","timestamp":"2026-01-01T00:00:01Z","message":{"role":"assistant","model":"m","content":[{"type":"text","text":"done"}]}}
`)
	if s.Fidelity.Version != "2.1.220" {
		t.Errorf("version = %q, want the release the session was last written by", s.Fidelity.Version)
	}
	if !s.Fidelity.Verified {
		t.Error("both releases have been read against, and the session was marked unverified")
	}
}

// TestDelegatedWorkKeepsItsResultsAndItsTokens covers the branch that returned
// to the top of the loop before a delegated call could be indexed or its tokens
// counted: results never paired, and usage dropped rather than moved to the
// delegation it belongs to.
func TestDelegatedWorkKeepsItsResultsAndItsTokens(t *testing.T) {
	s := loadOnly(t, `
{"type":"user","version":"2.1.220","sessionId":"s","cwd":"/w","timestamp":"2026-01-01T00:00:00Z","message":{"role":"user","content":"go"}}
{"type":"assistant","version":"2.1.220","sessionId":"s","cwd":"/w","isSidechain":true,"timestamp":"2026-01-01T00:00:05Z","message":{"role":"assistant","model":"m","usage":{"input_tokens":40,"cache_read_input_tokens":60,"output_tokens":9},"content":[{"type":"tool_use","id":"d1","name":"Bash","input":{"command":"ls"}}]}}
{"type":"user","version":"2.1.220","sessionId":"s","cwd":"/w","isSidechain":true,"timestamp":"2026-01-01T00:00:06Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"d1","content":"a b c"}]}}
`)
	if len(s.Delegated) != 1 {
		t.Fatalf("delegated = %+v, want one delegation", s.Delegated)
	}
	d := s.Delegated[0]
	if len(d.Turns) != 1 || len(d.Turns[0].Tools) != 1 {
		t.Fatalf("delegated turns = %+v", d.Turns)
	}
	if got := d.Turns[0].Tools[0].Result; got == nil || got.Text != "a b c" {
		t.Errorf("delegated result = %+v, want the answer two lines below the call", got)
	}
	if d.Usage.InputTokens != 100 || d.Usage.OutputTokens != 9 {
		t.Errorf("delegation usage = %+v, want the sub-agent's own tokens", d.Usage)
	}
	if s.Usage.InputTokens != 0 || s.Usage.OutputTokens != 0 {
		t.Errorf("session usage = %+v, want delegated work excluded from it", s.Usage)
	}
}

// TestAnOrphanResultWithdrawsTheOrderClaim covers a result whose call is absent
// from the transcript. It is dropped either way — there is nothing to attach it
// to — but a missing call means a prefix of the session is missing, which is
// exactly what OrderComplete exists to report.
func TestAnOrphanResultWithdrawsTheOrderClaim(t *testing.T) {
	s := loadOnly(t, `
{"type":"user","version":"2.1.220","sessionId":"s","cwd":"/w","timestamp":"2026-01-01T00:00:00Z","message":{"role":"user","content":"go"}}
{"type":"user","version":"2.1.220","sessionId":"s","cwd":"/w","timestamp":"2026-01-01T00:00:01Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"gone","content":"ok"}]}}
`)
	if s.OrderComplete {
		t.Error("order was claimed complete while a result answers a call the transcript does not hold")
	}
}

// TestAResultPairsAcrossAReallocatedTurnSlice pins the invariant the pairing
// used to rest on by accident. Registering a pointer into a turn's tool slice
// survives the turn slice growing, and would be orphaned in silence by any later
// change that appended a call to a turn already stored.
func TestAResultPairsAcrossAReallocatedTurnSlice(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"type":"assistant","version":"2.1.220","sessionId":"s","cwd":"/w","timestamp":"2026-01-01T00:00:00Z","message":{"role":"assistant","model":"m","content":[{"type":"tool_use","id":"first","name":"Bash","input":{"command":"ls"}}]}}` + "\n")
	for i := range 200 {
		fmt.Fprintf(&b, `{"type":"assistant","version":"2.1.220","sessionId":"s","cwd":"/w","timestamp":"2026-01-01T00:00:00Z","message":{"role":"assistant","model":"m","content":[{"type":"text","text":"filler %d"}]}}`+"\n", i)
	}
	b.WriteString(`{"type":"user","version":"2.1.220","sessionId":"s","cwd":"/w","timestamp":"2026-01-01T00:00:01Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"first","content":"late"}]}}` + "\n")

	s := loadOnly(t, b.String())
	if got := s.Turns[0].Tools[0].Result; got == nil || got.Text != "late" {
		t.Errorf("result = %+v, want the answer that arrived 200 turns later", got)
	}
}

// TestAnEmptyTranscriptIsMalformed covers a file that loaded as a complete
// session in which nothing happened. A truncated or zero-byte transcript is not
// that, and claiming to know the complete order of nothing is a claim about
// evidence that is not there.
func TestAnEmptyTranscriptIsMalformed(t *testing.T) {
	for name, body := range map[string]string{
		"empty":            "",
		"blank lines":      "\n\n\n",
		"bookkeeping only": `{"type":"queue-operation"}` + "\n" + `{"type":"ai-title"}` + "\n",
		"a fork record":    `{"type":"fork-context-ref","parentSessionId":"ancestor"}` + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parse(context.Background(), strings.NewReader(body)); !errors.Is(err, port.ErrMalformed) {
				t.Errorf("parse returned %v, want ErrMalformed", err)
			}
		})
	}
}

// TestAnOverlongLineIsUnsupportedNotMalformed covers this adapter's own limit
// being reported as damage to the source. The three sentinels reach different
// verdicts, and Assayer's own caps must never produce the one that looks like a
// regression.
func TestAnOverlongLineIsUnsupportedNotMalformed(t *testing.T) {
	line := `{"type":"user","version":"2.1.220","sessionId":"s","cwd":"/w","timestamp":"2026-01-01T00:00:00Z","message":{"role":"user","content":"` +
		strings.Repeat("x", maxLine) + `"}}` + "\n"
	_, err := parse(context.Background(), strings.NewReader(line))
	if !errors.Is(err, port.ErrUnsupported) {
		t.Errorf("parse returned %v, want ErrUnsupported; the source is intact and this adapter cannot read it", err)
	}
	if errors.Is(err, port.ErrMalformed) {
		t.Error("an over-long line was reported as damage to the transcript")
	}
}

// TestALineWithNoRecordTypeIsMalformed covers a null or type-less line, which
// unmarshals into a zero record and was refused as an unknown record type ""  —
// the wrong sentinel and a misleading reason.
func TestALineWithNoRecordTypeIsMalformed(t *testing.T) {
	for _, body := range []string{"null\n", `{"uuid":"u1"}` + "\n"} {
		_, err := parse(context.Background(), strings.NewReader(body))
		if !errors.Is(err, port.ErrMalformed) {
			t.Errorf("parse of %q returned %v, want ErrMalformed", body, err)
		}
	}
}

// TestAByteOrderMarkIsNotDamage covers a first line prefixed with a BOM. No
// file in the measured store carries one; refusing a whole transcript over
// three bytes of encoding preamble would be a manufactured failure.
func TestAByteOrderMarkIsNotDamage(t *testing.T) {
	s := loadOnly(t, "\ufeff"+strings.TrimSpace(transcriptWithATool))
	if len(s.Turns) != 3 {
		t.Errorf("got %d turns, want 3", len(s.Turns))
	}
}

// TestDiscoverFindsADirectoryWholeNameNeedsEncoding covers the query that
// matched almost nothing. The store replaces every character outside
// [A-Za-z0-9-] in a path, not just the separator: of 300 project directories
// recovered from the working directory they record, 300 need the wider rule and
// 234 are missed by replacing slashes alone.
func TestDiscoverFindsADirectoryWholeNameNeedsEncoding(t *testing.T) {
	root := writeStoreIn(t, "-tmp-my-project-x-2", "session.jsonl", transcriptWithATool)
	a := Adapter{Root: root}

	refs, err := a.Discover(context.Background(), port.Query{Dir: "/tmp/my.project_x.2"})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(refs) != 1 {
		t.Errorf("a directory containing punctuation returned %d refs, want 1", len(refs))
	}

	// The comparison was a substring test, so a query naming a prefix of another
	// project's path matched it.
	if refs, _ := a.Discover(context.Background(), port.Query{Dir: "/tmp/my"}); len(refs) != 0 {
		t.Errorf("a query naming a different directory returned %d refs", len(refs))
	}
}

// TestDiscoverFindsASessionByItsNativeIdentifier is what the run seam needs.
// Administering a sitting yields the harness's own identifier for the session it
// produced, and turning that into a Session means asking for it by name.
// Guessing at the newest transcript in a directory instead is a race the moment
// two sittings share a workspace.
func TestDiscoverFindsASessionByItsNativeIdentifier(t *testing.T) {
	const wanted = "de9e5ae2-218a-4bee-8552-fa0ef89c607d"
	const other = "3cd75529-412f-44ed-93be-45cf04679e95"

	root := writeStoreIn(t, "-work-project", wanted+".jsonl",
		strings.ReplaceAll(transcriptWithATool, `"sessionId":"s1"`, `"sessionId":"`+wanted+`"`))
	if err := os.WriteFile(filepath.Join(root, "-work-project", other+".jsonl"),
		[]byte(strings.ReplaceAll(transcriptWithATool, `"sessionId":"s1"`, `"sessionId":"`+other+`"`)+"\n"),
		0o644); err != nil {
		t.Fatalf("write second transcript: %v", err)
	}

	a := Adapter{Root: root}
	refs, err := a.Discover(context.Background(), port.Query{Native: wanted})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("Discover by native identifier returned %d refs, want 1", len(refs))
	}

	s, err := a.Load(context.Background(), refs[0])
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Lineage.ID != wanted {
		t.Errorf("loaded session %q, want %q; the narrowing matched a different "+
			"session than the one named", s.Lineage.ID, wanted)
	}

	none, err := a.Discover(context.Background(), port.Query{Native: "not-a-session"})
	if err != nil {
		t.Fatalf("Discover for an unknown identifier: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("an unknown native identifier returned %d refs, want 0", len(none))
	}
}

// TestNativeMatchesTheFileNameAndSaysSo pins the narrowing's one limitation
// rather than leaving it to be discovered.
//
// The match is on the file's name, because the store names a transcript for the
// session it holds — measured across 400 sampled transcripts, with no exception
// and no transcript missing the identifier — and because matching on contents
// instead would mean opening every file in a store that held 2,485 of them. A
// transcript whose contents disagreed with its name would not be found. That is
// a deliberate trade and it fails closed: nothing is returned, rather than the
// wrong session.
func TestNativeMatchesTheFileNameAndSaysSo(t *testing.T) {
	root := writeStore(t, "named-one-thing.jsonl", transcriptWithATool) // contents say s1

	a := Adapter{Root: root}
	refs, err := a.Discover(context.Background(), port.Query{Native: "s1"})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("Discover matched a transcript whose name is not its session "+
			"identifier; the narrowing is documented as a name match and must "+
			"fail closed, got %d refs", len(refs))
	}
}
