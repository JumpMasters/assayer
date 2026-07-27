package port

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/JumpMasters/assayer/internal/assay"
)

// everything is an adapter that can hold and apply anything.
func everything() assay.Guarantees {
	return assay.Guarantees{
		Budget:   assay.EnforcementNative,
		Wall:     assay.EnforcementSupervised,
		Turns:    assay.EnforcementNative,
		Model:    true,
		Tools:    true,
		Settings: true,
	}
}

func TestRefuseCatchesEveryUndeclaredCondition(t *testing.T) {
	work := assay.Assignment{Instruction: "do the thing", Dir: "/somewhere"}

	tests := []struct {
		name       string
		guarantees func() assay.Guarantees
		assignment func() assay.Assignment
		want       string
	}{
		{
			name:       "no instruction",
			guarantees: everything,
			assignment: func() assay.Assignment { return assay.Assignment{} },
			want:       "no instruction",
		},
		{
			name: "budget cap nobody keeps",
			guarantees: func() assay.Guarantees {
				g := everything()
				g.Budget = assay.EnforcementNone
				return g
			},
			assignment: func() assay.Assignment {
				a := work
				a.Caps.BudgetMicroUSD = 5_000_000
				return a
			},
			want: "budget cap",
		},
		{
			name: "wall cap nobody keeps",
			guarantees: func() assay.Guarantees {
				g := everything()
				g.Wall = assay.EnforcementNone
				return g
			},
			assignment: func() assay.Assignment {
				a := work
				a.Caps.Wall = time.Minute
				return a
			},
			want: "wall-clock cap",
		},
		{
			name: "turn cap nobody keeps",
			guarantees: func() assay.Guarantees {
				g := everything()
				g.Turns = assay.EnforcementNone
				return g
			},
			assignment: func() assay.Assignment {
				a := work
				a.Caps.Turns = 3
				return a
			},
			want: "turn cap",
		},
		{
			name: "model nobody can select",
			guarantees: func() assay.Guarantees {
				g := everything()
				g.Model = false
				return g
			},
			assignment: func() assay.Assignment {
				a := work
				a.Model = "some-model"
				return a
			},
			want: "named model",
		},
		{
			name: "allowlist nobody can apply",
			guarantees: func() assay.Guarantees {
				g := everything()
				g.Tools = false
				return g
			},
			assignment: func() assay.Assignment {
				a := work
				a.Tools = []string{"Read"}
				return a
			},
			want: "tool allowlist",
		},
		{
			name: "settings nobody can overlay",
			guarantees: func() assay.Guarantees {
				g := everything()
				g.Settings = false
				return g
			},
			assignment: func() assay.Assignment {
				a := work
				a.Settings = "{}"
				return a
			},
			want: "settings overlay",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assignment := tt.assignment()
			err := Refuse(tt.guarantees(), &assignment)
			if !errors.Is(err, ErrRefused) {
				t.Fatalf("Refuse returned %v, want ErrRefused", err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Refuse said %q, want it to name %q; a refusal that "+
					"does not say what was refused sends the reader guessing",
					err, tt.want)
			}
		})
	}
}

// TestRefuseAllowsWhatIsDeclared is the other half. A guard that refuses
// everything is not a guard, it is an outage.
func TestRefuseAllowsWhatIsDeclared(t *testing.T) {
	full := assay.Assignment{
		Instruction: "do the thing",
		Dir:         "/tmp/somewhere",
		Model:       "some-model",
		Tools:       []string{"Read", "Edit"},
		Settings:    `{"permissions":{}}`,
		Caps: assay.Caps{
			BudgetMicroUSD: 5_000_000,
			Wall:           time.Minute,
			Turns:          10,
		},
	}
	if err := Refuse(everything(), &full); err != nil {
		t.Errorf("Refuse rejected an assignment an adapter declared it can hold: %v", err)
	}
}

// TestRefuseIgnoresUnsetCaps pins that a zero cap is no cap. An adapter that
// cannot hold a wall clock must still accept assignments that never asked it to.
func TestRefuseIgnoresUnsetCaps(t *testing.T) {
	none := assay.Guarantees{}
	if err := Refuse(none, &assay.Assignment{Instruction: "do the thing", Dir: "/somewhere"}); err != nil {
		t.Errorf("Refuse rejected an assignment with no caps against an adapter "+
			"that guarantees nothing: %v", err)
	}
}

// TestRefuseOrderIsFixed pins that an assignment breaking two rules names the
// same one every time. A message that varies between identical runs reads as
// flakiness in the thing under examination.
func TestRefuseOrderIsFixed(t *testing.T) {
	a := assay.Assignment{
		Instruction: "do the thing",
		Dir:         "/somewhere",
		Model:       "some-model",
		Tools:       []string{"Read"},
		Caps:        assay.Caps{BudgetMicroUSD: 1, Wall: time.Minute, Turns: 1},
	}
	first := Refuse(assay.Guarantees{}, &a)
	for range 20 {
		if got := Refuse(assay.Guarantees{}, &a); got.Error() != first.Error() {
			t.Fatalf("Refuse is not deterministic: %q then %q", first, got)
		}
	}
	if !strings.Contains(first.Error(), "budget cap") {
		t.Errorf("Refuse reported %q first, want the budget cap; the order is "+
			"part of the contract", first)
	}
}

// TestRunSentinelsAreDistinct keeps the two ways of not running apart. One means
// the assignment was wrong and one means the machine was, and a caller cannot
// tell them apart by reading the message.
func TestRunSentinelsAreDistinct(t *testing.T) {
	if errors.Is(ErrRefused, ErrUnavailable) || errors.Is(ErrUnavailable, ErrRefused) {
		t.Error("ErrRefused and ErrUnavailable match each other")
	}
	for _, capture := range []error{ErrNotFound, ErrUnsupported, ErrMalformed} {
		if errors.Is(ErrRefused, capture) || errors.Is(ErrUnavailable, capture) {
			t.Errorf("a run sentinel matches the capture sentinel %v", capture)
		}
	}
}

// TestEveryAssignmentFieldIsGoverned is the run seam's default-deny check, and
// it is here because the field it would have caught had already slipped through.
//
// Assignment.Dir shipped in review governed by nothing: Refuse ignored it, no
// Guarantees entry covered it, and an empty value meant the harness ran in
// Assayer's own working directory — the user's repository — with whatever tool
// reach the harness defaults to. The capture side has the same shape of check
// over Session's fields, for the same reason: a field nobody decided about is
// not a field with a safe default, it is a field with an accidental one.
//
// A new field on Assignment fails this test until it is either refused when
// unusable or covered by a guarantee. Adding it to the map is the decision.
func TestEveryAssignmentFieldIsGoverned(t *testing.T) {
	// How each field is kept from being silently ignored.
	governed := map[string]string{
		"Instruction": "refused when empty",
		"Dir":         "refused when empty",
		"Model":       "Guarantees.Model",
		"Tools":       "Guarantees.Tools",
		"Settings":    "Guarantees.Settings",
		"Caps":        "Guarantees.Budget, .Wall, .Turns; negatives refused",
	}

	fields := reflect.TypeFor[assay.Assignment]()
	for i := range fields.NumField() {
		name := fields.Field(i).Name
		if _, ok := governed[name]; !ok {
			t.Errorf("Assignment.%s is governed by nothing: no refusal covers it "+
				"and no guarantee declares it, so an adapter may ignore it and "+
				"produce a sitting that looks like it ran under conditions nobody "+
				"imposed. Decide, then add it here", name)
		}
		delete(governed, name)
	}
	for name := range governed {
		t.Errorf("the map names %q, which Assignment no longer has", name)
	}
}

// TestRefuseRejectsAnAssignmentThatIsNotThere. Sit takes a pointer, so nil is
// reachable from any caller, and a published interface must not answer it with a
// panic.
func TestRefuseRejectsAnAssignmentThatIsNotThere(t *testing.T) {
	if err := Refuse(everything(), nil); !errors.Is(err, ErrRefused) {
		t.Errorf("Refuse(nil) returned %v, want ErrRefused", err)
	}
}

// TestRefuseRejectsANegativeCap. A negative cap is not "no cap", and the >0 test
// that would drop it is wrong in the one direction that costs money: an
// orchestrator computing a wall cap as the time remaining before a deadline
// produces a negative exactly when it is already late, so the run that most
// needed a bound would get none.
func TestRefuseRejectsANegativeCap(t *testing.T) {
	base := assay.Assignment{Instruction: "do the thing", Dir: "/somewhere"}

	for name, mutate := range map[string]func(*assay.Assignment){
		"budget": func(a *assay.Assignment) { a.Caps.BudgetMicroUSD = -1 },
		"wall":   func(a *assay.Assignment) { a.Caps.Wall = -time.Second },
		"turns":  func(a *assay.Assignment) { a.Caps.Turns = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			a := base
			mutate(&a)
			err := Refuse(everything(), &a)
			if !errors.Is(err, ErrRefused) {
				t.Errorf("a negative %s cap returned %v, want ErrRefused", name, err)
			}
		})
	}
}

// TestRefuseRejectsAnAssignmentWithNoWorkingDirectory. Without this the harness
// inherits Assayer's own, which is wherever the user happened to run the
// command.
func TestRefuseRejectsAnAssignmentWithNoWorkingDirectory(t *testing.T) {
	err := Refuse(everything(), &assay.Assignment{Instruction: "do the thing"})
	if !errors.Is(err, ErrRefused) {
		t.Errorf("an assignment with no working directory returned %v, want ErrRefused", err)
	}
	if !strings.Contains(err.Error(), "working directory") {
		t.Errorf("Refuse said %q, want it to name the working directory", err)
	}
}
