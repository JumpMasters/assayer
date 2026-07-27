// Package conformance holds the contract every capture adapter must satisfy,
// and a reference implementation used to keep that contract honest.
//
// It is not an adapter for any harness. It is the shared kit adapters are tested
// against, plus a deliberately awkward implementation whose only job is to be
// shaped differently from the harness this project measured first.
package conformance

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/JumpMasters/assayer/internal/assay"
)

// governed maps every observable leaf field of a session to the capability that
// must be declared before a caller may trust it.
//
// The key is the declaring type's name and the field name, not a full path, so
// that a type reached from several places is described once.
//
// This table is what makes a capability declaration binding rather than
// advisory. Without it, "an adapter must not report what it cannot see" is a
// sentence in a document; with it, the check below can be written, and a field
// added later without a decision about its observability fails rather than
// going quietly unchecked.
func governed(key string) (assay.Capability, bool) {
	switch key {
	case "Lineage.ID", "Lineage.Parent":
		return assay.CanSeeLineage, true

	case "Model.Alias", "Model.Canonical", "Model.Provider":
		return assay.CanSeeModelIdentity, true

	case "Workspace.Dirs", "Workspace.Revision", "Workspace.Branch", "Workspace.Origin",
		"Fingerprint.Name", "Fingerprint.Value":
		return assay.CanSeeWorkspace, true
	case "Workspace.Patch":
		return assay.CanSeeWorkspacePatch, true

	case "Turn.Role", "Turn.Text":
		return assay.CanSeeTurnText, true
	case "Turn.Reasoning":
		return assay.CanSeeReasoning, true
	case "Turn.At":
		return assay.CanSeeTiming, true

	case "ToolCall.Name", "ToolCall.Kind":
		return assay.CanSeeToolCalls, true
	case "ToolCall.Argv", "ToolCall.Input":
		return assay.CanSeeToolInputs, true
	case "ToolCall.DelegatedID", "Delegation.ID":
		return assay.CanSeeDelegation, true

	case "Mutation.Path", "Mutation.Kind":
		return assay.CanSeeFileMutations, true
	case "Mutation.Before", "Mutation.After":
		return assay.CanSeeMutationContent, true

	case "ToolResult.Text", "ToolResult.Truncated":
		return assay.CanSeeToolResults, true
	case "ToolResult.Outcome", "ToolResult.ExitCode":
		return assay.CanSeeToolOutcome, true

	case "Usage.CostMicroUSD":
		return assay.CanSeeCost, true
	case "Usage.InputTokens", "Usage.OutputTokens":
		return assay.CanSeeTokens, true
	case "Usage.Wall":
		return assay.CanSeeTiming, true
	}
	return 0, false
}

// exempt lists leaf fields that describe the capture itself rather than the
// session, and so cannot be governed by a capability without circularity: the
// declaration cannot be conditional on the declaration.
func exempt(key string) bool {
	switch key {
	case "Fidelity.Adapter", "Fidelity.Version", "Fidelity.Tier",
		"Fidelity.Observed", "Fidelity.Partial", "Fidelity.Verified",
		"Session.OrderComplete":
		return true
	}
	return false
}

// leafFields walks the session type and returns every observable leaf field, as
// "Type.Field" keys.
//
// A leaf is anything that is not a struct, a pointer to one, or a slice of
// them — with time.Time treated as a leaf, since it is a struct only
// incidentally. Container fields are not keyed: what matters is whether the
// values they eventually hold are governed.
func leafFields() []string {
	var out []string
	seen := map[reflect.Type]bool{}

	var walk func(t reflect.Type)
	walk = func(t reflect.Type) {
		for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct || seen[t] {
			return
		}
		seen[t] = true

		for i := range t.NumField() {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			key := t.Name() + "." + f.Name
			if isLeaf(f.Type) {
				out = append(out, key)
				continue
			}
			walk(f.Type)
		}
	}

	walk(reflect.TypeOf(assay.Session{}))
	sort.Strings(out)
	return out
}

func isLeaf(t reflect.Type) bool {
	if t == reflect.TypeOf(time.Time{}) {
		return true
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() == reflect.Slice {
		return isLeaf(t.Elem()) && t.Elem().Kind() != reflect.Struct
	}
	return t.Kind() != reflect.Struct
}

// CheckFieldCoverage reports fields of a session that no capability governs and
// that are not explicitly exempt.
//
// It is default-deny by construction: a new field is uncovered until someone
// decides what observing it means. That is the same posture the package layout
// takes towards a new package, and for the same reason — the moment a thing is
// added is the cheapest moment to decide what it implies.
func CheckFieldCoverage() error {
	var missing []string
	for _, key := range leafFields() {
		if exempt(key) {
			continue
		}
		if _, ok := governed(key); !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf(
		"no capability governs %s"+
			" -- add each to governed() in internal/adapter/conformance/coverage.go,"+
			" or to exempt() if it describes the capture rather than the session;"+
			" a field nobody decided about is a field where not-observed and"+
			" did-not-happen look identical",
		strings.Join(missing, ", "))
}

// zeroOutsideCapabilities reports fields carrying a value whose governing
// capability the adapter did not declare.
//
// An adapter that reports what it cannot see is worse than one that reports
// nothing: a caller trusting the declaration treats the stray value as evidence.
func zeroOutsideCapabilities(s *assay.Session) []string {
	var bad []string

	var walk func(v reflect.Value, typeName string)
	walk = func(v reflect.Value, typeName string) {
		switch v.Kind() {
		case reflect.Pointer:
			if !v.IsNil() {
				walk(v.Elem(), typeName)
			}
			return
		case reflect.Slice:
			for i := range v.Len() {
				walk(v.Index(i), typeName)
			}
			return
		case reflect.Struct:
			if v.Type() == reflect.TypeOf(time.Time{}) {
				return
			}
		default:
			return
		}

		t := v.Type()
		for i := range t.NumField() {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			key := t.Name() + "." + f.Name
			fv := v.Field(i)

			if isLeaf(f.Type) {
				if exempt(key) || fv.IsZero() {
					continue
				}
				c, ok := governed(key)
				if !ok {
					continue // reported by CheckFieldCoverage
				}
				if !s.Fidelity.Observed.Has(c) {
					bad = append(bad, fmt.Sprintf("%s is set but %s is not declared", key, c))
				}
				continue
			}
			walk(fv, f.Name)
		}
	}

	// Fidelity describes the capture and is exempt from its own rule.
	probe := *s
	probe.Fidelity = assay.Fidelity{}
	walk(reflect.ValueOf(probe), "Session")

	sort.Strings(bad)
	return bad
}
