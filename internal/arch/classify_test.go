package arch

import (
	"sort"
	"strings"
	"testing"
)

// class is a package's role in the star recorded in ADR-0005. What a package
// may import follows from its class, never from a row of its own: a row per
// package would make widening the star a thirty-second append that satisfies
// every check, while a class means widening is an edit to allowedInternal,
// which reads like a decision.
type class int

const (
	classLeaf class = iota
	classPorts
	classComponent
	classAdapter
	classRootApp
	classRootCLI
	classStandalone
	classCmd
)

func (c class) String() string {
	switch c {
	case classLeaf:
		return "leaf"
	case classPorts:
		return "ports"
	case classComponent:
		return "component"
	case classAdapter:
		return "adapter"
	case classRootApp:
		return "root(app)"
	case classRootCLI:
		return "root(cli)"
	case classStandalone:
		return "standalone"
	case classCmd:
		return "cmd"
	}
	return "unknown"
}

// classification assigns every package the guard walks to a class.
//
// It lists only packages that exist. A row for a package nobody has written
// constrains nothing, cannot be checked against reality, and is silently wrong
// if the shape changes before the package arrives; adding a package later is a
// one-line diff at exactly the moment its dependencies are cheapest to think
// about.
//
// This map is default-deny: a package that appears in the tree without a row
// fails TestEveryPackageIsClassified. That is the half depguard cannot do,
// since depguard permits by default any file no rule matches.
//
// The full intended shape of the graph is in docs/adr/0005-internal-package-layout.md.
var classification = map[string]class{
	"internal/buildinfo": classStandalone,
	"internal/arch":      classStandalone,
	"internal/assay":     classLeaf,
	"internal/port":      classPorts,
	"internal/cli":       classRootCLI,
	"cmd/assayer":        classCmd,

	"internal/adapter/conformance":        classAdapter,
	"internal/adapter/claudecode/capture": classAdapter,
	"internal/adapter/claudecode/run":     classAdapter,
}

// allowedInternal returns the in-module packages a class may import.
//
// classRootApp is special: the composition root may import anything classified,
// because the star forbids components from calling each other and so guarantees
// the root assembles everything.
func allowedInternal(c class, isTest bool) []string {
	switch c {
	case classLeaf, classStandalone:
		return nil
	case classPorts:
		return []string{"internal/assay"}
	case classComponent:
		return []string{"internal/assay", "internal/port"}
	case classAdapter:
		// internal/adapter/shared is described in ADR-0005 as the home for code
		// common to several adapters. It is not listed here because it does not
		// exist: an allowance for a package nobody has written cannot be checked
		// against anything, and is added in the change that creates it.
		allowed := []string{"internal/assay", "internal/port"}
		if isTest {
			allowed = append(allowed, "internal/adapter/conformance")
		}
		sort.Strings(allowed)
		return allowed
	case classRootApp:
		all := make([]string, 0, len(classification))
		for pkg := range classification {
			if strings.HasPrefix(pkg, "internal/") {
				all = append(all, pkg)
			}
		}
		sort.Strings(all)
		return all
	case classRootCLI:
		return []string{"internal/app", "internal/assay", "internal/buildinfo"}
	case classCmd:
		return []string{"internal/cli"}
	}
	return nil
}

// TestEveryPackageIsClassified is the default-deny half of the enforcement.
//
// depguard evaluates the rules someone wrote and silently permits any file no
// rule matches, so a package added in a hurry passes it unremarked. Here it
// fails, and the failure names the package and what to do about it.
func TestEveryPackageIsClassified(t *testing.T) {
	root := repoRoot(t)

	seen := map[string]bool{}
	for _, f := range goFiles(t, root) {
		seen[f.Pkg] = true
	}

	pkgs := make([]string, 0, len(seen))
	for pkg := range seen {
		pkgs = append(pkgs, pkg)
	}
	sort.Strings(pkgs)

	for _, pkg := range pkgs {
		if _, ok := classification[pkg]; !ok {
			t.Errorf("package %s is not classified.\n"+
				"  Add it to classification in internal/arch/classify_test.go, then run\n"+
				"  `go test ./internal/arch/` again to regenerate the depguard block.\n"+
				"  Pick its class from the table in docs/adr/0005-internal-package-layout.md;\n"+
				"  what it may import follows from the class, not from the row.", pkg)
		}
	}
}

// TestImportEdges checks every import — internal and third-party — against the
// allowance for the importing package's class.
//
// Third-party imports are checked too. Core importing a vendor's client library
// is caught by the vocabulary scan only when the module path happens to contain
// a listed word, which is true for one vendor and false for the rest.
func TestImportEdges(t *testing.T) {
	root := repoRoot(t)
	mod := modulePath(t, root)

	// Third-party imports permitted anywhere. Deliberately empty: this module
	// has no dependencies, and acquiring the first one should be a decision
	// somebody makes on purpose rather than a diff that slips through.
	allowedExternal := map[string]bool{}

	for _, f := range goFiles(t, root) {
		cls, ok := classification[f.Pkg]
		if !ok {
			continue // reported by TestEveryPackageIsClassified
		}

		allowed := map[string]bool{}
		for _, a := range allowedInternal(cls, f.IsTest) {
			allowed[a] = true
		}

		for _, spec := range f.AST.Imports {
			importPath := strings.Trim(spec.Path.Value, `"`)
			switch {
			case isStdlib(importPath):
				continue
			case importPath == mod || strings.HasPrefix(importPath, mod+"/"):
				rel := strings.TrimPrefix(strings.TrimPrefix(importPath, mod), "/")
				if rel == f.Pkg {
					continue
				}
				if !allowed[rel] {
					t.Errorf("%s: package %s (class %s) may not import %s.\n"+
						"  Permitted: %v\n"+
						"  Widening this means editing allowedInternal, not adding a row.",
						f.Path, f.Pkg, cls, rel, allowedInternal(cls, f.IsTest))
				}
			default:
				if !allowedExternal[importPath] {
					t.Errorf("%s: package %s imports third-party %s, which is not in allowedExternal.\n"+
						"  This module has no dependencies; adding one is a deliberate decision.",
						f.Path, f.Pkg, importPath)
				}
			}
		}
	}
}

// TestImportEdgeRuleRejectsFixtures runs the same allowance logic over fixtures,
// so each rule has a test of its own rather than resting on the real tree
// happening to be clean today.
func TestImportEdgeRuleRejectsFixtures(t *testing.T) {
	tests := []struct {
		fixture string
		cls     class
		wantBad []string
	}{
		{
			fixture: "badedge.go.txt",
			cls:     classComponent,
			wantBad: []string{"internal/ledger"},
		},
		{
			// go/parser does not evaluate //go:build, so this file is still
			// inspected. If someone "cleans up" the walker to use go/packages,
			// this case stops failing — which is exactly what it is here to catch.
			fixture: "buildignore.go.txt",
			cls:     classComponent,
			wantBad: []string{"internal/ledger"},
		},
	}

	root := repoRoot(t)
	mod := modulePath(t, root)

	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			f := parseFixture(t, tt.fixture)

			allowed := map[string]bool{}
			for _, a := range allowedInternal(tt.cls, false) {
				allowed[a] = true
			}

			var bad []string
			for _, spec := range f.Imports {
				importPath := strings.Trim(spec.Path.Value, `"`)
				if !strings.HasPrefix(importPath, mod+"/") {
					continue
				}
				rel := strings.TrimPrefix(importPath, mod+"/")
				if !allowed[rel] {
					bad = append(bad, rel)
				}
			}

			if len(bad) != len(tt.wantBad) {
				t.Fatalf("rejected %v, want %v", bad, tt.wantBad)
			}
			for i := range bad {
				if bad[i] != tt.wantBad[i] {
					t.Errorf("rejected %q, want %q", bad[i], tt.wantBad[i])
				}
			}
		})
	}
}

// TestAdapterTestsMayImportConformanceKit pins the one test-scoped relaxation.
// Every other rule applies to _test.go files everywhere: a core package that
// couples to one harness in its tests has failed the neutrality property in the
// most expensive place to find out.
func TestAdapterTestsMayImportConformanceKit(t *testing.T) {
	inProduction := map[string]bool{}
	for _, a := range allowedInternal(classAdapter, false) {
		inProduction[a] = true
	}
	if inProduction["internal/adapter/conformance"] {
		t.Error("adapters may import the conformance kit from non-test files; it is a test-only kit")
	}

	inTest := map[string]bool{}
	for _, a := range allowedInternal(classAdapter, true) {
		inTest[a] = true
	}
	if !inTest["internal/adapter/conformance"] {
		t.Error("adapter tests must be able to import the conformance kit")
	}

	// The relaxation is exactly one package wide.
	if len(inTest)-len(inProduction) != 1 {
		t.Errorf("test relaxation widened by %d packages, want exactly 1",
			len(inTest)-len(inProduction))
	}
}
