package capture

import (
	"context"
	"errors"
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
	root := t.TempDir()
	dir := filepath.Join(root, "-work-project")
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
}
