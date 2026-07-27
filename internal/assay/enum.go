package assay

import "fmt"

// Almost every enumeration in this package reserves its zero value for "unknown".
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

// Enforcement is how strongly a run adapter can hold one cap.
//
// Three values rather than a boolean, because the two ways of holding a cap
// fail differently and a report that names the guarantee behind a verdict has
// to be able to say which one it had.
type Enforcement int

const (
	// EnforcementNone means the adapter cannot hold this cap at all.
	//
	// An assignment carrying a cap nobody enforces is refused rather than run.
	// Dropping it silently would turn a budget the user set into one nobody
	// kept, and the run would look like a clean sitting afterwards.
	EnforcementNone Enforcement = iota

	// EnforcementSupervised means Assayer holds the cap from outside, by owning
	// the harness process and ending it. What the harness was doing at the
	// moment it died is unknown, and its own accounting of the run is lost.
	EnforcementSupervised

	// EnforcementNative means the harness holds the cap itself: it stops on its
	// own terms and reports why, so the sitting still carries usable figures.
	//
	// Stronger than supervised in that respect and in no other. In particular it
	// bounds nothing. A measured run capped at $0.001 cost $0.0047 — a cap is a
	// threshold the harness notices having crossed, not a ceiling it cannot
	// cross — and a measured run capped at one turn reported two. Anything that
	// prints this as a spending limit is claiming something nobody measured.
	EnforcementNative
)

func (e Enforcement) String() string {
	switch e {
	case EnforcementNone:
		return "none"
	case EnforcementSupervised:
		return "supervised"
	case EnforcementNative:
		return "native"
	}
	return "none"
}

// MarshalText implements encoding.TextMarshaler.
func (e Enforcement) MarshalText() ([]byte, error) { return []byte(e.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (e *Enforcement) UnmarshalText(b []byte) error {
	switch string(b) {
	case "none":
		*e = EnforcementNone
	case "supervised":
		*e = EnforcementSupervised
	case "native":
		*e = EnforcementNative
	default:
		return fmt.Errorf("assay: unknown enforcement %q", b)
	}
	return nil
}

// StopReason is why a sitting ended.
//
// This enum is where ERROR ≠ FAIL stops being a rule people remember and starts
// being a thing the types enforce. Exactly one value permits a verdict about the
// model under examination; every other value describes Assayer, the harness, or
// the environment, and a caller that wants a pass or a fail has to go through
// Decides to get one.
//
// The distinction cannot be recovered from the process exit status, which is the
// obvious place to look for it. A measured run that hit its turn cap and a
// measured run that hit its budget cap both exited 1, the same code a harness
// returns when it has genuinely fallen over.
type StopReason int

const (
	// StopUnknown means the sitting ended in a way the adapter could not
	// classify — output that did not parse, most often. Something happened and
	// nothing can be said about what.
	StopUnknown StopReason = iota

	// StopComplete means the harness finished the work and said so. The only
	// value a pass or a fail may rest on.
	StopComplete

	// StopBudget, StopTurns and StopWall are caps: the sitting was stopped
	// before the work was finished, so the work was not judged.
	StopBudget
	StopTurns
	StopWall

	// StopDenied means the harness stalled on a permission it did not have.
	// Replay is non-interactive by doctrine, so this is a defect in the exam's
	// tool allowlist or its isolation tier, not evidence about the model.
	StopDenied

	// StopFailed means the harness fell over for its own reasons. Its stderr
	// travels with the sitting, because the reason is the harness's to give.
	StopFailed
)

func (s StopReason) String() string {
	switch s {
	case StopComplete:
		return "complete"
	case StopBudget:
		return "budget"
	case StopTurns:
		return "turns"
	case StopWall:
		return "wall"
	case StopDenied:
		return "denied"
	case StopFailed:
		return "failed"
	case StopUnknown:
		return "unknown"
	}
	return "unknown"
}

// MarshalText implements encoding.TextMarshaler.
func (s StopReason) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (s *StopReason) UnmarshalText(b []byte) error {
	switch string(b) {
	case "unknown":
		*s = StopUnknown
	case "complete":
		*s = StopComplete
	case "budget":
		*s = StopBudget
	case "turns":
		*s = StopTurns
	case "wall":
		*s = StopWall
	case "denied":
		*s = StopDenied
	case "failed":
		*s = StopFailed
	default:
		return fmt.Errorf("assay: unknown stop reason %q", b)
	}
	return nil
}

// Decides reports whether a verdict about the thing under examination may rest
// on a sitting that ended this way.
func (s StopReason) Decides() bool { return s == StopComplete }
