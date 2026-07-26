package assay

import "fmt"

// Every enumeration in this package reserves its zero value for "unknown".
//
// Go zero-values structs, so whichever variant sits at zero is what an unfilled
// struct claims to be. Putting a meaningful variant there means a session
// nobody populated asserts something — that it was captured from a native
// transcript, that a tool call succeeded, that a turn came from a human — and
// asserts it invisibly, because the value looks deliberate. Unknown at zero
// turns every one of those into something a caller must handle.
//
// The text forms below, not the integer values, are what may ever be written to
// disk. Committed baselines are user data, and inserting a variant in the middle
// of one of these blocks renumbers everything after it; a reader would silently
// reinterpret existing artifacts with no error to detect. UnmarshalText fails
// on an unrecognised name rather than falling back to zero, so an artifact from
// a newer writer is a loud error and not a quiet "unknown".

// Role is who produced a turn.
//
// Two meaningful values is deliberate. A system role belongs to the exam's
// configuration fingerprint rather than to the transcript, and a tool role is
// carried by ToolResult; a harness that models more roles collapses them at the
// adapter boundary rather than widening what core has to understand.
type Role int

const (
	RoleUnknown Role = iota
	RoleHuman
	RoleAgent
)

func (r Role) String() string {
	switch r {
	case RoleHuman:
		return "human"
	case RoleAgent:
		return "agent"
	case RoleUnknown:
		return "unknown"
	}
	return "unknown"
}

// MarshalText implements encoding.TextMarshaler.
func (r Role) MarshalText() ([]byte, error) { return []byte(r.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (r *Role) UnmarshalText(b []byte) error {
	switch string(b) {
	case "unknown":
		*r = RoleUnknown
	case "human":
		*r = RoleHuman
	case "agent":
		*r = RoleAgent
	default:
		return fmt.Errorf("assay: unknown role %q", b)
	}
	return nil
}

// Tier is how much a capture method can be trusted, coarsely and honestly.
//
// TierNative currently covers both formats a vendor documents and private
// stores read by reverse engineering, which are not equally trustworthy. A
// fourth value splitting them is additive and is deferred until an adapter
// needs it.
type Tier int

const (
	TierUnknown Tier = iota
	// TierNative parses a harness's own session records.
	TierNative
	// TierWire records API traffic and cannot see client-side execution.
	TierWire
	// TierWrapped observes only a command and its output.
	TierWrapped
)

func (t Tier) String() string {
	switch t {
	case TierNative:
		return "native"
	case TierWire:
		return "wire"
	case TierWrapped:
		return "wrapped"
	case TierUnknown:
		return "unknown"
	}
	return "unknown"
}

// MarshalText implements encoding.TextMarshaler.
func (t Tier) MarshalText() ([]byte, error) { return []byte(t.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (t *Tier) UnmarshalText(b []byte) error {
	switch string(b) {
	case "unknown":
		*t = TierUnknown
	case "native":
		*t = TierNative
	case "wire":
		*t = TierWire
	case "wrapped":
		*t = TierWrapped
	default:
		return fmt.Errorf("assay: unknown tier %q", b)
	}
	return nil
}

// ToolKind is a neutral classification of what a tool call did.
//
// It exists so that core can reason about tool behaviour without reading tool
// names. A behavioural metric such as a read-to-edit ratio otherwise has only
// one possible implementation — matching the names one harness happens to use,
// inside a package that is supposed to know no harness. That coupling imports
// nothing and names no vendor, so no import rule or vocabulary scan can catch
// it; the only defence is a vocabulary core can use instead.
type ToolKind int

const (
	ToolUnknown ToolKind = iota
	// ToolRead observed state without changing it.
	ToolRead
	// ToolMutate changed files.
	ToolMutate
	// ToolExec ran a command.
	ToolExec
	// ToolDelegate handed work to a sub-agent.
	ToolDelegate
	// ToolOther is a call the adapter recognises but none of the above describe.
	ToolOther
)

func (k ToolKind) String() string {
	switch k {
	case ToolRead:
		return "read"
	case ToolMutate:
		return "mutate"
	case ToolExec:
		return "exec"
	case ToolDelegate:
		return "delegate"
	case ToolOther:
		return "other"
	case ToolUnknown:
		return "unknown"
	}
	return "unknown"
}

// MarshalText implements encoding.TextMarshaler.
func (k ToolKind) MarshalText() ([]byte, error) { return []byte(k.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (k *ToolKind) UnmarshalText(b []byte) error {
	switch string(b) {
	case "unknown":
		*k = ToolUnknown
	case "read":
		*k = ToolRead
	case "mutate":
		*k = ToolMutate
	case "exec":
		*k = ToolExec
	case "delegate":
		*k = ToolDelegate
	case "other":
		*k = ToolOther
	default:
		return fmt.Errorf("assay: unknown tool kind %q", b)
	}
	return nil
}

// MutationKind is what happened to a file.
//
// There is no rename: a Mutation carries one path and a rename needs two.
// Adapters emit a delete and a write.
type MutationKind int

const (
	MutationUnknown MutationKind = iota
	MutationWrite
	MutationDelete
)

func (k MutationKind) String() string {
	switch k {
	case MutationWrite:
		return "write"
	case MutationDelete:
		return "delete"
	case MutationUnknown:
		return "unknown"
	}
	return "unknown"
}

// MarshalText implements encoding.TextMarshaler.
func (k MutationKind) MarshalText() ([]byte, error) { return []byte(k.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (k *MutationKind) UnmarshalText(b []byte) error {
	switch string(b) {
	case "unknown":
		*k = MutationUnknown
	case "write":
		*k = MutationWrite
	case "delete":
		*k = MutationDelete
	default:
		return fmt.Errorf("assay: unknown mutation kind %q", b)
	}
	return nil
}

// Outcome is how a tool call ended.
//
// A boolean cannot carry this. Three different facts would land on it: a
// command that ran and exited non-zero, which is a real result and may
// legitimately fail an assertion; a call the harness refused, or that hit a cap;
// and a call that never returned. The last two are Assayer's own plumbing or the
// environment, and a regression verdict must never be manufactured from either.
// Separating them without this enum would mean searching result text for
// phrases, which is exactly what the text field must not be used for.
type Outcome int

const (
	// OutcomeUnknown means the adapter could not determine how the call ended.
	// An assertion resting on it resolves to an error, never to a failure.
	OutcomeUnknown Outcome = iota
	OutcomeOK
	// OutcomeNonZero is the only value that may carry an assertion to failure.
	OutcomeNonZero
	// OutcomeDenied is a call the harness refused: permissions, sandbox policy.
	OutcomeDenied
	// OutcomeAborted is a call that hit a cap, crashed, or never returned.
	OutcomeAborted
)

func (o Outcome) String() string {
	switch o {
	case OutcomeOK:
		return "ok"
	case OutcomeNonZero:
		return "non-zero"
	case OutcomeDenied:
		return "denied"
	case OutcomeAborted:
		return "aborted"
	case OutcomeUnknown:
		return "unknown"
	}
	return "unknown"
}

// MarshalText implements encoding.TextMarshaler.
func (o Outcome) MarshalText() ([]byte, error) { return []byte(o.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (o *Outcome) UnmarshalText(b []byte) error {
	switch string(b) {
	case "unknown":
		*o = OutcomeUnknown
	case "ok":
		*o = OutcomeOK
	case "non-zero":
		*o = OutcomeNonZero
	case "denied":
		*o = OutcomeDenied
	case "aborted":
		*o = OutcomeAborted
	default:
		return fmt.Errorf("assay: unknown outcome %q", b)
	}
	return nil
}

// Decides reports whether an assertion may be carried to a failure verdict by
// this outcome. Everything except a genuine non-zero exit is Assayer's own
// blind spot or the environment's, and must surface as an error instead.
func (o Outcome) Decides() bool { return o == OutcomeOK || o == OutcomeNonZero }
