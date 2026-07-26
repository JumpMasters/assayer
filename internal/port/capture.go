// Package port holds the interfaces components are wired against.
//
// It depends only on the domain types and is the second half of the leaf: the
// data lives next door, the contracts live here. Splitting them keeps the
// package everything imports from becoming the place every shared thing
// accumulates, and gives a new interface a home that is not beside the structs.
package port

import (
	"context"
	"time"

	"github.com/JumpMasters/assayer/internal/assay"
)

// Capture turns sessions produced by one harness into the neutral
// representation. Implementations are the only code in this program permitted
// to know that a particular harness exists.
type Capture interface {
	// ID names the adapter. It appears in reports so that a verdict can be read
	// alongside the guarantee behind it, and it is opaque to every caller.
	ID() string

	// Capabilities is what this adapter can observe in general, independent of
	// any one session. A caller uses it to decide which questions are
	// answerable before spending anything on running them.
	Capabilities() assay.CapabilitySet

	// Tier is the guarantee behind this adapter's observations.
	Tier() assay.Tier

	// Discover lists sessions this adapter can see, newest first.
	Discover(ctx context.Context, q Query) ([]Ref, error)

	// Load translates one discovered session into the neutral representation.
	//
	// It returns ErrNotFound when the reference names nothing — sessions can be
	// pruned between discovery and loading — ErrUnsupported when the source is
	// recognised but cannot be read, and ErrMalformed when it is damaged. The
	// three reach different verdicts and a caller cannot tell them apart by
	// inspecting the error text.
	Load(ctx context.Context, ref Ref) (assay.Session, error)
}

// Query narrows a discovery. Its zero value means everything.
//
// It exists because discovery is not cheap: one machine held 2,485 session
// records after a few months, and a reference's label is derived from a
// session's contents, so listing everything means opening everything. The three
// narrowings here are the ones a command line actually asks for. Adding them
// later would be a breaking change to an interface published in this phase.
type Query struct {
	// Dir limits results to sessions that worked in this directory.
	Dir string
	// Since limits results to sessions no older than this instant.
	Since time.Time
	// Limit caps the number returned. Zero means no cap.
	Limit int
}

// Ref identifies a session to the adapter that produced it.
//
// Compare references by ID alone. The struct is comparable, so == compiles, but
// the timestamp carries a monotonic reading and a location pointer that do not
// survive being written down and read back — an equality that held in memory
// stops holding after a round trip, intermittently and silently.
type Ref struct {
	// ID is meaningful to the adapter that produced it and opaque to everyone
	// else. Core never parses it.
	ID string

	// Label is for a human choosing which session to bless.
	Label string

	At time.Time
}

// portError is the type of this package's sentinel errors.
//
// A defined string type with constant values, rather than package-level
// variables holding errors.New results. Sentinels declared as variables would
// need the leaf's no-package-state rule relaxed, and that rule is load-bearing:
// it is what stops a registry of harnesses from being assembled here at start-up
// through entirely legal imports. The rule is also recorded in an accepted
// decision, which this project changes by superseding rather than by amending in
// passing. Constants cost three lines, keep errors.Is working — a defined string
// type is comparable, so wrapping and unwrapping behave normally — and leave
// both the rule and the record alone.
type portError string

func (e portError) Error() string { return string(e) }

const (
	// ErrUnsupported reports a source the adapter recognises but cannot read.
	ErrUnsupported = portError("capture: source recognised but not readable")

	// ErrMalformed reports a source that is damaged or does not parse.
	ErrMalformed = portError("capture: source malformed")

	// ErrNotFound reports a reference that names no session. Stores are live:
	// a session discovered a moment ago can be pruned before it is loaded.
	ErrNotFound = portError("capture: no such session")
)
