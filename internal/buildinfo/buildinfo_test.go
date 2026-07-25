package buildinfo

import (
	"runtime/debug"
	"strings"
	"testing"
)

// stubReader returns a read function that reports the given build info.
func stubReader(info *debug.BuildInfo, ok bool) func() (*debug.BuildInfo, bool) {
	return func() (*debug.BuildInfo, bool) { return info, ok }
}

// withVCS builds a BuildInfo carrying the given main-module version and
// version-control settings.
func withVCS(mainVersion, revision, modified string) *debug.BuildInfo {
	info := &debug.BuildInfo{}
	info.Main.Version = mainVersion
	if revision != "" {
		info.Settings = append(info.Settings, debug.BuildSetting{Key: "vcs.revision", Value: revision})
	}
	if modified != "" {
		info.Settings = append(info.Settings, debug.BuildSetting{Key: "vcs.modified", Value: modified})
	}
	return info
}

func TestFormat(t *testing.T) {
	const fullRevision = "0123456789abcdef0123456789abcdef01234567"

	tests := []struct {
		name    string
		version string
		info    *debug.BuildInfo
		ok      bool
		want    string
	}{
		{
			name:    "stamped version wins over build metadata",
			version: "v1.2.3",
			info:    withVCS("v9.9.9", fullRevision, "true"),
			ok:      true,
			want:    "v1.2.3",
		},
		{
			name: "no build info at all",
			ok:   false,
			want: unknown,
		},
		{
			name: "module version from go install",
			info: withVCS("v0.4.0", "", ""),
			ok:   true,
			want: "v0.4.0",
		},
		{
			name: "devel module falls through to the revision",
			info: withVCS("(devel)", fullRevision, "false"),
			ok:   true,
			want: devPrefix + "0123456789ab",
		},
		{
			name: "dirty tree is flagged",
			info: withVCS("(devel)", fullRevision, "true"),
			ok:   true,
			want: devPrefix + "0123456789ab.dirty",
		},
		{
			name: "short revision is not truncated",
			info: withVCS("", "abc123", "false"),
			ok:   true,
			want: devPrefix + "abc123",
		},
		{
			name: "no version and no revision",
			info: withVCS("", "", ""),
			ok:   true,
			want: unknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := format(tt.version, stubReader(tt.info, tt.ok)); got != tt.want {
				t.Errorf("format() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestStringIsNeverEmpty guards the promise callers rely on: String is printed
// unconditionally, so an empty result would silently produce a blank line.
func TestStringIsNeverEmpty(t *testing.T) {
	got := String()
	if strings.TrimSpace(got) == "" {
		t.Error("String() returned an empty version")
	}
}
