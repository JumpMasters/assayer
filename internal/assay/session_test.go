package assay

import (
	"testing"
	"time"
)

func TestCapabilitySetOperations(t *testing.T) {
	var s CapabilitySet

	if s.Has(CanSeeCost) {
		t.Error("empty set reports a capability")
	}

	s = s.With(CanSeeCost, CanSeeTokens)
	if !s.Has(CanSeeCost) || !s.Has(CanSeeTokens) {
		t.Error("With did not add both capabilities")
	}
	if s.Has(CanSeeLineage) {
		t.Error("set reports a capability that was never added")
	}

	// Adding twice is the same set: a bitset cannot hold duplicates, which is
	// half the reason it is not a slice.
	if s.With(CanSeeCost) != s {
		t.Error("adding an existing capability changed the set")
	}

	// Order of construction cannot change the value, which is the other half:
	// baselines are committed and compared, and two adapters that declare the
	// same things in different orders must produce the same bytes.
	if CapabilitySet(0).With(CanSeeTokens, CanSeeCost) != s {
		t.Error("declaration order changed the set")
	}

	if !s.Contains(CapabilitySet(0).With(CanSeeCost)) {
		t.Error("Contains rejected a subset")
	}
	if s.Contains(CapabilitySet(0).With(CanSeeLineage)) {
		t.Error("Contains accepted a non-subset")
	}

	if got := len(s.All()); got != 2 {
		t.Errorf("All() returned %d capabilities, want 2", got)
	}
}

func TestCapabilitySetRoundTripsAsNames(t *testing.T) {
	want := CapabilitySet(0).With(CanSeeLineage, CanSeeCost, CanSeeDelegation)

	b, err := want.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText: %v", err)
	}
	if string(b) != "lineage,cost,delegation" {
		t.Errorf("MarshalText = %q, want capability names in declaration order", b)
	}

	var got CapabilitySet
	if err := got.UnmarshalText(b); err != nil {
		t.Fatalf("UnmarshalText(%q): %v", b, err)
	}
	if got != want {
		t.Errorf("round-trip: got %v, want %v", got, want)
	}

	var empty CapabilitySet
	if err := empty.UnmarshalText(nil); err != nil {
		t.Errorf("UnmarshalText(nil): %v", err)
	}
	if empty != 0 {
		t.Error("empty text did not decode to the empty set")
	}

	var bad CapabilitySet
	if err := bad.UnmarshalText([]byte("lineage,telepathy")); err == nil {
		t.Error("UnmarshalText accepted an unknown capability name")
	}
}

func TestCapabilitiesAreDistinctAndComplete(t *testing.T) {
	all := Capabilities()
	if len(all) == 0 {
		t.Fatal("Capabilities() is empty")
	}

	seen := map[string]bool{}
	for _, c := range all {
		name := c.String()
		if name == "unknown" {
			t.Errorf("capability %d has no name", uint(c))
		}
		if seen[name] {
			t.Errorf("two capabilities share the name %q", name)
		}
		seen[name] = true
	}

	// Every capability must fit the bitset's width, or Has silently returns
	// false for the ones past the end.
	var s CapabilitySet
	s = s.With(all...)
	for _, c := range all {
		if !s.Has(c) {
			t.Errorf("capability %v does not fit CapabilitySet", c)
		}
	}
}

// TestModelsAreCollectedPerTurn covers the measurement that drove the model out
// of the session and into the turn: 54 of 2,459 local transcripts used more
// than one model, and 3 used three.
func TestModelsAreCollectedPerTurn(t *testing.T) {
	small := Model{Alias: "small", Canonical: "small-1"}
	large := Model{Alias: "large", Canonical: "large-1"}

	s := Session{Turns: []Turn{
		{Model: small},
		{Model: small},
		{Model: large},
		{}, // a turn with no observed model contributes nothing
		{Model: small},
	}}

	got := s.Models()
	if len(got) != 2 {
		t.Fatalf("Models() returned %d, want 2: %v", len(got), got)
	}
	if got[0] != small || got[1] != large {
		t.Errorf("Models() = %v, want first-use order [small large]", got)
	}

	var empty Session
	if len(empty.Models()) != 0 {
		t.Error("a session with no turns reported a model")
	}
}

// TestFidelityDistinguishesPartialFromWhole is the mechanism that keeps a
// blind spot from becoming a regression. Partial visibility is ordinary —
// compaction, truncation, a recorder that sees requested edits but not what a
// shell command rewrote — and a capability declared whole when it is partial
// produces a failure on evidence that was never complete.
func TestFidelityDistinguishesPartialFromWhole(t *testing.T) {
	f := Fidelity{
		Observed: CapabilitySet(0).With(CanSeeToolCalls, CanSeeFileMutations),
		Partial:  CapabilitySet(0).With(CanSeeFileMutations),
	}

	if !f.Sees(CanSeeToolCalls) {
		t.Error("Sees rejected a wholly observed capability")
	}
	if f.SeesPartly(CanSeeToolCalls) {
		t.Error("SeesPartly accepted a wholly observed capability")
	}

	if f.Sees(CanSeeFileMutations) {
		t.Error("Sees accepted a partially observed capability; assertions would run on incomplete evidence")
	}
	if !f.SeesPartly(CanSeeFileMutations) {
		t.Error("SeesPartly rejected a partially observed capability")
	}

	if f.Sees(CanSeeCost) || f.SeesPartly(CanSeeCost) {
		t.Error("an undeclared capability was reported as observed")
	}
}

func TestUsageIsExclusiveOfDelegation(t *testing.T) {
	// The type cannot enforce this, so the test pins the documented meaning:
	// a parent's figures stand alone and summing is the caller's job. Left
	// unstated, adapters would differ and cost comparisons would be meaningless
	// while looking fine.
	s := Session{
		Usage:     Usage{InputTokens: 100, Wall: time.Minute},
		Delegated: []Delegation{{Usage: Usage{InputTokens: 10}}},
	}

	if s.Usage.InputTokens != 100 {
		t.Error("a session's own usage should not include its delegations")
	}

	total := s.Usage.InputTokens
	for _, d := range s.Delegated {
		total += d.Usage.InputTokens
	}
	if total != 110 {
		t.Errorf("summed usage = %d, want 110", total)
	}
}
