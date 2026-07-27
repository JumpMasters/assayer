package conformance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JumpMasters/assayer/internal/assay"
	"github.com/JumpMasters/assayer/internal/port"
)

// VerifyRun exercises a run adapter against the contract every run adapter must
// satisfy, whatever harness it drives.
//
// It has a limit its capture counterpart does not, and the limit shapes the
// whole kit: **this never calls Sit on a path that can reach a harness**.
// Reading a transcript is free, so Verify loads one and checks what came back.
// Administering a sitting is not free — it starts a coding agent against a real
// model on someone's key — and a conformance kit that spends a user's money to
// prove an adapter is well behaved is a kit that gets deleted from the test run.
//
// So what is checked here is the half that terminates before a process starts:
// the declarations, and the refusals that follow from them. Whether an adapter
// drives its harness correctly needs a fixture pinned against a real result, and
// that arrives with each adapter, in its own tests, against a stub.
//
// The declaration check runs one way, as on the capture side. An adapter asking
// for more than it declared is caught here. An adapter declaring a guarantee it
// never keeps is not, and cannot be without watching it run.
func VerifyRun(t *testing.T, r port.Run) {
	t.Helper()
	ctx := context.Background()

	t.Run("identity is stable and non-empty", func(t *testing.T) {
		if r.ID() == "" {
			t.Error("ID() is empty; reports name the adapter behind a verdict")
		}
		if first, second := r.ID(), r.ID(); first != second {
			t.Errorf("ID() is not stable: %q then %q", first, second)
		}
		if first, second := r.Guarantees(), r.Guarantees(); first != second {
			t.Errorf("Guarantees() is not stable: %+v then %+v", first, second)
		}
	})

	t.Run("tier is a way of starting a run", func(t *testing.T) {
		switch r.Tier() {
		case assay.TierNative, assay.TierWrapped:
		case assay.TierUnknown:
			t.Error("Tier() is TierUnknown; an adapter must state its guarantee")
		case assay.TierWire:
			t.Error("Tier() is TierWire, which describes watching a run rather " +
				"than starting one; a run adapter cannot have it")
		}
	})

	t.Run("an assignment with no instruction is refused", func(t *testing.T) {
		// First because it is the cheapest mistake to make and the most
		// expensive to get wrong: a harness handed nothing to do still bills.
		_, err := r.Sit(ctx, &assay.Assignment{})
		if !errors.Is(err, port.ErrRefused) {
			t.Errorf("Sit with an empty assignment returned %v, want ErrRefused", err)
		}
	})

	t.Run("anything beyond the declared guarantees is refused", func(t *testing.T) {
		cases := beyond(r.Guarantees())
		for i := range cases {
			c := &cases[i]
			if _, err := r.Sit(ctx, &c.assignment); !errors.Is(err, port.ErrRefused) {
				t.Errorf("Sit asking for %s, which this adapter does not "+
					"guarantee, returned %v, want ErrRefused; a condition "+
					"dropped rather than refused produces a sitting that looks "+
					"like it ran under conditions nobody imposed", c.what, err)
			}
		}
		if len(cases) == 0 {
			t.Skip("adapter guarantees everything an assignment can ask for; " +
				"nothing to refuse")
		}
	})
}

// beyond builds one assignment per thing the guarantees say the adapter cannot
// do. An adapter that can do everything yields none, and the check above says so
// rather than passing quietly: a vacuous pass and a real one should not look the
// same in a test log.
func beyond(g assay.Guarantees) []struct {
	what       string
	assignment assay.Assignment
} {
	// The instruction and the directory are present throughout, so that a
	// refusal proves the adapter refused the condition under test rather than an
	// assignment that was incomplete to begin with. The directory is never
	// reached: every case here is refused before a process starts.
	base := assay.Assignment{
		Instruction: "do the thing",
		Dir:         "/nonexistent-workspace-for-conformance",
	}

	var out []struct {
		what       string
		assignment assay.Assignment
	}
	add := func(what string, mutate func(*assay.Assignment)) {
		a := base
		mutate(&a)
		out = append(out, struct {
			what       string
			assignment assay.Assignment
		}{what, a})
	}

	if g.Budget == assay.EnforcementNone {
		add("a budget cap", func(a *assay.Assignment) { a.Caps.BudgetMicroUSD = 1_000_000 })
	}
	if g.Wall == assay.EnforcementNone {
		add("a wall-clock cap", func(a *assay.Assignment) { a.Caps.Wall = time.Minute })
	}
	if g.Turns == assay.EnforcementNone {
		add("a turn cap", func(a *assay.Assignment) { a.Caps.Turns = 5 })
	}
	if !g.Model {
		add("a named model", func(a *assay.Assignment) { a.Model = "some-model" })
	}
	if !g.Tools {
		add("a tool allowlist", func(a *assay.Assignment) { a.Tools = []string{"Read"} })
	}
	if !g.Settings {
		add("a settings overlay", func(a *assay.Assignment) { a.Settings = "{}" })
	}
	return out
}
