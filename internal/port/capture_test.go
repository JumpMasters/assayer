package port

import (
	"errors"
	"fmt"
	"testing"
)

// TestSentinelsSurviveWrapping is why the sentinels are constants of a defined
// string type rather than package-level variables: the leaf holds no state, and
// this form keeps errors.Is working because a defined string type is comparable.
func TestSentinelsSurviveWrapping(t *testing.T) {
	for _, sentinel := range []error{ErrUnsupported, ErrMalformed, ErrNotFound} {
		wrapped := fmt.Errorf("load ref %q: %w", "abc", sentinel)
		if !errors.Is(wrapped, sentinel) {
			t.Errorf("errors.Is lost %v through wrapping", sentinel)
		}
	}
}

// TestSentinelsAreDistinct pins that the three reach different verdicts. A
// pruned session, a mistyped reference and a damaged source are different
// facts, and one sentinel would force every caller to guess between them.
func TestSentinelsAreDistinct(t *testing.T) {
	all := []error{ErrUnsupported, ErrMalformed, ErrNotFound}
	for i, a := range all {
		for j, b := range all {
			if i == j {
				continue
			}
			if errors.Is(a, b) {
				t.Errorf("sentinel %v is indistinguishable from %v", a, b)
			}
		}
	}
	for _, e := range all {
		if e.Error() == "" {
			t.Error("a sentinel has no message")
		}
	}
}

// TestZeroQueryMeansEverything pins the documented meaning of the zero value,
// which is what makes the narrowing fields additive rather than a behaviour
// change for existing callers.
func TestZeroQueryMeansEverything(t *testing.T) {
	var q Query
	if q.Dir != "" || !q.Since.IsZero() || q.Limit != 0 {
		t.Error("the zero Query is not empty")
	}
}

// TestRefIsComparedByID records the trap rather than leaving it to be found.
// Ref is comparable, so == compiles, but a time carries a monotonic reading and
// a location pointer that do not survive being written down and read back.
func TestRefIsComparedByID(t *testing.T) {
	a := Ref{ID: "one", Label: "first"}
	b := Ref{ID: "one", Label: "renamed later"}

	if a == b {
		t.Error("two refs with the same ID compared equal on every field; " +
			"this test exists because they should be compared on ID alone")
	}
	if a.ID != b.ID {
		t.Error("refs naming the same session disagree on ID")
	}
}
