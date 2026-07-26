package cli

import (
	"encoding/json"
	"io"

	"github.com/JumpMasters/assayer/internal/buildinfo"
)

// Machine-readable output carries its own identity, so a consumer can tell what
// it is holding without knowing which command produced it.
//
// The schema identifier is rooted at this repository rather than at a domain.
// An earlier draft used assayer.dev, on the stated assumption that nothing was
// served there; that domain is in fact registered and serving an unrelated
// product of the same name, so documents would have carried a namespace this
// project does not control and cannot publish to.
const (
	schemaBase = "https://github.com/JumpMasters/assayer/schemas"

	// versionSchema names the document kind and its major version.
	versionSchema = schemaBase + "/version/v0"

	// stabilityExperimental means the shape may still break, and every break
	// ships a way to migrate. It becomes additive-only at the first release.
	stabilityExperimental = "experimental"
)

// documentRevision is incremented on every change to an emitted shape, within
// a major version.
//
// The identifier alone cannot carry this. While the contract is experimental
// its shape may break without the major version moving, so a consumer matching
// only the identifier cannot tell one v0 from another and gets a parse error
// where it wanted a clean rejection. A monotonic revision beside the identifier
// is what lets a consumer ask whether the field it needs is present. Tools that
// do this well carry both — an identifier for the kind and a separate number
// for the revision — and the ones that carry only an identifier make their
// consumers pin a tool version instead.
const documentRevision = 1

// versionDocument is what `assayer version --json` emits.
type versionDocument struct {
	Schema    string `json:"schema"`
	Revision  int    `json:"revision"`
	Stability string `json:"stability"`

	// Emits lists every schema this build can produce, so a consumer can find
	// out what it is talking to without running a command that costs money.
	Emits []string `json:"emits"`

	// Build identifies the binary. It is one opaque token rather than separate
	// version, commit and dirty fields, because the three cases the build
	// carries — a stamped release, a development build with a revision, and a
	// binary built outside a repository that knows neither — do not decompose
	// into a string and a boolean without the boolean claiming something false
	// in the third case. Two runs came from the same build exactly when this
	// token matches.
	Build string `json:"build"`
}

func newVersionDocument(build string) versionDocument {
	return versionDocument{
		Schema:    versionSchema,
		Revision:  documentRevision,
		Stability: stabilityExperimental,
		Emits:     []string{versionSchema},
		Build:     build,
	}
}

// encodeDocument writes a machine-readable document.
//
// Compact, exactly one trailing newline, and HTML escaping off. Go's encoder
// escapes <, > and & by default, which is harmless in a version document and
// actively damaging in anything carrying a shell command or a diff — a
// consumer would receive `go test ./... && ...` and either display it
// wrong or unescape it a second time. Turning it off here rather than when the
// first such document arrives keeps every document consistent from the start.
func encodeDocument(w io.Writer, doc any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(doc)
}

// writeVersion emits the version document, machine-readable or not.
func writeVersion(w io.Writer, asJSON bool, build string) error {
	if !asJSON {
		_, err := io.WriteString(w, build+"\n")
		return err
	}
	return encodeDocument(w, newVersionDocument(build))
}

// currentBuild is the running binary's identity.
func currentBuild() string { return buildinfo.String() }
