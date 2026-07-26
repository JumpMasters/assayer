// Package arch enforces the internal package layout recorded in ADR-0005.
//
// It is a test-only package on purpose. It has no callers, nothing in the
// program should be able to import the thing that polices the program, and it
// must not import the packages it inspects — it reads them as source text.
package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// parsedFile is one Go source file, with the package directory it belongs to
// expressed relative to the module root (for example "internal/cli").
type parsedFile struct {
	Path   string // repo-relative, slash-separated
	Pkg    string // repo-relative package directory, slash-separated
	IsTest bool
	AST    *ast.File
}

// scanRoots are the directories the guard polices.
var scanRoots = []string{"internal", "cmd"}

// repoRoot walks up from the working directory to the directory holding
// go.mod. `go test` sets the working directory to the package under test, so
// this resolves identically under `go test ./internal/arch/` and `go test ./...`.
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

// modulePath reads the module line out of go.mod.
func modulePath(t *testing.T, root string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(rest)
		}
	}
	t.Fatal("no module line in go.mod")
	return ""
}

// walkFiles calls fn for every file under the scan roots, with the path
// relative to the module root. Directories named testdata are skipped:
// fixtures deliberately contain violations.
func walkFiles(t *testing.T, root string, fn func(rel string, d fs.DirEntry) error) {
	t.Helper()
	for _, sub := range scanRoots {
		base := filepath.Join(root, sub)
		if _, err := os.Stat(base); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "testdata" {
					return fs.SkipDir
				}
				return nil
			}
			rel, rerr := filepath.Rel(root, path)
			if rerr != nil {
				return rerr
			}
			return fn(filepath.ToSlash(rel), d)
		})
		if err != nil {
			t.Fatalf("walk %s: %v", base, err)
		}
	}
}

// goFiles parses every .go file under the scan roots.
//
// Build constraints are deliberately NOT evaluated: go/parser ignores
// //go:build, so a file excluded from the build is still inspected here. That
// is over-inclusive, which fails safe, and it is why this uses go/parser rather
// than go/packages — go/packages would have to build the tree this exists to
// police. testdata/buildignore.go.txt exists so that a future "cleanup" to
// go/packages fails this suite rather than silently narrowing it.
func goFiles(t *testing.T, root string) []parsedFile {
	t.Helper()
	fset := token.NewFileSet()
	var out []parsedFile

	walkFiles(t, root, func(rel string, _ fs.DirEntry) error {
		if !strings.HasSuffix(rel, ".go") {
			return nil
		}
		f, err := parser.ParseFile(fset, filepath.Join(root, rel), nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", rel, err)
		}
		out = append(out, parsedFile{
			Path:   rel,
			Pkg:    path0(rel),
			IsTest: strings.HasSuffix(rel, "_test.go"),
			AST:    f,
		})
		return nil
	})

	if len(out) == 0 {
		t.Fatal("walked no Go files — the guard is not pointed at the tree")
	}
	return out
}

// path0 returns the directory of a slash-separated relative path.
func path0(rel string) string {
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		return rel[:i]
	}
	return "."
}

// parseFixture parses a testdata/*.go.txt fixture.
//
// Fixtures are named .go.txt, never .go: `gofmt -s -l .` in make fmt-check and
// in CI descends into testdata/ even though go build, go vet, and golangci-lint
// all skip it. A .go fixture would fail the format gate, and `make fmt` would
// silently rewrite the ones that are meant to be malformed.
func parseFixture(t *testing.T, name string) *ast.File {
	t.Helper()
	if !strings.HasSuffix(name, ".go.txt") {
		t.Fatalf("fixture %s must be named *.go.txt", name)
	}
	path := filepath.Join("testdata", name)
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	f, err := parser.ParseFile(token.NewFileSet(), name, src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse fixture %s: %v", path, err)
	}
	return f
}

// isStdlib reports whether an import path is in the standard library. Stdlib
// paths have no dot in their first segment; every module path does.
func isStdlib(importPath string) bool {
	first, _, _ := strings.Cut(importPath, "/")
	return !strings.Contains(first, ".")
}
