package cli

// Process exit codes.
//
// These are a contract. They get written into other people's scripts, so the
// numbers are part of the public interface and changing one breaks code this
// project cannot see.
//
// The verdict range is 80 to 83 rather than 70 to 73, which earlier drafts of
// the design specified on the stated grounds that the range was "clear of
// sysexits' common values". It is not: sysexits.h defines EX_SOFTWARE as 70,
// EX_OSERR 71, EX_OSFILE 72 and EX_CANTCREAT 73, four consecutive values in the
// middle of its 64-to-78 block. The collision at 70 is the damaging one — the
// convention reserves it for "internal software error", which is the opposite
// of a regression in the thing being measured, and is what this program means
// by exitError. A wrapper that speaks sysexits and dies of its own fault would
// have been reported as a regression, which is exactly the manufactured verdict
// the whole design forbids. 80 to 83 sits above the sysexits block and below
// the shell's 126-to-128 range and the 128-plus-signal range.
const (
	// exitOK means everything held, or a command outside the verdict path did
	// what was asked.
	exitOK = 0

	// exitFailure is any failure that is not a verdict and not a usage error:
	// unreadable configuration, a missing exam, a corrupt ledger.
	exitFailure = 1

	// exitUsage is a malformed command line. It is deliberately outside the
	// verdict range: a tool that cannot tell a typo from a regression is not an
	// instrument.
	exitUsage = 2

	// exitRegressed reports a regression verdict at any scope. It requires an
	// escalated series; a single failing sitting never produces it.
	exitRegressed = 80

	// exitDriftSuspected reports a result that is not yet a regression —
	// including an escalation that a budget cap stopped, which pins here with a
	// stated reason rather than being silently downgraded or promoted.
	exitDriftSuspected = 81

	// exitError reports that no verdict could be reached: Assayer's own
	// plumbing, a harness crash, a cap hit. It is not a failure of the thing
	// under examination and must never be reported as one.
	exitError = 82

	// exitStale reports a pin that no longer applies, and only under
	// --strict-stale. A rotted pin is not a regression, so by default staleness
	// does not fail a build; a caller who wants it to has to ask.
	exitStale = 83
)

// verdictExitCodes are the codes reserved for the verdict path. No command
// outside it may return one of these.
func verdictExitCodes() []int {
	return []int{exitRegressed, exitDriftSuspected, exitError, exitStale}
}

// resolveExit picks the exit code when more than one condition holds at once,
// which the suite decision rule makes routine: a suite can carry a regression
// and a rotted pin and an errored exam simultaneously.
//
// Severity order is exitError, then exitRegressed, then exitDriftSuspected,
// then exitStale, then exitOK. Error outranks everything because it says the
// other answers could not be trusted, and reporting a regression computed from
// a run that half failed would be asserting more than was measured.
//
// Without a stated order, two correct implementations of this contract disagree
// and a consumer's `case` statement silently gets a different answer depending
// on which condition the implementation happened to check first.
func resolveExit(codes ...int) int {
	for _, want := range []int{exitError, exitRegressed, exitDriftSuspected, exitStale} {
		for _, got := range codes {
			if got == want {
				return want
			}
		}
	}
	for _, got := range codes {
		if got != exitOK {
			return got
		}
	}
	return exitOK
}
