package cli

import "testing"

// TestExitCodeValuesAreFixed pins the numbers. They are written into other
// people's scripts, so renumbering one is a breaking change to code this
// project cannot see, and it must not happen by accident.
func TestExitCodeValuesAreFixed(t *testing.T) {
	fixed := map[string]struct {
		got  int
		want int
	}{
		"ok":              {exitOK, 0},
		"failure":         {exitFailure, 1},
		"usage":           {exitUsage, 2},
		"regressed":       {exitRegressed, 80},
		"drift suspected": {exitDriftSuspected, 81},
		"error":           {exitError, 82},
		"stale":           {exitStale, 83},
	}
	for name, c := range fixed {
		if c.got != c.want {
			t.Errorf("%s exit code is %d, want %d", name, c.got, c.want)
		}
	}
}

// TestVerdictRangeAvoidsSysexits is the check that the earlier numbering
// failed.
//
// sysexits.h defines EX_SOFTWARE 70, EX_OSERR 71, EX_OSFILE 72 and EX_CANTCREAT
// 73, inside a block running from 64 to 78. A verdict range overlapping that
// means a wrapper dying of its own internal fault exits 70, and a consumer
// reading Assayer's contract reports a regression that never happened — the
// manufactured verdict the whole design forbids, arriving through the exit code
// rather than through any verdict logic.
func TestVerdictRangeAvoidsSysexits(t *testing.T) {
	const sysexitsLow, sysexitsHigh = 64, 78

	for _, code := range verdictExitCodes() {
		if code >= sysexitsLow && code <= sysexitsHigh {
			t.Errorf("verdict exit code %d falls inside the sysexits block (%d-%d)",
				code, sysexitsLow, sysexitsHigh)
		}
		// 126 and 127 are the shell's "cannot execute" and "not found"; 128+n is
		// a process killed by signal n.
		if code >= 126 {
			t.Errorf("verdict exit code %d collides with shell or signal conventions", code)
		}
		if code <= exitUsage {
			t.Errorf("verdict exit code %d overlaps the non-verdict codes", code)
		}
	}
}

// TestExitPrecedenceIsTotalAndOrdered pins the order in which simultaneous
// conditions resolve.
//
// The suite decision rule makes simultaneity routine: one exam can fail its
// escalated series while another's pin has rotted and a third errored. Without
// a stated order, two conforming implementations disagree, and a consumer's
// branch gets a different answer depending on which condition the
// implementation happened to check first — including, in the worst case, a real
// regression reported as a stale pin and waved through.
func TestExitPrecedenceIsTotalAndOrdered(t *testing.T) {
	tests := []struct {
		name string
		in   []int
		want int
	}{
		{"nothing", nil, exitOK},
		{"only ok", []int{exitOK, exitOK}, exitOK},
		{"regression beats stale", []int{exitStale, exitRegressed}, exitRegressed},
		{"regression beats drift", []int{exitDriftSuspected, exitRegressed}, exitRegressed},
		{"drift beats stale", []int{exitStale, exitDriftSuspected}, exitDriftSuspected},
		{"error beats regression", []int{exitRegressed, exitError}, exitError},
		{"error beats everything", []int{exitStale, exitRegressed, exitDriftSuspected, exitError}, exitError},
		{"order of arguments does not matter", []int{exitError, exitRegressed}, exitError},
		{"a plain failure survives when nothing else holds", []int{exitOK, exitFailure}, exitFailure},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveExit(tt.in...); got != tt.want {
				t.Errorf("resolveExit(%v) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

// TestErrorOutranksRegression states the reason separately from the table,
// because it is the one ordering that carries a product invariant rather than a
// preference: if the run could not be trusted, a regression computed from it
// asserts more than was measured.
func TestErrorOutranksRegression(t *testing.T) {
	if resolveExit(exitRegressed, exitError) != exitError {
		t.Error("a run that could not reach a verdict reported a regression instead")
	}
}

// TestNoCommandUsesTheVerdictRangeYet enforces the rule that keeps the alarm
// and the readout together.
//
// Until a machine-readable verdict object ships, exiting 80 would tell a
// consumer that something regressed and give them no way to learn what, which
// leaves scraping human output as the only option. Those scrapers then become
// an unversioned contract that breaks when the human format changes. So no
// command returns a verdict code until there is a document to read alongside it.
func TestNoCommandUsesTheVerdictRangeYet(t *testing.T) {
	reserved := map[int]bool{}
	for _, c := range verdictExitCodes() {
		reserved[c] = true
	}

	for _, args := range [][]string{
		nil,
		{"version"},
		{"version", "--json"},
		{"--version"},
		{"help"},
		{"--help"},
		{"wat"},
		{"version", "--nonsense"},
	} {
		if got := Run(args, discard{}, discard{}); reserved[got] {
			t.Errorf("Run(%q) returned %d, which is reserved for verdicts", args, got)
		}
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
