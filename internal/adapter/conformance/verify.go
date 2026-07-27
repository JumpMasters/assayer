package conformance

import (
	"context"
	"errors"
	"testing"

	"github.com/JumpMasters/assayer/internal/assay"
	"github.com/JumpMasters/assayer/internal/port"
)

// Verify exercises a capture adapter against the contract every adapter must
// satisfy, whatever harness it reads.
//
// It takes a *testing.T and therefore lives in non-test source, so that other
// adapters' tests can call it. That is deliberate: a kit only adapters
// themselves could run would not be shared.
//
// What this checks is the contract half. It does not check that an adapter
// reads its harness correctly — that needs a fixture corpus per adapter, round
// tripped, which arrives with each adapter and depends on a redaction gate that
// is not trustworthy yet. An adapter passing Verify is well behaved; it has not
// thereby been shown to parse anything correctly.
//
// The capability check runs one way, for the reason given in coverage.go: a
// value outside a declared capability fails here, a capability declared whole
// and never filled does not.
func Verify(t *testing.T, c port.Capture) {
	t.Helper()
	ctx := context.Background()

	t.Run("identity is stable and non-empty", func(t *testing.T) {
		if c.ID() == "" {
			t.Error("ID() is empty; reports name the adapter behind a verdict")
		}
		if first, second := c.ID(), c.ID(); first != second {
			t.Errorf("ID() is not stable: %q then %q", first, second)
		}
		if c.Tier() == assay.TierUnknown {
			t.Error("Tier() is TierUnknown; an adapter must state its guarantee")
		}
		if first, second := c.Tier(), c.Tier(); first != second {
			t.Errorf("Tier() is not stable: %v then %v", first, second)
		}
		if first, second := c.Capabilities(), c.Capabilities(); first != second {
			t.Errorf("Capabilities() is not stable: %v then %v", first, second)
		}
		if c.Capabilities() == 0 {
			t.Error("Capabilities() is empty; an adapter that observes nothing cannot capture")
		}
	})

	t.Run("field coverage is total", func(t *testing.T) {
		if err := CheckFieldCoverage(); err != nil {
			t.Error(err)
		}
	})

	t.Run("discover on a non-matching query is empty, not an error", func(t *testing.T) {
		refs, err := c.Discover(ctx, port.Query{Dir: "/nonexistent-directory-for-conformance"})
		if err != nil {
			t.Fatalf("Discover with a non-matching query returned %v, want no error", err)
		}
		if len(refs) != 0 {
			t.Errorf("Discover with a non-matching query returned %d refs, want 0", len(refs))
		}
	})

	t.Run("discover honours a limit", func(t *testing.T) {
		refs, err := c.Discover(ctx, port.Query{Limit: 1})
		if err != nil {
			t.Fatalf("Discover: %v", err)
		}
		if len(refs) > 1 {
			t.Errorf("Discover with Limit 1 returned %d refs", len(refs))
		}
	})

	t.Run("discover by an unknown native identifier is empty, not an error", func(t *testing.T) {
		// The narrowing the run seam depends on. An adapter that ignores it
		// returns whatever it would have returned anyway, and a sitting is then
		// attributed to whichever session happened to sort first — the race the
		// field exists to remove, made silent. Checking that it fails closed is
		// the half of the contract a kit can check without knowing the store.
		refs, err := c.Discover(ctx, port.Query{Native: "definitely-not-a-session"})
		if err != nil {
			t.Fatalf("Discover by an unknown native identifier returned %v, want no error", err)
		}
		if len(refs) != 0 {
			t.Errorf("Discover by an unknown native identifier returned %d refs, want 0; "+
				"an adapter that cannot resolve one must return nothing rather than "+
				"something", len(refs))
		}
	})

	t.Run("load of an unknown ref reports not found", func(t *testing.T) {
		_, err := c.Load(ctx, port.Ref{ID: "definitely-not-a-session"})
		if !errors.Is(err, port.ErrNotFound) {
			t.Errorf("Load of an unknown ref returned %v, want ErrNotFound; a pruned or "+
				"mistyped reference must be distinguishable from an unreadable one", err)
		}
	})

	refs, err := c.Discover(ctx, port.Query{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(refs) == 0 {
		t.Skip("adapter discovered no sessions; the session checks need at least one")
	}

	s, err := c.Load(ctx, refs[0])
	if err != nil {
		t.Fatalf("Load(%q): %v", refs[0].ID, err)
	}

	t.Run("session declares the adapter that produced it", func(t *testing.T) {
		if s.Fidelity.Adapter != c.ID() {
			t.Errorf("session Fidelity.Adapter = %q, adapter ID is %q", s.Fidelity.Adapter, c.ID())
		}
		if s.Fidelity.Tier != c.Tier() {
			t.Errorf("session Fidelity.Tier = %v, adapter Tier is %v", s.Fidelity.Tier, c.Tier())
		}
	})

	t.Run("observed is within what the adapter claims", func(t *testing.T) {
		if !c.Capabilities().Contains(s.Fidelity.Observed) {
			t.Errorf("session observed %v, beyond the adapter's %v; an adapter may see less "+
				"in one session than it can in general, never more",
				s.Fidelity.Observed, c.Capabilities())
		}
		if !s.Fidelity.Observed.Contains(s.Fidelity.Partial) {
			t.Errorf("session partial %v is not within observed %v",
				s.Fidelity.Partial, s.Fidelity.Observed)
		}
	})

	t.Run("nothing is reported outside the declared capabilities", func(t *testing.T) {
		for _, bad := range zeroOutsideCapabilities(&s) {
			t.Errorf("%s; a caller trusting the declaration would read it as evidence", bad)
		}
	})

	t.Run("a session with tool calls but no turn text still loads", func(t *testing.T) {
		// The turn structure is a reconstruction every adapter performs, and not
		// every harness has one to reconstruct from. Nothing in the
		// representation may require it.
		var calls int
		for i := range s.Turns {
			calls += len(s.Turns[i].Tools)
			if len(s.Turns[i].Tools) > 0 && s.Turns[i].Text == "" {
				return // the shape exists and was accepted
			}
		}
		if calls == 0 {
			t.Skip("adapter reported no tool calls")
		}
	})
}
