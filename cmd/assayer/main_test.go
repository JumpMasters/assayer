package main

import (
	"strings"
	"testing"
)

func TestRunPrintsUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"help"}, {"--help"}, {"-h"}} {
		var out strings.Builder
		if err := run(args, &out); err != nil {
			t.Fatalf("run(%v) returned %v, want nil", args, err)
		}
		if !strings.Contains(out.String(), "Usage:") {
			t.Errorf("run(%v) printed %q, want usage", args, out.String())
		}
	}
}

func TestRunPrintsVersion(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}, {"-version"}} {
		var out strings.Builder
		if err := run(args, &out); err != nil {
			t.Fatalf("run(%v) returned %v, want nil", args, err)
		}
		if strings.TrimSpace(out.String()) == "" {
			t.Errorf("run(%v) printed nothing, want a version", args)
		}
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	var out strings.Builder
	err := run([]string{"blesss"}, &out)
	if err == nil {
		t.Fatal("expected an error for an unknown command")
	}
	// The typo must appear in the message; "unknown command" alone leaves the
	// user guessing which word was wrong.
	if !strings.Contains(err.Error(), "blesss") {
		t.Errorf("error %q should name the unknown command", err)
	}
	if out.Len() != 0 {
		t.Errorf("unknown command wrote %q to stdout, want nothing", out.String())
	}
}
