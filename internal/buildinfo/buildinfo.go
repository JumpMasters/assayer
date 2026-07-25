// Package buildinfo reports the version of the running binary.
//
// Released binaries carry a version stamped in at link time. Binaries built
// straight from a checkout carry none, so the version is derived from the
// version-control metadata the Go toolchain embeds automatically. Either way
// the reported string identifies the exact source a result came from, which is
// what makes a recorded verdict traceable back to the code that produced it.
package buildinfo

import "runtime/debug"

// Version is the release version, stamped at build time with
// -ldflags "-X github.com/JumpMasters/assayer/internal/buildinfo.Version=<tag>".
// It is empty for builds made directly from a checkout.
var Version string

const (
	// unknown is reported when neither a stamped version nor usable
	// version-control metadata is available, which happens for binaries built
	// outside a repository (for example `go build` on an extracted archive).
	unknown = "unknown"

	// devPrefix marks a version derived from version-control metadata rather
	// than from a release tag.
	devPrefix = "dev+"

	// shortRevisionLen is the number of leading hex characters kept from a
	// commit hash: enough to identify a commit by eye, short enough to read.
	shortRevisionLen = 12
)

// String returns the version of the running binary. It never returns an empty
// string, so callers can print it unconditionally.
func String() string {
	return format(Version, debug.ReadBuildInfo)
}

// format derives the version string from a stamped version and build metadata.
// The read function is a parameter rather than a direct call to
// debug.ReadBuildInfo so that every branch is reachable from a test; the build
// metadata of a test binary is not something a test can otherwise arrange.
func format(version string, read func() (*debug.BuildInfo, bool)) string {
	if version != "" {
		return version
	}

	info, ok := read()
	if !ok {
		return unknown
	}

	// A module installed by `go install module@version` carries its version
	// here. A build from a checkout reports "(devel)", which says nothing.
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}

	var revision string
	var modified bool
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}

	if revision == "" {
		return unknown
	}
	if len(revision) > shortRevisionLen {
		revision = revision[:shortRevisionLen]
	}

	// An uncommitted tree is flagged: a result produced by a dirty build cannot
	// be traced to any commit, and silently implying otherwise would be a lie
	// told by the part of the system whose job is provenance.
	if modified {
		return devPrefix + revision + ".dirty"
	}
	return devPrefix + revision
}
