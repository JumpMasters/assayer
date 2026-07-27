// Package assay holds the harness-neutral representation of an agent session
// and the other domain types every component operates on.
//
// This package imports nothing else in this module and holds no state. Its
// types are the contract between capture adapters, which translate a particular
// harness into them, and everything downstream, which must work without knowing
// a harness exists.
//
// The shape here is grounded in one measured harness and a reading of a second,
// which is a real limit on how neutral it can be claimed to be. Two rules keep
// that limit from widening: a field exists because a named consumer needs it,
// not because a transcript happened to contain it; and every field is covered by
// a Capability, so an adapter that cannot observe something says so rather than
// leaving a zero value that reads as an observation.
package assay

import "time"

// Session is one completed agent session.
type Session struct {
	Lineage   Lineage
	Workspace Workspace
	Turns     []Turn
	Usage     Usage
	Delegated []Delegation
	Fidelity  Fidelity

	// OrderComplete reports whether the order of everything the session did is
	// fully known. It is false when delegated work could not be attributed to
	// the call that spawned it, or when a prefix of the session was compacted
	// away. Assertions about the order in which tools ran must resolve to an
	// error rather than a failure when it is false: an incomplete order is
	// missing evidence, not evidence of misbehaviour.
	OrderComplete bool
}

// Models returns the distinct models observed across the session's turns, in
// first-use order.
//
// A method rather than a field. The model is a property of a turn, because
// sessions change model part-way through: a census of 2,459 local transcripts
// found 54 that used more than one and 3 that used three, and one harness
// records the model per turn by construction. A session-level copy would be a
// second source of truth that drifts from the first.
//
// A pointer receiver because a session is large and this must not copy it.
func (s *Session) Models() []Model {
	var out []Model
	seen := make(map[Model]bool, len(s.Turns))
	for i := range s.Turns {
		m := s.Turns[i].Model
		if m == (Model{}) || seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, m)
	}
	return out
}

// Lineage identifies a session and the session it continues.
//
// Two fields rather than one identifier. A resumed or forked session carries
// both its own identity and a pointer to what it came from, and the two are
// stored separately by the one harness measured here — recording only one, or
// preferring the wrong one, attributes a session's work to its parent. The
// parent pointer is a measured field, not an inference: the reference harness
// writes it on a dedicated record.
type Lineage struct {
	ID     string
	Parent string // empty when this session began on its own
}

// Model is the resolved identity of a model.
//
// Alias and Canonical are separate because they disagree exactly when it
// matters. A provider quietly resolving a stable alias to a different model is
// drift worth reporting on its own, and it is undetectable if only the name
// that was asked for is recorded.
//
// An empty Canonical means the adapter could not observe it and must never be
// compared against a recorded one: an adapter that cannot resolve the identity
// declares no CanSeeModelIdentity, and comparing against something unobserved is
// an error rather than a finding.
type Model struct {
	Alias     string
	Canonical string
	Provider  string
}

// Turn is one exchange in a session, in the order it ran.
//
// The nesting of tool calls under a turn is the weakest structural assumption in
// this package. Neither harness examined stores sessions this way — both are
// flat streams of records — so every adapter reconstructs turn boundaries, and
// where it draws them determines any metric counting turns. The alternative, a
// flat list of events, moves the same reconstruction into every consumer
// instead of doing it once at the boundary.
type Turn struct {
	Role Role
	Text string

	// Reasoning is the model's own working, kept apart from Text. Folding the
	// two together would put a scratchpad into anything that reads assistant
	// prose, and would make a diagnostic that counts reasoning behaviour
	// impossible to compute on any harness.
	Reasoning string

	Model Model
	Tools []ToolCall
	At    time.Time
}

// ToolCall is one tool invocation and what it did.
type ToolCall struct {
	// Name is the tool as the harness named it. Opaque to core: nothing outside
	// an adapter may branch on its value. Kind is what core reasons about.
	Name string
	Kind ToolKind

	// Argv is the command as executed, one element per argument.
	//
	// A slice rather than a string because at least one harness records
	// arguments already split, and joining them into a line that is later
	// re-split is wrong for any argument containing a space. Harnesses that
	// record a single command line use a one-element slice, which is at least
	// honest about the ambiguity rather than hiding it.
	Argv []string

	// Input is what was put to the tool: the question asked of the user, the
	// body written to a file.
	//
	// Distilling a real session found the load-bearing content living here
	// rather than in the human turns — the decision that re-scoped the work was
	// an answer to a question the agent asked, and the specification it worked
	// from was the body of a file it wrote. A distiller reading only what the
	// human typed reproduces that failure.
	Input string

	Mutations []Mutation

	// Result is nil when the adapter did not observe one.
	//
	// A pointer rather than a value because a zero-valued result is
	// indistinguishable from a call that succeeded quietly, and the measured
	// pairing invariant runs one way only: no result was seen without a call,
	// which says nothing about calls that never returned.
	Result *ToolResult

	// DelegatedID references an entry in the session's Delegated slice. Empty
	// when the call delegated nothing, or when the link could not be made.
	DelegatedID string
}

// Mutation is one file a tool call changed.
type Mutation struct {
	Path string
	Kind MutationKind

	// Before and After carry the file's content, empty when unobserved.
	//
	// Without them an assertion comparing a replay's changes against the golden
	// session's by similarity cannot be computed at all. The content is
	// available: the reference harness records it in a stream that the first
	// survey classified as bookkeeping and never opened.
	Before string
	After  string
}

// ToolResult is what a tool call returned.
type ToolResult struct {
	Text string

	// Truncated reports that Text is a prefix. Harnesses offload and shorten
	// large results; an assertion that searches a truncated body is answering
	// on partial evidence and needs to know it.
	Truncated bool

	Outcome Outcome

	// ExitCode is nil when no code was observed, which is the ordinary case
	// rather than the exception.
	//
	// A pointer because zero is a meaningful exit code and would otherwise be
	// indistinguishable from absence. The reference harness records whether a
	// call errored and never records the code itself, so an adapter that filled
	// this with 0 would be reporting success beside an Outcome saying the call
	// failed. The distinction the field preserves when it IS observed is
	// load-bearing: test runners use separate non-zero codes for "tests failed"
	// and "the thing you named does not exist", and the second is a pin that has
	// rotted rather than a regression.
	ExitCode *int
}

// Workspace is the state of the files a session worked on.
//
// This type assumes a local filesystem, and Revision assumes a version control
// system with content-addressed revisions. That coupling is stated rather than
// hidden behind neutral names: sessions that ran in a container, on a remote
// host, or against a server-held workspace are not capturable this way, and
// their adapters say so instead of filling these fields with something local.
type Workspace struct {
	// Dirs are the working directories used, in first-use order.
	//
	// A slice because sessions move between directories part-way through: 29
	// transcripts across 15 of 18 measured harness releases did. A single
	// directory would pin the wrong tree.
	Dirs []string

	Revision string // opaque to core
	Branch   string
	Origin   string

	// Patch is the uncommitted state of the tree when the session began.
	//
	// The patch itself, not a flag saying there was one: an exam has to carry it
	// to reproduce the starting state, and a session blessed from a record of
	// work done days ago cannot recover it from a tree that has moved on.
	Patch string

	// Toolchain is the environment fingerprint that staleness is measured
	// against. Without it, a dependency upgrade underneath an exam is
	// indistinguishable from the model getting worse.
	Toolchain []Fingerprint
}

// Fingerprint is one element of an environment, named and versioned.
type Fingerprint struct {
	Name  string
	Value string
}

// Usage is what a session consumed.
//
// Usage is exclusive of Delegated: a session's figures do not include the work
// it handed to sub-agents. Summing is the caller's job. Harnesses differ on
// this, and leaving it unstated would make cost comparisons across adapters
// meaningless while looking fine.
//
// There is no turn count: it would duplicate the length of the turn slice and
// disagree with it the moment an adapter counts something it cannot reconstruct.
type Usage struct {
	// CostMicroUSD is money in integer millionths of a dollar, and is set only
	// when the harness itself reported a price. Costs are summed and compared
	// against bands; binary floating point is the wrong representation for
	// either.
	CostMicroUSD int64

	// InputTokens counts every input token the harness billed against, cached
	// or not. Harnesses report the split — freshly written cache, cache read,
	// neither — and summing it here rather than carrying three fields keeps one
	// number comparable across adapters that record the split differently. A
	// consumer that needs the split is a reason to add fields, and there is not
	// one yet; on the measured store cached input is 99.97% of the total, so an
	// adapter reading only the uncached field would look three orders of
	// magnitude cheaper than one that reads all of it.
	InputTokens  int64
	OutputTokens int64

	// Wall is the span between the first and last observed timestamps. It is
	// neither the session's duration nor the time the agent worked: a transcript
	// records when records were written, and sessions sit idle between them.
	Wall time.Duration
}

// Delegation is work a session handed to a sub-agent.
//
// Flat, and not a Session. Nesting delegated work under the call that spawned
// it, with a separate bucket for work that belongs to no call, encodes one
// harness's storage arrangement: records filed under a call's identifier, left
// orphaned when the part of the session holding that call is compacted away.
// Harnesses that store delegated work as sibling sessions with a pointer to
// their parent cannot produce an orphan at all, and harnesses without
// delegation leave this empty. An unattributed delegation is one no ToolCall
// references, which is a question about the data rather than a second field.
type Delegation struct {
	ID    string
	Turns []Turn
	Usage Usage
}

// Fidelity states what produced a session and how much of it was visible.
// Every report names this; concealing it is a defect, not a simplification.
type Fidelity struct {
	Adapter string

	// Version is the harness release the session came from, opaque to core.
	//
	// It is kept because an exam records the version it was blessed against, a
	// heartbeat check compares the current version to it, and isolating a
	// regression across harness releases needs it as a dimension. Reducing it to
	// the Verified flag below would discard all three.
	//
	// One release, and a session can span a bump: harnesses update while a
	// session is open, and one transcript in a local store of 730 spanned six
	// releases. It names the release the session's last record was written by,
	// which is what the heartbeat above is comparing today's version against.
	// Adapters must not join several into one value — the equality test that
	// field exists for would then never match on exactly the sessions that had
	// already seen a bump.
	Version string

	Tier Tier

	// Observed is what the adapter could see for this session.
	Observed CapabilitySet

	// Partial is what it could see only incompletely — a capability that is real
	// but whose evidence is known to be missing some of what happened.
	//
	// Partial visibility is the ordinary case rather than an exotic one:
	// compaction removes early turns, large results are shortened, and an
	// adapter watching API traffic sees the edits an agent requested but not the
	// files a shell command rewrote underneath it. A capability declared whole
	// when it is partial produces a failure on an assertion about scope — which
	// is precisely the manufactured regression this whole mechanism exists to
	// prevent. An assertion over a partial capability that would fail resolves
	// to an error instead; one that passes still passes.
	Partial CapabilitySet

	// Verified reports whether this harness release has been tested against.
	//
	// Refusing releases nobody has written a fixture for breaks capture about
	// twice a week, at the rate releases were measured arriving. Treating them
	// as verified hides a real class of silent change. Reading them and saying
	// which is which is the honest third option.
	Verified bool
}

// Sees reports whether the adapter observed c completely.
func (f Fidelity) Sees(c Capability) bool {
	return f.Observed.Has(c) && !f.Partial.Has(c)
}

// SeesPartly reports whether the adapter observed c but knows the evidence is
// incomplete.
func (f Fidelity) SeesPartly(c Capability) bool {
	return f.Observed.Has(c) && f.Partial.Has(c)
}
