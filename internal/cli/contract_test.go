package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// renderSchema produces a JSON Schema from a Go type.
//
// Generated, not hand-written. A hand-written schema beside a Go struct is two
// sources of truth that can be wrong together: a field renamed in the struct
// regenerates the golden and still validates, because a schema that does not
// forbid extra properties accepts the new name and does not miss the old one.
// Deriving the schema from the type makes that impossible by construction, and
// it is the same move this repository already makes for its linter rules, which
// are rendered from the package classification rather than compared against it.
//
// It also needs no dependency. A schema validator would be this module's first,
// pulled in to check a property the type system can express directly.
func renderSchema(t *testing.T, id, title string, typ reflect.Type) string {
	t.Helper()
	if typ.Kind() != reflect.Struct {
		t.Fatalf("renderSchema: %s is not a struct", typ)
	}

	var required []string
	props := map[string]any{}

	for i := range typ.NumField() {
		f := typ.Field(i)
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		if name == "" || name == "-" {
			t.Fatalf("field %s.%s has no json tag; every emitted field needs a stable name",
				typ.Name(), f.Name)
		}
		required = append(required, name)
		props[name] = jsonSchemaFor(t, f.Type)
	}

	schema := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$id":     id,
		"title":   title,
		"type":    "object",
		// Both of these are what make the schema a real check. Without
		// additionalProperties an added field passes silently; without a
		// complete required list a removed one does.
		"additionalProperties": false,
		"required":             required,
		"properties":           props,
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(schema); err != nil {
		t.Fatalf("encode schema: %v", err)
	}
	return buf.String()
}

func jsonSchemaFor(t *testing.T, typ reflect.Type) map[string]any {
	t.Helper()
	switch typ.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int64:
		return map[string]any{"type": "integer"}
	case reflect.Slice:
		return map[string]any{"type": "array", "items": jsonSchemaFor(t, typ.Elem())}
	}
	t.Fatalf("no JSON Schema mapping for %s; add one before emitting a field of that type", typ)
	return nil
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found walking up from the working directory")
		}
		dir = parent
	}
}

// TestVersionSchemaMatchesTheType fails when the committed schema and the type
// that produces documents have drifted apart. On failure it prints the schema
// to write, so the fix is a copy rather than a hunt.
func TestVersionSchemaMatchesTheType(t *testing.T) {
	want := renderSchema(t, versionSchema, "assayer version document",
		reflect.TypeOf(versionDocument{}))

	path := filepath.Join(repoRoot(t), "schemas", "version", "v0.json")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v\n\nWrite this file:\n\n%s", path, err, want)
	}
	if string(got) != want {
		t.Errorf("schemas/version/v0.json does not match versionDocument.\n"+
			"Replace it with:\n\n%s", want)
	}
}

// TestEveryCommittedSchemaIsProduced stops a schema for a document nobody emits
// from rotting unnoticed in a directory consumers are told to trust.
func TestEveryCommittedSchemaIsProduced(t *testing.T) {
	root := filepath.Join(repoRoot(t), "schemas")

	emitted := map[string]bool{}
	for _, id := range newVersionDocument("x").Emits {
		emitted[id] = true
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read schemas/: %v", err)
	}
	for _, kind := range entries {
		if !kind.IsDir() {
			continue
		}
		versions, err := os.ReadDir(filepath.Join(root, kind.Name()))
		if err != nil {
			t.Fatalf("read schemas/%s: %v", kind.Name(), err)
		}
		for _, v := range versions {
			id := fmt.Sprintf("%s/%s/%s", schemaBase, kind.Name(),
				strings.TrimSuffix(v.Name(), ".json"))
			if !emitted[id] {
				t.Errorf("schemas/%s/%s is committed but no document declares it in Emits",
					kind.Name(), v.Name())
			}
		}
	}
}

// TestVersionDocumentGolden pins the emitted bytes. The build token is injected
// so the document is identical everywhere; the real binary's identity changes
// with every commit and could not be compared against anything fixed.
func TestVersionDocumentGolden(t *testing.T) {
	var buf bytes.Buffer
	if err := writeVersion(&buf, true, "v9.9.9-test"); err != nil {
		t.Fatalf("writeVersion: %v", err)
	}

	const want = `{"schema":"https://github.com/JumpMasters/assayer/schemas/version/v0",` +
		`"revision":1,"stability":"experimental",` +
		`"emits":["https://github.com/JumpMasters/assayer/schemas/version/v0"],` +
		`"build":"v9.9.9-test"}` + "\n"

	if buf.String() != want {
		t.Errorf("version document changed.\n got: %s\nwant: %s", buf.String(), want)
	}
}

// TestMachineOutputIsCompactAndNewlineTerminated pins the framing consumers
// depend on: one object, one line, one trailing newline. A stream of documents
// is only line-splittable if this holds.
func TestMachineOutputIsCompactAndNewlineTerminated(t *testing.T) {
	var buf bytes.Buffer
	if err := writeVersion(&buf, true, "v1"); err != nil {
		t.Fatalf("writeVersion: %v", err)
	}
	out := buf.String()

	if !strings.HasSuffix(out, "\n") {
		t.Error("machine output does not end with a newline")
	}
	if strings.Count(out, "\n") != 1 {
		t.Errorf("machine output spans %d lines; it must be one object on one line",
			strings.Count(out, "\n"))
	}
	if strings.Contains(out, "  ") {
		t.Error("machine output is indented; it must be compact")
	}
}

// TestMachineOutputDoesNotEscapeHTML pins the encoder setting. Go escapes <, >
// and & by default, which would corrupt any document carrying a shell command
// or a diff, and those documents are coming.
func TestMachineOutputDoesNotEscapeHTML(t *testing.T) {
	var buf bytes.Buffer
	if err := encodeDocument(&buf, map[string]string{"cmd": "go test ./... && echo <ok>"}); err != nil {
		t.Fatalf("encodeDocument: %v", err)
	}
	out := buf.String()
	for _, escaped := range []string{`\u0026`, `\u003c`, `\u003e`} {
		if strings.Contains(out, escaped) {
			t.Errorf("machine output contains the escape %s: %s", escaped, out)
		}
	}
	if !strings.Contains(out, "&& echo <ok>") {
		t.Errorf("machine output did not preserve the literal characters: %s", out)
	}
}

// TestVersionDocumentIsValidJSON guards against the golden being pinned to
// something that is not actually parseable.
func TestVersionDocumentIsValidJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := writeVersion(&buf, true, "v1"); err != nil {
		t.Fatalf("writeVersion: %v", err)
	}
	var doc versionDocument
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("emitted document does not parse: %v", err)
	}
	if doc.Schema != versionSchema {
		t.Errorf("document schema = %q, want %q", doc.Schema, versionSchema)
	}
	if doc.Stability != stabilityExperimental {
		t.Errorf("document stability = %q, want %q", doc.Stability, stabilityExperimental)
	}
}

// TestSchemaIdentifierIsNotADomainWeDoNotOwn pins the namespace to this
// repository. An earlier draft rooted it at assayer.dev on the assumption that
// nothing was served there; that domain is registered and serving an unrelated
// product of the same name, so every emitted document would have pointed
// consumers at a third party.
func TestSchemaIdentifierIsNotADomainWeDoNotOwn(t *testing.T) {
	if !strings.HasPrefix(schemaBase, "https://github.com/JumpMasters/assayer/") {
		t.Errorf("schemaBase = %q, want a namespace this project controls", schemaBase)
	}
	if strings.Contains(schemaBase, "assayer.dev") {
		t.Error("schemaBase points at assayer.dev, which is someone else's product")
	}
}
