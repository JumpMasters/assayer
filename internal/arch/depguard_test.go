package arch

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Sentinels bounding the generated depguard rules inside .golangci.yml.
const (
	depguardBegin = "      # depguard:begin — generated from internal/arch/classify_test.go. Do not edit by hand:"
	depguardNote  = "      # change the classification, run `go test ./internal/arch/`, and paste what it prints."
	depguardEnd   = "      # depguard:end"
)

// renderDepguard produces the depguard rules block from the classification, so
// the linter configuration and the architecture test cannot disagree.
//
// This RENDERS rather than parses. Parsing .golangci.yml would need a YAML
// library, and this module has no third-party dependencies — acquiring the
// first one to support a test is a poor trade, and the arch class's own
// "imports nothing" rule would then be violated by the test's own import.
// Rendering is also the stronger check: one source of truth, rather than set
// equality between two hand-maintained copies that can both be wrong together.
//
// list-mode is lax with a deny on this module's own prefix and allow
// exceptions. The alternatives do not work:
//
//   - `allow: []` is a hard configuration error ("must have an Allow and/or
//     Deny package list"), so a class permitting nothing is inexpressible as
//     an empty allow list.
//   - `list-mode: strict` denies everything not listed, including the standard
//     library, so a package allow-listed with its class row alone fails on
//     `import "fmt"`.
//   - `list-mode: lax` with only an allow list enforces nothing at all.
func renderDepguard(mod string) string {
	var b strings.Builder

	b.WriteString(depguardBegin + "\n")
	b.WriteString(depguardNote + "\n")
	b.WriteString("      rules:\n")

	pkgs := make([]string, 0, len(classification))
	for p := range classification {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)

	for _, pkg := range pkgs {
		cls := classification[pkg]

		fmt.Fprintf(&b, "        %s:\n", ruleName(pkg))
		b.WriteString("          list-mode: lax\n")

		// Adapters exclude test files from the broad rule. Overlapping depguard
		// rules are combined rather than overridden, so a narrower _test.go rule
		// cannot widen a broader one, and the legitimate conformance-kit import
		// would be rejected by the broad rule it sits underneath.
		if cls == classAdapter {
			fmt.Fprintf(&b, "          files: [\"**/%s/*.go\", \"!$test\"]\n", pkg)
		} else {
			fmt.Fprintf(&b, "          files: [\"**/%s/*.go\"]\n", pkg)
		}

		var qualified []string
		for _, a := range allowedInternal(cls, false) {
			if a == pkg {
				continue
			}
			qualified = append(qualified, mod+"/"+a)
		}
		sort.Strings(qualified)
		if len(qualified) > 0 {
			fmt.Fprintf(&b, "          allow: [%s]\n", strings.Join(qualified, ", "))
		}

		b.WriteString("          deny:\n")
		fmt.Fprintf(&b, "            - pkg: %s\n", mod)
		fmt.Fprintf(&b, "              desc: %q\n", describe(pkg, cls))
	}

	b.WriteString(depguardEnd)
	return b.String()
}

// ruleName turns a package directory into a depguard rule key.
func ruleName(pkg string) string {
	name := strings.TrimPrefix(pkg, "internal/")
	name = strings.ReplaceAll(name, "/", "-")
	return name
}

// describe is the message a developer sees when depguard rejects their import.
// It names the class and where the rule is written down, because an error that
// only says "not allowed" sends people to guess.
func describe(pkg string, cls class) string {
	allowed := allowedInternal(cls, false)
	if len(allowed) == 0 {
		return fmt.Sprintf("%s is class %s: it may import no other package in this module (ADR-0005)", pkg, cls)
	}
	return fmt.Sprintf("%s is class %s: within this module it may import only %s (ADR-0005)",
		pkg, cls, strings.Join(allowed, ", "))
}

// TestDepguardBlockMatchesClassification fails if .golangci.yml has been hand
// edited, or if the classification changed without the block being regenerated.
//
// On failure it prints the block to paste, so the fix is a copy rather than a
// hunt. A generator binary would be a non-test file in a package that must stay
// test-only, which is a worse trade than one paste per classification change.
func TestDepguardBlockMatchesClassification(t *testing.T) {
	root := repoRoot(t)
	mod := modulePath(t, root)
	want := renderDepguard(mod)

	raw, err := os.ReadFile(filepath.Join(root, ".golangci.yml"))
	if err != nil {
		t.Fatalf("read .golangci.yml: %v", err)
	}
	cfg := string(raw)

	start := strings.Index(cfg, depguardBegin)
	end := strings.Index(cfg, depguardEnd)
	if start < 0 || end < 0 || end < start {
		t.Fatalf("depguard sentinels not found in .golangci.yml.\n"+
			"Add this under linters.settings.depguard:\n\n%s\n", want)
	}

	if onDisk := cfg[start : end+len(depguardEnd)]; onDisk != want {
		t.Errorf("the depguard block in .golangci.yml does not match the classification.\n"+
			"Replace everything between the sentinels with:\n\n%s\n", want)
	}
}

// TestDepguardRulesCoverEveryClassifiedPackage guards against a rendered block
// that is syntactically fine and covers nothing — a rule whose files pattern
// matches no file is textually indistinguishable from a correct one.
func TestDepguardRulesCoverEveryClassifiedPackage(t *testing.T) {
	root := repoRoot(t)
	block := renderDepguard(modulePath(t, root))

	for pkg := range classification {
		if !strings.Contains(block, fmt.Sprintf("**/%s/*.go", pkg)) {
			t.Errorf("no depguard rule matches package %s", pkg)
		}
	}

	// Every rule's files pattern must match at least one real file, or the rule
	// is decoration.
	for pkg := range classification {
		found := false
		for _, f := range goFiles(t, root) {
			if f.Pkg == pkg {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("depguard rule for %s matches no file in the tree; "+
				"the classification should list only packages that exist", pkg)
		}
	}
}

// TestDepguardDenyIsTheModulePrefix pins the mechanism itself. A future edit
// that narrows the deny to something more specific would silently permit every
// internal edge, and every other test here would still pass.
func TestDepguardDenyIsTheModulePrefix(t *testing.T) {
	root := repoRoot(t)
	mod := modulePath(t, root)
	block := renderDepguard(mod)

	wantDeny := fmt.Sprintf("            - pkg: %s\n", mod)
	if !strings.Contains(block, wantDeny) {
		t.Errorf("rendered block does not deny the module prefix %q; "+
			"without that deny, lax mode enforces nothing", mod)
	}

	if strings.Contains(block, "list-mode: strict") {
		t.Error("strict list-mode denies the standard library; use lax with a module-prefix deny")
	}
}
