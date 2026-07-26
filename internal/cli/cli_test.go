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

// TestVerdictExitCodesUnused guards a public contract: exit codes 70 to 73 are
// reserved for verdicts, and no command implemented today produces a verdict.
func TestVerdictExitCodesUnused(t *testing.T) {
	for _, args := range [][]string{nil, {"version"}, {"help"}, {"wat"}} {
		var stdout, stderr bytes.Buffer
		if got := Run(args, &stdout, &stderr); got >= 70 && got <= 73 {
			t.Errorf("Run(%q) = %d, which is inside the reserved verdict range", args, got)
		}
	}
}
