package assay

import (
	"encoding"
	"testing"
)

// TestZeroValuesAreUnknown is the guard against the most dangerous default an
// enumeration can have. A meaningful variant at zero means an unfilled struct
// makes a claim — that a capture was native, that a call succeeded — silently
// and in a form that looks deliberate.
func TestZeroValuesAreUnknown(t *testing.T) {
	tests := []struct {
		name string
		zero interface{ String() string }
	}{
		{"Role", Role(0)},
		{"Tier", Tier(0)},
		{"ToolKind", ToolKind(0)},
		{"MutationKind", MutationKind(0)},
		{"Outcome", Outcome(0)},
	}
	for _, tt := range tests {
		if got := tt.zero.String(); got != "unknown" {
			t.Errorf("%s zero value is %q, want \"unknown\"", tt.name, got)
		}
	}
}

// TestEnumsRoundTripAsText pins that only names are ever written. Committed
// baselines are user data: if integers were the wire form, inserting a variant
// would renumber everything after it and silently reinterpret artifacts already
// on disk, with nothing to detect the change.
func TestEnumsRoundTripAsText(t *testing.T) {
	roles := []Role{RoleUnknown, RoleHuman, RoleAgent}
	for _, want := range roles {
		b, err := want.MarshalText()
		if err != nil {
			t.Fatalf("Role(%v).MarshalText: %v", want, err)
		}
		var got Role
		if err := got.UnmarshalText(b); err != nil {
			t.Fatalf("Role.UnmarshalText(%q): %v", b, err)
		}
		if got != want {
			t.Errorf("Role round-trip: got %v, want %v", got, want)
		}
	}

	tiers := []Tier{TierUnknown, TierNative, TierWire, TierWrapped}
	for _, want := range tiers {
		b, _ := want.MarshalText()
		var got Tier
		if err := got.UnmarshalText(b); err != nil {
			t.Fatalf("Tier.UnmarshalText(%q): %v", b, err)
		}
		if got != want {
			t.Errorf("Tier round-trip: got %v, want %v", got, want)
		}
	}

	kinds := []ToolKind{ToolUnknown, ToolRead, ToolMutate, ToolExec, ToolDelegate, ToolOther}
	for _, want := range kinds {
		b, _ := want.MarshalText()
		var got ToolKind
		if err := got.UnmarshalText(b); err != nil {
			t.Fatalf("ToolKind.UnmarshalText(%q): %v", b, err)
		}
		if got != want {
			t.Errorf("ToolKind round-trip: got %v, want %v", got, want)
		}
	}

	muts := []MutationKind{MutationUnknown, MutationWrite, MutationDelete}
	for _, want := range muts {
		b, _ := want.MarshalText()
		var got MutationKind
		if err := got.UnmarshalText(b); err != nil {
			t.Fatalf("MutationKind.UnmarshalText(%q): %v", b, err)
		}
		if got != want {
			t.Errorf("MutationKind round-trip: got %v, want %v", got, want)
		}
	}

	outcomes := []Outcome{OutcomeUnknown, OutcomeOK, OutcomeNonZero, OutcomeDenied, OutcomeAborted}
	for _, want := range outcomes {
		b, _ := want.MarshalText()
		var got Outcome
		if err := got.UnmarshalText(b); err != nil {
			t.Fatalf("Outcome.UnmarshalText(%q): %v", b, err)
		}
		if got != want {
			t.Errorf("Outcome round-trip: got %v, want %v", got, want)
		}
	}
}

// TestUnknownNamesAreRejected pins that a value written by a newer version is a
// loud error rather than a quiet fallback to the zero value. Falling back would
// turn "a capability this reader has never heard of" into "unknown", which is
// exactly the reading a caller would then act on.
func TestUnknownNamesAreRejected(t *testing.T) {
	var r Role
	if err := r.UnmarshalText([]byte("wizard")); err == nil {
		t.Error("Role accepted an unknown name")
	}
	var ti Tier
	if err := ti.UnmarshalText([]byte("telepathy")); err == nil {
		t.Error("Tier accepted an unknown name")
	}
	var k ToolKind
	if err := k.UnmarshalText([]byte("summon")); err == nil {
		t.Error("ToolKind accepted an unknown name")
	}
	var m MutationKind
	if err := m.UnmarshalText([]byte("rename")); err == nil {
		t.Error("MutationKind accepted an unknown name")
	}
	var o Outcome
	if err := o.UnmarshalText([]byte("maybe")); err == nil {
		t.Error("Outcome accepted an unknown name")
	}
}

// TestEnumsImplementTextCodecs fails at compile time if a codec is dropped.
func TestEnumsImplementTextCodecs(t *testing.T) {
	var (
		_ encoding.TextMarshaler   = Role(0)
		_ encoding.TextMarshaler   = Tier(0)
		_ encoding.TextMarshaler   = ToolKind(0)
		_ encoding.TextMarshaler   = MutationKind(0)
		_ encoding.TextMarshaler   = Outcome(0)
		_ encoding.TextMarshaler   = Capability(0)
		_ encoding.TextMarshaler   = CapabilitySet(0)
		_ encoding.TextUnmarshaler = new(Role)
		_ encoding.TextUnmarshaler = new(Tier)
		_ encoding.TextUnmarshaler = new(ToolKind)
		_ encoding.TextUnmarshaler = new(MutationKind)
		_ encoding.TextUnmarshaler = new(Outcome)
		_ encoding.TextUnmarshaler = new(Capability)
		_ encoding.TextUnmarshaler = new(CapabilitySet)
	)
}

// TestOnlyGenuineExitsDecide is the ERROR-versus-FAIL boundary in one assertion.
// A refused call, a cap hit, and a call that never returned are Assayer's own
// plumbing or the environment; none of them may carry an assertion to a failure.
func TestOnlyGenuineExitsDecide(t *testing.T) {
	deciding := map[Outcome]bool{
		OutcomeUnknown: false,
		OutcomeOK:      true,
		OutcomeNonZero: true,
		OutcomeDenied:  false,
		OutcomeAborted: false,
	}
	for o, want := range deciding {
		if got := o.Decides(); got != want {
			t.Errorf("Outcome(%v).Decides() = %v, want %v", o, got, want)
		}
	}
}

// TestOnlyCompleteSittingsDecide is the same boundary at the level of a whole
// sitting. A run stopped by a cap did not finish the work, so nothing it
// produced is evidence about whether the work can still be done; a harness that
// fell over is evidence about the harness. One value survives.
func TestOnlyCompleteSittingsDecide(t *testing.T) {
	deciding := map[StopReason]bool{
		StopUnknown:  false,
		StopComplete: true,
		StopBudget:   false,
		StopTurns:    false,
		StopWall:     false,
		StopDenied:   false,
		StopFailed:   false,
	}
	for s, want := range deciding {
		if got := s.Decides(); got != want {
			t.Errorf("StopReason(%v).Decides() = %v, want %v", s, got, want)
		}
	}
	if StopReason(0).String() != "unknown" {
		t.Errorf("StopReason zero value is %q, want \"unknown\"; an unfilled "+
			"sitting must not read as a completed one", StopReason(0))
	}
}

// TestZeroEnforcementPromisesNothing pins the one enumeration here whose zero
// value is deliberately not "unknown".
//
// Enforcement's safe default is not ignorance but refusal: an unfilled
// Guarantees must promise nothing, so that an adapter which forgets to declare a
// cap refuses assignments carrying it rather than accepting them and silently
// keeping no cap at all.
func TestZeroEnforcementPromisesNothing(t *testing.T) {
	if got := Enforcement(0); got != EnforcementNone {
		t.Errorf("Enforcement zero value is %v, want EnforcementNone", got)
	}
	if got := (Guarantees{}).Budget; got != EnforcementNone {
		t.Errorf("an unfilled Guarantees declares Budget %v, want EnforcementNone", got)
	}
}

// TestSittingEnumsRoundTripAsText extends the names-on-the-wire rule to the
// enumerations the run seam adds. Sittings land in a ledger that outlives the
// binary that wrote them.
func TestSittingEnumsRoundTripAsText(t *testing.T) {
	stops := []StopReason{
		StopUnknown, StopComplete, StopBudget, StopTurns, StopWall, StopDenied, StopFailed,
	}
	for _, want := range stops {
		b, err := want.MarshalText()
		if err != nil {
			t.Fatalf("StopReason(%v).MarshalText: %v", want, err)
		}
		var got StopReason
		if err := got.UnmarshalText(b); err != nil {
			t.Fatalf("StopReason.UnmarshalText(%q): %v", b, err)
		}
		if got != want {
			t.Errorf("StopReason round-trip: got %v, want %v", got, want)
		}
	}

	levels := []Enforcement{EnforcementNone, EnforcementSupervised, EnforcementNative}
	for _, want := range levels {
		b, _ := want.MarshalText()
		var got Enforcement
		if err := got.UnmarshalText(b); err != nil {
			t.Fatalf("Enforcement.UnmarshalText(%q): %v", b, err)
		}
		if got != want {
			t.Errorf("Enforcement round-trip: got %v, want %v", got, want)
		}
	}

	var bad StopReason
	if err := bad.UnmarshalText([]byte("nope")); err == nil {
		t.Error("StopReason.UnmarshalText accepted an unknown name; a sitting " +
			"written by a newer writer must not decode as an unrelated stop")
	}
	var badLevel Enforcement
	if err := badLevel.UnmarshalText([]byte("nope")); err == nil {
		t.Error("Enforcement.UnmarshalText accepted an unknown name")
	}
}
