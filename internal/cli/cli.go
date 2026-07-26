// Package cli implements the assayer command line: argument dispatch, human
// readable output, machine-readable documents, and the mapping from a result to
// a process exit code.
package cli

import (
	"fmt"
	"io"
)

const usage = `assayer — golden-session regression testing for coding agents

Usage:
  assayer version [--json]   print the version and exit
  assayer help               print this message and exit

Assayer is in early development and no examining commands are implemented yet.
See https://github.com/JumpMasters/assayer for the roadmap.
`

// Run executes a single invocation and returns the process exit code.
//
// It returns an int rather than an error because a verdict is the result of a
// run that succeeded: the verdict exit codes report what was found, not that
// anything went wrong, and an error return cannot express them. Keeping the
// whole mapping here rather than in package main also keeps it inside the
// coverage gate, which measures internal/... only.
//
// Only `run` and `ci` will ever return a verdict code. Every other command
// returns 0, 1, or 2, so a consumer wiring several commands into a pipeline can
// tell which of them is allowed to make a claim about the thing under test.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		// A bare invocation is someone asking what this is, not an error.
		fmt.Fprint(stdout, usage)
		return exitOK
	}

	switch args[0] {
	case "version", "--version", "-version":
		asJSON, err := parseFlags(args[1:], "--json")
		if err != nil {
			return usageError(stderr, err)
		}
		if err := writeVersion(stdout, asJSON["--json"], currentBuild()); err != nil {
			fmt.Fprintf(stderr, "assayer: %v\n", err)
			return exitFailure
		}
		return exitOK

	case "help", "--help", "-h":
		if _, err := parseFlags(args[1:]); err != nil {
			return usageError(stderr, err)
		}
		fmt.Fprint(stdout, usage)
		return exitOK

	default:
		return usageError(stderr, fmt.Errorf("unknown command %q", args[0]))
	}
}

// parseFlags accepts only the flags a command declares and rejects everything
// else.
//
// Silently ignoring what follows a recognised command is worse than it looks:
// `assayer version --jsonn` would print human text and exit 0, so a script that
// asked for a machine-readable document receives prose and a success code, and
// discovers the problem somewhere further downstream with nothing pointing back
// here. It also makes the version document useless for discovery, since an
// older binary that does not know a flag has to reject it loudly rather than
// returning something that looks like an answer.
func parseFlags(args []string, allowed ...string) (map[string]bool, error) {
	permitted := make(map[string]bool, len(allowed))
	for _, f := range allowed {
		permitted[f] = true
	}

	seen := make(map[string]bool, len(allowed))
	for _, arg := range args {
		if !permitted[arg] {
			return nil, fmt.Errorf("unexpected argument %q", arg)
		}
		seen[arg] = true
	}
	return seen, nil
}

func usageError(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "assayer: %v; run \"assayer help\" for usage\n", err)
	return exitUsage
}
