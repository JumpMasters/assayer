package arch

import (
	"fmt"
	"go/ast"
	"strings"
	"testing"
)

// leafPackages may hold types and constants and nothing else.
var leafPackages = []string{"internal/assay", "internal/port"}

// bannedLeafFieldTypes are the escape hatches that would let a harness-specific
// payload cross the neutral boundary without ever naming a vendor.
var bannedLeafFieldTypes = []string{"any", "interface{}", "map[string]any", "json.RawMessage"}

// adapterRegistryFile is the one file allowed to name a vendor outside the
// adapter tree, and therefore the one file not allowed to make decisions.
const adapterRegistryFile = "internal/app/adapters.go"

// leafViolations reports package-level state, init functions, and exported
// fields typed as an escape hatch.
//
// Package-level var plus init is how an idiomatic Go registry gets built — a
// port.Register map populated from each adapter's init, in the style of
// database/sql. Every edge in that pattern is legal and it names no vendor, and
// it makes the set of known harnesses mutable at initialisation time, which is
// exactly the coupling the star was drawn to prevent.
//
// This closes the generic form of a harness-shaped neutral representation. It
// does not close the named form: a field called SidechainUUID uses no vendor
// word and crosses no illegal edge, and no cheap check can tell it from a
// legitimate one. That is governed by the record specifying the representation,
// by review, and by the stub harness in the adapter conformance kit — a second,
// deliberately impoverished implementation, so an interface only one harness
// can satisfy fails at build time rather than when the second adapter is tried.
func leafViolations(f *ast.File) []string {
	var out []string

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil && d.Name.Name == "init" {
				out = append(out, "init function")
			}
		case *ast.GenDecl:
			if d.Tok.String() == "var" {
				out = append(out, "package-level var")
			}
		}
	}

	ast.Inspect(f, func(n ast.Node) bool {
		st, ok := n.(*ast.StructType)
		if !ok || st.Fields == nil {
			return true
		}
		for _, field := range st.Fields.List {
			exported := len(field.Names) == 0 // embedded fields are exported by their type
			for _, name := range field.Names {
				if name.IsExported() {
					exported = true
				}
			}
			if !exported {
				continue
			}
			typ := typeString(field.Type)
			for _, banned := range bannedLeafFieldTypes {
				if typ == banned {
					out = append(out, fmt.Sprintf("exported field typed %s", banned))
				}
			}
		}
		return true
	})

	return out
}

// typeString renders a type expression well enough to compare against
// bannedLeafFieldTypes. It is deliberately shallow, because those types are.
func typeString(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.InterfaceType:
		if t.Methods == nil || len(t.Methods.List) == 0 {
			return "interface{}"
		}
		return "interface{...}"
	case *ast.MapType:
		return "map[" + typeString(t.Key) + "]" + typeString(t.Value)
	case *ast.SelectorExpr:
		return typeString(t.X) + "." + t.Sel.Name
	case *ast.StarExpr:
		return "*" + typeString(t.X)
	case *ast.ArrayType:
		return "[]" + typeString(t.Elt)
	}
	return ""
}

// controlFlowViolations reports branching.
func controlFlowViolations(f *ast.File) []string {
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.IfStmt:
			out = append(out, "if")
		case *ast.SwitchStmt:
			out = append(out, "switch")
		case *ast.TypeSwitchStmt:
			out = append(out, "type switch")
		case *ast.ForStmt:
			out = append(out, "for")
		case *ast.RangeStmt:
			out = append(out, "range")
		}
		return true
	})
	return out
}

// TestLeafPackagesHoldNoStateOrEscapeHatches enforces the structural half of
// leaf neutrality on internal/assay and internal/port.
//
// Neither package exists yet. That is deliberate: the check is in place before
// the code it constrains, which is the reason this record comes first.
func TestLeafPackagesHoldNoStateOrEscapeHatches(t *testing.T) {
	root := repoRoot(t)

	isLeaf := map[string]bool{}
	for _, leaf := range leafPackages {
		isLeaf[leaf] = true
	}

	for _, f := range goFiles(t, root) {
		if f.IsTest || !isLeaf[f.Pkg] {
			continue
		}
		for _, v := range leafViolations(f.AST) {
			t.Errorf("%s: %s is not permitted in a leaf package.\n"+
				"  Leaf packages hold types and constants. Package-level state plus an\n"+
				"  adapter init is a registry, which makes the harness set mutable at\n"+
				"  startup; an any-typed exported field is a channel for harness-specific\n"+
				"  payload. See docs/adr/0005-internal-package-layout.md.", f.Path, v)
		}
	}
}

// TestAdapterRegistryHasNoControlFlow keeps the vocabulary exemption honest.
//
// internal/app/adapters.go is the one file outside the adapter tree where a
// vendor name is legal. Without this, "harness-specific logic elsewhere in app
// still fails" would be untrue inside precisely the file where it matters most.
func TestAdapterRegistryHasNoControlFlow(t *testing.T) {
	root := repoRoot(t)

	for _, f := range goFiles(t, root) {
		if f.Path != adapterRegistryFile {
			continue
		}
		for _, v := range controlFlowViolations(f.AST) {
			t.Errorf("%s: %q is not permitted.\n"+
				"  This file is a registration table, and it is the only file outside\n"+
				"  internal/adapter/** allowed to name a vendor. It may hold names; it\n"+
				"  may not hold decisions.", f.Path, v)
		}
	}
}

func TestLeafRulesRejectFixtures(t *testing.T) {
	tests := []struct {
		fixture string
		want    string
		why     string
	}{
		{"leafvar.go.txt", "package-level var", "a registry map populated by adapter init functions"},
		{"leafinit.go.txt", "init function", "an initialisation side effect in the leaf"},
		{"leafany.go.txt", "exported field typed map[string]any", "a generic channel for harness-specific payload"},
	}

	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			got := leafViolations(parseFixture(t, tt.fixture))
			for _, g := range got {
				if strings.Contains(g, tt.want) {
					return
				}
			}
			t.Errorf("leafViolations(%s) = %v, want it to contain %q — the fixture is %s",
				tt.fixture, got, tt.want, tt.why)
		})
	}
}

// TestLeafRuleAcceptsCleanFixture guards against the rule being trivially true.
// A leaf full of types and constants must pass.
func TestLeafRuleAcceptsCleanFixture(t *testing.T) {
	if got := leafViolations(parseFixture(t, "leafclean.go.txt")); len(got) != 0 {
		t.Errorf("leafViolations(leafclean.go.txt) = %v, want none", got)
	}
}

func TestControlFlowRuleRejectsFixture(t *testing.T) {
	if got := controlFlowViolations(parseFixture(t, "registrybranch.go.txt")); len(got) == 0 {
		t.Error("controlFlowViolations(registrybranch.go.txt) = none, want at least one")
	}
}

// TestControlFlowRuleAcceptsATable pins that a real registration table passes,
// so the rule constrains logic rather than the file's existence.
func TestControlFlowRuleAcceptsATable(t *testing.T) {
	if got := controlFlowViolations(parseFixture(t, "registrytable.go.txt")); len(got) != 0 {
		t.Errorf("controlFlowViolations(registrytable.go.txt) = %v, want none", got)
	}
}
