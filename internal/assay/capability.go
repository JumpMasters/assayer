package assay

import (
	"fmt"
	"strings"
)

// Capability is something a capture adapter may or may not be able to observe.
//
// This exists because "not observed" and "did not happen" are different facts
// that must reach different verdicts, and nothing else in the representation
// distinguishes them. An empty list of tool calls means one thing when it came
// from a native transcript and another when it came from an adapter that wraps
// a command and sees only its output. Conflating them turns Assayer's own blind
// spot into the model's regression, which is the one verdict this tool must
// never manufacture.
//
// The set is finer-grained than it first appears it needs to be, because the
// granularity has to match the fields, not the concepts. An adapter that
// watches API traffic sees token counts but no price; one that reads result
// bodies may still have no exit status. A capability covering both halves would
// force such an adapter either to declare something it cannot see or to discard
// something it can.
type Capability uint

const (
	CanSeeLineage Capability = iota
	CanSeeModelIdentity
	CanSeeWorkspace
	CanSeeWorkspacePatch
	CanSeeToolCalls
	CanSeeToolInputs
	CanSeeToolResults
	CanSeeToolOutcome
	CanSeeFileMutations
	CanSeeMutationContent
	CanSeeTurnText
	CanSeeReasoning
	CanSeeTokens
	// CanSeeCost means the harness reported money.
	//
	// It does not mean a price was derived by multiplying token counts against a
	// table. Most harnesses report no cost at all, and one reports a field that
	// is always zero; an adapter that presents a derived or absent price as an
	// observed one turns a cost band into a comparison against a number nobody
	// measured. Relative comparisons belong on tokens, which every harness
	// reports; absolute caps belong here.
	CanSeeCost
	CanSeeTiming
	CanSeeDelegation

	// capabilityCount is the number of capabilities. It must remain last.
	capabilityCount
)

func (c Capability) String() string {
	switch c {
	case CanSeeLineage:
		return "lineage"
	case CanSeeModelIdentity:
		return "model-identity"
	case CanSeeWorkspace:
		return "workspace"
	case CanSeeWorkspacePatch:
		return "workspace-patch"
	case CanSeeToolCalls:
		return "tool-calls"
	case CanSeeToolInputs:
		return "tool-inputs"
	case CanSeeToolResults:
		return "tool-results"
	case CanSeeToolOutcome:
		return "tool-outcome"
	case CanSeeFileMutations:
		return "file-mutations"
	case CanSeeMutationContent:
		return "mutation-content"
	case CanSeeTurnText:
		return "turn-text"
	case CanSeeReasoning:
		return "reasoning"
	case CanSeeTokens:
		return "tokens"
	case CanSeeCost:
		return "cost"
	case CanSeeTiming:
		return "timing"
	case CanSeeDelegation:
		return "delegation"
	}
	return "unknown"
}

// MarshalText implements encoding.TextMarshaler.
func (c Capability) MarshalText() ([]byte, error) {
	if c >= capabilityCount {
		return nil, fmt.Errorf("assay: capability out of range: %d", uint(c))
	}
	return []byte(c.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (c *Capability) UnmarshalText(b []byte) error {
	for i := Capability(0); i < capabilityCount; i++ {
		if i.String() == string(b) {
			*c = i
			return nil
		}
	}
	return fmt.Errorf("assay: unknown capability %q", b)
}

// Capabilities returns every capability, in declaration order.
//
// A function rather than a package-level slice: a shared slice is mutable by
// any caller that receives it, and the leaf holds no state.
func Capabilities() []Capability {
	all := make([]Capability, 0, capabilityCount)
	for i := Capability(0); i < capabilityCount; i++ {
		all = append(all, i)
	}
	return all
}

// CapabilitySet is a set of capabilities.
//
// A bitset rather than a slice of Capability. A slice would leave Fidelity, and
// therefore Session, non-comparable; it would serialise differently depending
// on the order an adapter happened to append in, which matters because
// baselines are committed and compared; it can hold duplicates; and its zero
// value under a length-based construction is a set containing several copies of
// whichever capability is numbered zero, silently declaring it.
type CapabilitySet uint32

// With returns a copy of the set with cs added.
func (s CapabilitySet) With(cs ...Capability) CapabilitySet {
	for _, c := range cs {
		if c < capabilityCount {
			s |= 1 << uint(c)
		}
	}
	return s
}

// Has reports whether c is in the set.
func (s CapabilitySet) Has(c Capability) bool {
	if c >= capabilityCount {
		return false
	}
	return s&(1<<uint(c)) != 0
}

// Contains reports whether every capability in other is also in s.
func (s CapabilitySet) Contains(other CapabilitySet) bool { return s&other == other }

// All returns the capabilities in the set, in declaration order.
func (s CapabilitySet) All() []Capability {
	var out []Capability
	for i := Capability(0); i < capabilityCount; i++ {
		if s.Has(i) {
			out = append(out, i)
		}
	}
	return out
}

// String renders the set as a stable, comma-separated list.
func (s CapabilitySet) String() string {
	names := make([]string, 0, capabilityCount)
	for _, c := range s.All() {
		names = append(names, c.String())
	}
	return strings.Join(names, ",")
}

// MarshalText implements encoding.TextMarshaler. The set is written as names so
// that renumbering the constants cannot reinterpret an artifact already written.
func (s CapabilitySet) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler. An unrecognised name is an
// error rather than a silent omission: a set read from a newer writer must not
// quietly lose the capability a caller is about to rely on.
func (s *CapabilitySet) UnmarshalText(b []byte) error {
	var out CapabilitySet
	if len(b) == 0 {
		*s = 0
		return nil
	}
	for _, name := range strings.Split(string(b), ",") {
		var c Capability
		if err := c.UnmarshalText([]byte(name)); err != nil {
			return err
		}
		out = out.With(c)
	}
	*s = out
	return nil
}
