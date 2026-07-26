package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/JumpMasters/assayer/internal/buildinfo"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string // substring; empty means "must be empty"
		wantStderr string // substring; empty means "must be empty"
	}{
		{name: "no arguments prints usage", args: nil, wantCode: 0, wantStdout: "Usage:"},
		{name: "version", args: []string{"version"}, wantCode: 0, wantStdout: buildinfo.String()},
		{name: "version long flag", args: []string{"--version"}, wantCode: 0, wantStdout: buildinfo.String()},
		{name: "version short flag", args: []string{"-version"}, wantCode: 0, wantStdout: buildinfo.String()},
		{name: "help", args: []string{"help"}, wantCode: 0, wantStdout: "Usage:"},
		{name: "help long flag", args: []string{"--help"}, wantCode: 0, wantStdout: "Usage:"},
		{name: "help short flag", args: []string{"-h"}, wantCode: 0, wantStdout: "Usage:"},
		{name: "unknown command", args: []string{"wat"}, wantCode: 2, wantStderr: `unknown command "wat"`},
		{name: "version --json", args: []string{"version", "--json"}, wantCode: 0, wantStdout: `"schema":`},
		{name: "a typo'd flag is a usage error, not a silent success",
			args: []string{"version", "--jsonn"}, wantCode: 2, wantStderr: `unexpected argument "--jsonn"`},
		{name: "trailing junk is rejected",
			args: []string{"version", "extra", "junk"}, wantCode: 2, wantStderr: "unexpected argument"},
		{name: "help takes no flags",
			args: []string{"help", "--format", "json"}, wantCode: 2, wantStderr: "unexpected argument"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			got := Run(tt.args, &stdout, &stderr)

			if got != tt.wantCode {
				t.Errorf("Run(%q) = %d, want %d", tt.args, got, tt.wantCode)
			}
			if tt.wantStdout == "" && stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
			if tt.wantStdout != "" && !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.wantStdout)
			}
			if tt.wantStderr == "" && stderr.Len() != 0 {
				t.Errorf("stderr = %q, want empty", stderr.String())
			}
			if tt.wantStderr != "" && !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

// The reserved-range check now lives in exit_test.go, alongside the codes
// themselves, so that adding a command cannot leave it behind.
