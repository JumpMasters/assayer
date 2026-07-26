package conformance

import (
	"context"
	"time"

	"github.com/JumpMasters/assayer/internal/assay"
	"github.com/JumpMasters/assayer/internal/port"
)

// Reference is a capture adapter for a harness that does not exist.
//
// Its purpose is to be shaped differently from the one harness this project has
// measured, so that the neutral representation is exercised by something that
// does not share its arrangement. Every awkwardness below is a real property of
// a coding agent named in the roadmap:
//
//   - no turn structure. The source is a flat event log; tool calls belong to
//     no turn, so they arrive in a single synthesised agent turn with no text.
//   - two consecutive agent turns with no human turn between them.
//   - the model changes part-way through the session.
//   - delegated work is a sibling with a pointer to its parent, so it is never
//     orphaned, and no tool call references it.
//   - tokens are reported, money is not.
//   - commands arrive already split into arguments.
//   - there is no version control and no patch.
//
// A poorer implementation would not have caught any of that. An implementation
// that merely leaves fields empty satisfies any shape, and shape is where a
// harness-specific assumption hides. This one has to be filled in differently,
// which is the only thing that puts pressure on the vocabulary.
//
// It is written by the same hand as the representation it tests, which limits
// what it can prove. It is a check, not a demonstration of neutrality; that
// arrives with an adapter for a format nobody here chose.
type Reference struct{}

// Compile-time assertion that the reference satisfies the port. This is the one
// thing a second implementation constrains for free: a method the reference
// cannot answer stops the build here rather than when a real adapter is written.
var _ port.Capture = Reference{}

// ID implements port.Capture.
func (Reference) ID() string { return "reference" }

// Tier implements port.Capture. The source is a wrapped command, which is the
// weakest of the three and therefore the most useful to model.
func (Reference) Tier() assay.Tier { return assay.TierWrapped }

// Capabilities implements port.Capture.
//
// Deliberately a strict subset: no cost, no lineage, no workspace patch, no
// mutation content, no reasoning. An adapter this limited must still produce a
// session every consumer can reason about.
func (Reference) Capabilities() assay.CapabilitySet {
	return assay.CapabilitySet(0).With(
		assay.CanSeeTurnText,
		assay.CanSeeToolCalls,
		assay.CanSeeToolInputs,
		assay.CanSeeToolResults,
		assay.CanSeeExitStatus,
		assay.CanSeeFileMutations,
		assay.CanSeeModelIdentity,
		assay.CanSeeTokens,
		assay.CanSeeTiming,
		assay.CanSeeWorkspace,
		assay.CanSeeDelegation,
	)
}

// referenceStart is the fixed instant the reference session is anchored to.
// A function rather than a package-level variable: the leaf's no-state rule is
// not in force here, but a shared mutable time is a bad idea regardless, and a
// fixed instant keeps the fixture reproducible.
func referenceStart() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }

// Discover implements port.Capture.
func (r Reference) Discover(_ context.Context, q Query) ([]port.Ref, error) {
	refs := []port.Ref{{
		ID:    "reference-session",
		Label: "a session from a harness with no turns",
		At:    referenceStart(),
	}}

	if !q.Since.IsZero() && refs[0].At.Before(q.Since) {
		return []port.Ref{}, nil
	}
	if q.Dir != "" && q.Dir != "/work" {
		return []port.Ref{}, nil
	}
	if q.Limit > 0 && q.Limit < len(refs) {
		refs = refs[:q.Limit]
	}
	return refs, nil
}

// Query is an alias so the reference reads naturally; it is port.Query.
type Query = port.Query

// Load implements port.Capture.
func (r Reference) Load(_ context.Context, ref port.Ref) (assay.Session, error) {
	if ref.ID != "reference-session" {
		return assay.Session{}, port.ErrNotFound
	}

	first := assay.Model{Alias: "small", Canonical: "small-1", Provider: "reference"}
	second := assay.Model{Alias: "large", Canonical: "large-1", Provider: "reference"}
	at := referenceStart()

	exit := 0
	toolTurn := assay.Turn{
		// No text: the source has no turn structure, so the calls are gathered
		// into a turn that never existed as such.
		Role:  assay.RoleAgent,
		Model: first,
		At:    at,
		Tools: []assay.ToolCall{{
			Name: "shell",
			Kind: assay.ToolExec,
			Argv: []string{"go", "test", "./..."},
			Result: &assay.ToolResult{
				Text:     "ok\n",
				Outcome:  assay.OutcomeOK,
				ExitCode: exit,
			},
		}, {
			Name:      "put",
			Kind:      assay.ToolMutate,
			Input:     "the body written to the file",
			Mutations: []assay.Mutation{{Path: "main.go", Kind: assay.MutationWrite}},
			Result:    &assay.ToolResult{Outcome: assay.OutcomeOK},
		}},
	}

	return assay.Session{
		// No lineage: the source has no session identity of its own.
		Workspace: assay.Workspace{Dirs: []string{"/work"}},
		Turns: []assay.Turn{
			{Role: assay.RoleHuman, Text: "do the thing", Model: first, At: at},
			toolTurn,
			// A second agent turn with no human turn between: the source emits
			// several assistant messages per ask.
			{Role: assay.RoleAgent, Text: "done", Model: second, At: at.Add(time.Minute)},
		},
		Usage: assay.Usage{
			// Tokens but no money: most harnesses report no price at all.
			InputTokens:  100,
			OutputTokens: 200,
			Wall:         time.Minute,
		},
		Delegated: []assay.Delegation{{
			// A sibling with its own identity. No tool call references it,
			// which on this harness is normal rather than a lost link.
			ID:    "reference-delegation",
			Turns: []assay.Turn{{Role: assay.RoleAgent, Text: "sub-task", Model: first, At: at}},
			Usage: assay.Usage{InputTokens: 10, OutputTokens: 20, Wall: time.Second},
		}},
		// Ordering is incomplete: delegated work cannot be placed among the
		// calls, so assertions about order must not be answered from this.
		OrderComplete: false,
		Fidelity: assay.Fidelity{
			Adapter:  r.ID(),
			Version:  "0",
			Tier:     r.Tier(),
			Observed: r.Capabilities(),
			Verified: true,
		},
	}, nil
}
