package port

import (
	"context"
	"fmt"

	"github.com/JumpMasters/assayer/internal/assay"
)

// Run administers a sitting on one harness. It is the second of the two seams,
// and the other place in this program permitted to know a harness exists.
//
// The seam is separate from Capture rather than one interface with four methods
// because the two are separately implementable and separately useful: a harness
// whose transcripts can be read but which cannot be driven headlessly still has
// a working capture adapter, and an agent with no transcripts at all can still
// be run from a command template. Joining them would make every adapter claim
// both halves or stub one.
type Run interface {
	// ID names the adapter. It appears in reports so a verdict can be read
	// alongside the guarantee behind it, and it is opaque to every caller.
	ID() string

	// Tier is how the adapter drives the harness: TierNative for a harness's own
	// headless mode, TierWrapped for a command template. TierWire describes a
	// way of watching a run rather than a way of starting one and is not a
	// meaningful answer here.
	Tier() assay.Tier

	// Guarantees is what this adapter can hold and apply, in general.
	//
	// A caller reads it before spending anything, which is the point: an
	// assignment this adapter cannot honour must fail before a process starts,
	// not after the money is gone.
	Guarantees() assay.Guarantees

	// Sit administers one assignment.
	//
	// It returns ErrRefused when the assignment asks for something outside the
	// declared guarantees, and ErrUnavailable when the harness cannot be started
	// at all. Both mean nothing ran and nothing was spent.
	//
	// Anything that did run returns a Sitting and a nil error, however badly it
	// went — a crashed harness is a result about the harness, not a failure of
	// this call, and the distinction is what keeps Assayer's own plumbing from
	// manufacturing a verdict. The caller branches on Sitting.Stop.
	//
	// A cancelled context returns the context's error.
	//
	// The assignment is passed by pointer because it is large enough that
	// copying it per sitting is wasteful, and an implementation must treat it as
	// read-only: it describes conditions the caller imposed, and an adapter that
	// edited them would produce a sitting run under conditions nobody chose.
	Sit(ctx context.Context, a *assay.Assignment) (assay.Sitting, error)
}

const (
	// ErrUnavailable reports a harness that could not be started: not installed,
	// not executable, not on the path.
	ErrUnavailable = portError("run: harness unavailable")

	// ErrRefused reports an assignment asking for a cap or a condition the
	// adapter does not guarantee. It is returned before anything is spent.
	ErrRefused = portError("run: assignment beyond the adapter's guarantees")
)

// Refuse reports whether an assignment asks for more than the guarantees allow,
// naming the first thing it cannot have.
//
// Shared rather than left to each adapter, because "never silently drop a cap"
// is an invariant of the seam and an invariant every implementer has to
// re-derive is one that eventually gets derived wrong. Adapters call it first,
// before touching a process.
//
// The order of the checks is fixed so that an adapter refusing two things at
// once refuses the same one every time; a message that varies between runs
// reads as flakiness.
func Refuse(g assay.Guarantees, a *assay.Assignment) error {
	switch {
	case a.Instruction == "":
		// Not a guarantee, but the same rule: an assignment that cannot produce
		// a meaningful sitting must not start a process. A harness handed
		// nothing to do still costs money to ask.
		return fmt.Errorf("%w: an assignment with no instruction", ErrRefused)
	case a.Caps.BudgetMicroUSD > 0 && g.Budget == assay.EnforcementNone:
		return fmt.Errorf("%w: a budget cap", ErrRefused)
	case a.Caps.Wall > 0 && g.Wall == assay.EnforcementNone:
		return fmt.Errorf("%w: a wall-clock cap", ErrRefused)
	case a.Caps.Turns > 0 && g.Turns == assay.EnforcementNone:
		return fmt.Errorf("%w: a turn cap", ErrRefused)
	case a.Model != "" && !g.Model:
		return fmt.Errorf("%w: a named model", ErrRefused)
	case len(a.Tools) > 0 && !g.Tools:
		return fmt.Errorf("%w: a tool allowlist", ErrRefused)
	case a.Settings != "" && !g.Settings:
		return fmt.Errorf("%w: a settings overlay", ErrRefused)
	}
	return nil
}
