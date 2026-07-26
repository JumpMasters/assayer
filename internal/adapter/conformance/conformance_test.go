package conformance

import (
	"context"
	"testing"
	"time"

	"github.com/JumpMasters/assayer/internal/assay"
	"github.com/JumpMasters/assayer/internal/port"
)

// TestReferenceSatisfiesTheContract runs the kit against the reference
// implementation, so the contract is exercised from the first commit rather
// than when the first real adapter arrives.
func TestReferenceSatisfiesTheContract(t *testing.T) {
	Verify(t, Reference{})
}

// TestReferenceIsShapedUnlikeTheMeasuredHarness pins the awkwardnesses the
// reference exists to carry. Without this, the reference drifts towards being a
// convenient fixture, and it stops putting any pressure on the vocabulary.
func TestReferenceIsShapedUnlikeTheMeasuredHarness(t *testing.T) {
	s, err := Reference{}.Load(context.Background(), port.Ref{ID: "reference-session"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	t.Run("tool calls belong to a turn with no text", func(t *testing.T) {
		for _, turn := range s.Turns {
			if len(turn.Tools) > 0 && turn.Text == "" {
				return
			}
		}
		t.Error("no turn carries tool calls without text; the no-turn-structure case is gone")
	})

	t.Run("two agent turns run consecutively", func(t *testing.T) {
		for i := 1; i < len(s.Turns); i++ {
			if s.Turns[i-1].Role == assay.RoleAgent && s.Turns[i].Role == assay.RoleAgent {
				return
			}
		}
		t.Error("no two consecutive agent turns; the several-messages-per-ask case is gone")
	})

	t.Run("the model changes mid-session", func(t *testing.T) {
		if got := len(s.Models()); got < 2 {
			t.Errorf("Models() returned %d, want at least 2; the mid-session model change is gone", got)
		}
	})

	t.Run("delegated work is a sibling no call references", func(t *testing.T) {
		if len(s.Delegated) == 0 {
			t.Fatal("no delegated work; the sibling-delegation case is gone")
		}
		for _, turn := range s.Turns {
			for _, c := range turn.Tools {
				if c.DelegatedID != "" {
					t.Error("a call references delegated work; the unreferenced-sibling case is gone")
				}
			}
		}
		if s.OrderComplete {
			t.Error("OrderComplete is true despite unattributed delegation")
		}
	})

	t.Run("tokens are reported without money", func(t *testing.T) {
		if s.Usage.InputTokens == 0 {
			t.Error("no tokens reported")
		}
		if s.Usage.CostMicroUSD != 0 {
			t.Error("a price is reported; the tokens-without-money case is gone")
		}
		if s.Fidelity.Observed.Has(assay.CanSeeCost) {
			t.Error("CanSeeCost is declared; most harnesses report no price at all")
		}
	})

	t.Run("commands arrive already split", func(t *testing.T) {
		for _, turn := range s.Turns {
			for _, c := range turn.Tools {
				if len(c.Argv) > 1 {
					return
				}
			}
		}
		t.Error("no multi-element Argv; the pre-split-arguments case is gone")
	})

	t.Run("there is no version control and no patch", func(t *testing.T) {
		if s.Workspace.Revision != "" || s.Workspace.Patch != "" {
			t.Error("a revision or patch is reported; the no-VCS case is gone")
		}
	})
}

// TestReferenceObservesLessThanEverything keeps the reference from quietly
// becoming a second rich implementation, at which point it would stop being a
// check on anything.
func TestReferenceObservesLessThanEverything(t *testing.T) {
	declared := Reference{}.Capabilities()
	var all assay.CapabilitySet
	all = all.With(assay.Capabilities()...)

	if declared == all {
		t.Error("the reference declares every capability; it is meant to be a limited adapter")
	}
	for _, c := range []assay.Capability{
		assay.CanSeeCost,
		assay.CanSeeLineage,
		assay.CanSeeWorkspacePatch,
		assay.CanSeeMutationContent,
		assay.CanSeeReasoning,
	} {
		if declared.Has(c) {
			t.Errorf("the reference declares %v; it is meant not to observe it", c)
		}
	}
}

// TestFieldCoverageIsTotal is the check that makes a capability declaration
// binding. It is separate from Verify so that it fails once, loudly, rather
// than once per adapter.
func TestFieldCoverageIsTotal(t *testing.T) {
	if err := CheckFieldCoverage(); err != nil {
		t.Error(err)
	}
}

// TestZeroOutsideCapabilitiesCatchesAStrayValue proves the check has teeth
// rather than passing because nothing ever violates it.
func TestZeroOutsideCapabilitiesCatchesAStrayValue(t *testing.T) {
	s, err := Reference{}.Load(context.Background(), port.Ref{ID: "reference-session"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if bad := zeroOutsideCapabilities(&s); len(bad) != 0 {
		t.Fatalf("the reference already violates the rule: %v", bad)
	}

	// A price, from an adapter that never declared it could see one.
	s.Usage.CostMicroUSD = 1234
	bad := zeroOutsideCapabilities(&s)
	if len(bad) == 0 {
		t.Error("a cost was reported without CanSeeCost and the check did not notice")
	}
}

// TestReferenceDiscoveryHonoursTheQuery covers the narrowings that exist so the
// port did not have to be broken later to add them.
func TestReferenceDiscoveryHonoursTheQuery(t *testing.T) {
	ctx := context.Background()
	r := Reference{}

	all, err := r.Discover(ctx, port.Query{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(all) == 0 {
		t.Fatal("the zero query returned nothing; it is meant to mean everything")
	}

	t.Run("a later Since excludes it", func(t *testing.T) {
		refs, err := r.Discover(ctx, port.Query{Since: all[0].At.Add(time.Hour)})
		if err != nil {
			t.Fatalf("Discover: %v", err)
		}
		if len(refs) != 0 {
			t.Errorf("got %d refs for a session older than Since", len(refs))
		}
	})

	t.Run("an earlier Since includes it", func(t *testing.T) {
		refs, err := r.Discover(ctx, port.Query{Since: all[0].At.Add(-time.Hour)})
		if err != nil {
			t.Fatalf("Discover: %v", err)
		}
		if len(refs) != len(all) {
			t.Errorf("got %d refs, want %d", len(refs), len(all))
		}
	})

	t.Run("a matching Dir includes it", func(t *testing.T) {
		refs, err := r.Discover(ctx, port.Query{Dir: "/work"})
		if err != nil {
			t.Fatalf("Discover: %v", err)
		}
		if len(refs) != len(all) {
			t.Errorf("got %d refs for the session's own directory, want %d", len(refs), len(all))
		}
	})
}

// TestExemptFieldsAreOnlyAboutTheCapture guards the one hole in the coverage
// rule. Exempting a field that describes the session, rather than the capture,
// would remove it from the check without anybody deciding to.
func TestExemptFieldsAreOnlyAboutTheCapture(t *testing.T) {
	for _, key := range []string{"Fidelity.Adapter", "Fidelity.Observed", "Session.OrderComplete"} {
		if !exempt(key) {
			t.Errorf("%s should be exempt; it describes the capture, not the session", key)
		}
	}
	for _, key := range []string{"Usage.CostMicroUSD", "Turn.Text", "Mutation.Before", "Lineage.ID"} {
		if exempt(key) {
			t.Errorf("%s must not be exempt; it is an observation about the session", key)
		}
		if _, ok := governed(key); !ok {
			t.Errorf("%s is neither governed nor exempt", key)
		}
	}
}
