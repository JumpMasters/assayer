// Command assayer is the Assayer command-line interface.
//
// Assayer is in early development: `version` and `help` are the only commands
// that do anything today. The README states what is planned and in what order.
//
// Everything this binary does lives in internal/cli, which is inside the
// coverage gate; main is one line so that there is nothing here to test.
package main

import (
	"os"

	"github.com/JumpMasters/assayer/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
