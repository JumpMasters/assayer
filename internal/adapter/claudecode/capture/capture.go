// Package capture reads Claude Code's own session transcripts and translates
// them into the neutral representation.
//
// This is one of the two places in the program allowed to know a harness
// exists; everything downstream works from what this produces without knowing
// where it came from.
//
// What it can observe is measured rather than assumed. A survey of the local
// store found timestamps, working directories and branches, per-message model
// identity and token counts, tool calls with their inputs, and tool results —
// and found no cost anywhere, in any record, in any version. So this adapter
// does not declare that it can see money, and a caller asking for a spending
// comparison gets an honest refusal rather than a zero.
package capture

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/JumpMasters/assayer/internal/assay"
	"github.com/JumpMasters/assayer/internal/port"
)

// Adapter reads transcripts from a Claude Code store.
type Adapter struct {
	// Root is the directory holding per-project transcript directories. Empty
	// means the default location under the user's home directory.
	Root string
}

var _ port.Capture = Adapter{}

// ID implements port.Capture.
func (Adapter) ID() string { return "claude-code" }

// Tier implements port.Capture. Transcripts are the harness's own record of
// what it did, which is the strongest capture this project has.
func (Adapter) Tier() assay.Tier { return assay.TierNative }

// Capabilities implements port.Capture.
//
// Four absences are deliberate and each was checked against the store rather
// than assumed:
//
//   - cost. No record in any sampled version carries a price. The harness
//     reports one when it finishes a run, but a transcript is not that report,
//     and deriving a price from token counts and a table would be presenting a
//     calculation as an observation.
//   - the workspace patch. A transcript records what the agent did, not the
//     state of the tree before it started.
//   - file mutations and their content. The store does carry a file-history
//     stream, which an earlier survey classified as bookkeeping and skipped;
//     reading it is worthwhile and is not done here, so the capability is not
//     claimed.
func (Adapter) Capabilities() assay.CapabilitySet {
	return assay.CapabilitySet(0).With(
		assay.CanSeeLineage,
		assay.CanSeeModelIdentity,
		assay.CanSeeWorkspace,
		assay.CanSeeToolCalls,
		assay.CanSeeToolInputs,
		assay.CanSeeToolResults,
		assay.CanSeeToolOutcome,
		assay.CanSeeTurnText,
		assay.CanSeeReasoning,
		assay.CanSeeTokens,
		assay.CanSeeTiming,
		assay.CanSeeDelegation,
	)
}

// partialCapabilities are observed but known to be incomplete for every session
// this adapter produces.
//
// Model identity is partial because a transcript records the model that served
// a message and never the alias that was asked for, so a silent remapping of an
// alias is invisible from here alone.
//
// Tool results are partial because large ones are written elsewhere and
// shortened in place, so a result body may be a prefix of what the tool
// actually returned.
//
// The workspace is partial because the capability governs six fields and this
// adapter fills two. A transcript records the working directory and the branch;
// it never records the revision, the origin, or a toolchain fingerprint.
// Declared whole, an assertion that a replay ran on the same revision would
// compare a recorded SHA against an empty string and return a failure where the
// honest answer is an error. Reading the three from the repository at load time
// is possible and is not the same measurement: it would report today's state,
// not the session's.
//
// Delegation is partial for a reason worth stating plainly, because it was
// nearly an overclaim. Delegated work is recorded two ways: inline in the
// session's own transcript, which this adapter reads, and offloaded into
// separate files beside it, which it does not. Scanning 601 project-root
// transcripts found no inline delegation at all — every one of them keeps it in
// the files this adapter skips. So the capability is declared, because the
// inline form is read, and marked partial, because on the store as it exists
// today the answer is always empty, and because what is read is carried as one
// delegation with no identifier: nothing in a transcript separates two
// sub-agent runs or links either to the call that spawned it, so
// Delegation.ID and ToolCall.DelegatedID stay empty. An assertion about
// delegated work that would fail on this evidence resolves to an error instead,
// which is the right answer: nothing was seen because nothing was looked at.
func partialCapabilities() assay.CapabilitySet {
	return assay.CapabilitySet(0).With(
		assay.CanSeeModelIdentity,
		assay.CanSeeToolResults,
		assay.CanSeeWorkspace,
		assay.CanSeeDelegation,
	)
}

func (a Adapter) root() (string, error) {
	if a.Root != "" {
		return a.Root, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating the transcript store: %w", err)
	}
	return filepath.Join(home, ".claude", "projects"), nil
}

// Discover implements port.Capture.
//
// Only transcripts sitting directly in a project directory are listed. Anything
// deeper is a record of delegated work, which belongs to the session that
// spawned it rather than standing on its own.
//
// The label is built from the path rather than from the session's contents, so
// that listing does not mean opening every file. One local store held 2,485 of
// them.
func (a Adapter) Discover(ctx context.Context, q port.Query) ([]port.Ref, error) {
	root, err := a.root()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return []port.Ref{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading the transcript store: %w", err)
	}

	refs := []port.Ref{}
	for _, project := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !project.IsDir() {
			continue
		}
		dir := filepath.Join(root, project.Name())
		files, err := os.ReadDir(dir)
		if err != nil {
			continue // a project directory we cannot read is not a fatal condition
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
				continue
			}
			info, err := f.Info()
			if err != nil {
				continue
			}
			stem := strings.TrimSuffix(f.Name(), ".jsonl")
			ref := port.Ref{
				ID:    filepath.Join(dir, f.Name()),
				Label: project.Name() + "/" + stem,
				At:    info.ModTime(),
			}
			if !q.Since.IsZero() && ref.At.Before(q.Since) {
				continue
			}
			// The store names a transcript for the session it holds, so this
			// narrowing costs a string comparison rather than a file opened. It
			// is a measured property, not a documented one: across 400 sampled
			// transcripts the first record's session identifier equalled the
			// file's name every time, and was absent from none.
			if q.Native != "" && stem != q.Native {
				continue
			}
			// Equality, not a substring test: Dir names a directory, and
			// containment made a query for /work match a project named for
			// /workspace/other.
			if q.Dir != "" && project.Name() != encodeProjectDir(q.Dir) {
				continue
			}
			refs = append(refs, ref)
		}
	}

	sort.Slice(refs, func(i, j int) bool { return refs[i].At.After(refs[j].At) })
	if q.Limit > 0 && len(refs) > q.Limit {
		refs = refs[:q.Limit]
	}
	return refs, nil
}

// encodeProjectDir renders a filesystem path the way the store names the
// directory it keeps that project's transcripts in.
//
// Every character outside [A-Za-z0-9-] is replaced, not just the separator.
// Recovering 300 project directory names from the working directory their
// transcripts record, all 300 need the wider rule and 234 are missed by
// replacing slashes alone — so a query naming a path holding a dot or an
// underscore used to return nothing, which is indistinguishable from a
// directory that genuinely holds no sessions.
func encodeProjectDir(dir string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			return r
		}
		return '-'
	}, strings.TrimSuffix(dir, "/"))
}

// Load implements port.Capture.
func (a Adapter) Load(ctx context.Context, ref port.Ref) (assay.Session, error) {
	if ref.ID == "" {
		return assay.Session{}, port.ErrNotFound
	}

	f, err := os.Open(ref.ID)
	if errors.Is(err, fs.ErrNotExist) {
		// Stores are live: a session listed a moment ago can be pruned before
		// it is read.
		return assay.Session{}, port.ErrNotFound
	}
	if err != nil {
		return assay.Session{}, fmt.Errorf("%w: %w", port.ErrUnsupported, err)
	}
	defer f.Close()

	s, err := parse(ctx, f)
	if err != nil {
		return assay.Session{}, err
	}

	s.Fidelity = assay.Fidelity{
		Adapter:  a.ID(),
		Version:  s.Fidelity.Version,
		Tier:     a.Tier(),
		Observed: a.Capabilities(),
		Partial:  partialCapabilities(),
		Verified: s.Fidelity.Verified,
	}
	return s, nil
}

// knownVersions are the harness releases this adapter has been read against.
//
// An unknown release is parsed rather than refused, and the session is marked
// unverified. Releases arrived every few days across the measured period while
// the keys this adapter reads never moved, so refusing them would break capture
// roughly twice a week to guard against a change that has not yet happened.
// Saying "read, but not from a release anyone has checked" is the honest third
// option between refusing and pretending.
func knownVersions() []string {
	return []string{
		"2.1.142", "2.1.149", "2.1.156", "2.1.170", "2.1.177", "2.1.181",
		"2.1.190", "2.1.191", "2.1.197", "2.1.198", "2.1.199", "2.1.201",
		"2.1.209", "2.1.210", "2.1.211", "2.1.217", "2.1.219", "2.1.220",
	}
}

func isKnownVersion(v string) bool {
	for _, known := range knownVersions() {
		if v == known {
			return true
		}
	}
	return false
}
