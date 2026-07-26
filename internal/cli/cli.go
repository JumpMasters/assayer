// Package cli implements the assayer command line: argument dispatch, human
// readable output, and the mapping from a result to a process exit code.
package cli

import (
	"fmt"
	"io"

	"github.com/JumpMasters/assayer/internal/buildinfo"
)

// Process exit codes.
//
// Exit codes 70 to 73 are reserved for verdicts (see the README's machine
// contracts) and must never be used to report a problem with the command line:
// a tool that cannot tell a typo from a regression is not an instrument. No
// verdict path exists yet, so Run emits only exitOK and exitUsage.
const (
	exitOK    = 0
	exitUsage = 2
)

const usage = `assayer — golden-session regression testing for coding agents

Usage:
  assayer version    print the version and exit
  assayer help       print this message and exit

Assayer is in early development and no examining commands are implemented yet.
See https://github.com/JumpMasters/assayer for the roadmap.
`

// Run executes a single invocation and returns the process exit code.
//
// It returns an int rather than an error because a verdict is the result of a
// run that succeeded: exit codes 70 to 73 report what was found, not that
// anything went wrong, and an error return cannot express them. Keeping the
// whole mapping here rather than in package main also keeps it inside the
// coverage gate, which measures internal/... only.
func Run(args []string, stdout, stderr io.Writer) int {
	// A bare invocation is someone asking what this is, not an error.
	if len(args) == 0 {
		fmt.Fprint(stdout, usage)
		return exitOK
	}

	switch args[0] {
	case "version", "--version", "-version":
		fmt.Fprintln(stdout, buildinfo.String())
		return exitOK
	case "help", "--help", "-h":
		fmt.Fprint(stdout, usage)
		return exitOK
	default:
		fmt.Fprintf(stderr, "assayer: unknown command %q; run \"assayer help\" for usage\n", args[0])
		return exitUsage
	}
}
