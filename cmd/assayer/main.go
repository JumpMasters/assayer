// Command assayer is the Assayer command-line interface.
//
// Assayer is in early development. The design is settled and the variance
// calibration described in the README is under way, but no examining commands
// are implemented yet: `version` is the only command that does anything today.
// The README states what is planned and in what order.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/JumpTechCode/assayer/internal/buildinfo"
)

// exitUsage reports a malformed invocation. Exit codes 70 to 73 are reserved
// for verdicts (see the README's machine contracts) and must never be used to
// report a problem with the command line: a tool that cannot tell a typo from a
// regression is not an instrument.
const exitUsage = 2

const usage = `assayer — golden-session regression testing for coding agents

Usage:
  assayer version    print the version and exit
  assayer help       print this message and exit

Assayer is in early development and no examining commands are implemented yet.
See https://github.com/JumpTechCode/assayer for the roadmap.
`

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "assayer: %v\n", err)
		os.Exit(exitUsage)
	}
}

// run executes a single invocation and reports a usage error, if any. It is
// separated from main, and writes to an injected io.Writer rather than directly
// to os.Stdout, so the whole surface is exercisable from a test without
// exiting the process.
func run(args []string, out io.Writer) error {
	// A bare invocation is someone asking what this is, not an error.
	if len(args) == 0 {
		_, err := io.WriteString(out, usage)
		return err
	}

	switch args[0] {
	case "version", "--version", "-version":
		_, err := fmt.Fprintln(out, buildinfo.String())
		return err
	case "help", "--help", "-h":
		_, err := io.WriteString(out, usage)
		return err
	default:
		return fmt.Errorf("unknown command %q; run \"assayer help\" for usage", args[0])
	}
}
