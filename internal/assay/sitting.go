package assay

import "time"

// Assignment is one administration of an exam, described neutrally.
//
// It is what a run adapter is handed, and it is deliberately not the exam: an
// exam is a committed artifact with assertions, provenance and a baseline, none
// of which a runner has any business reading. What is here is only what changes
// the run.
//
// Every field beyond the first two is something some adapter cannot honour — a
// runner driving an agent through a command template has no concept of a tool
// allowlist and no settings file to overlay — so each one is a claim an adapter
// makes in its Guarantees and is refused, never quietly dropped, by an adapter
// that cannot make it.
type Assignment struct {
	// Instruction is the distilled ask, delivered whole.
	Instruction string

	// Dir is the prepared working directory. The adapter runs in it and does not
	// create it: isolation — a worktree at a pinned revision, a sandbox, a
	// container — is a decision about the sitting rather than about the harness,
	// and an adapter that made it would be making it once per harness.
	Dir string

	// Model is the model to examine, named as a user would name it. An empty
	// value leaves the harness on its default, which is a different measurement
	// and is recorded as such.
	Model string

	// Tools is the allowlist derived from the golden session, plus whatever
	// widening was reviewed. Nil leaves the adapter's default reach in place,
	// which is wider than any exam should need.
	Tools []string

	// Settings is the configuration under examination — a path or literal JSON,
	// as the adapter's harness accepts it. It is the other half of a hermetic
	// sitting: ambient configuration is skipped, and this is what applies
	// instead.
	Settings string

	Caps Caps
}

// Caps bound a sitting. A zero field is no cap of that kind.
//
// Caps do not exist to make sittings cheap. They exist so that a sitting which
// runs away is stopped by something that knows it stopped it, and reaches a
// verdict of ERROR with a reason, rather than running until a human notices.
type Caps struct {
	// BudgetMicroUSD is money in integer millionths of a dollar, matching Usage.
	BudgetMicroUSD int64

	// Wall is the longest a sitting may take.
	Wall time.Duration

	// Turns is the most exchanges a sitting may use. A harness enforcing this
	// may report having used more than it was allowed; see Enforcement.
	Turns int
}

// Guarantees is what a run adapter can actually promise about a sitting.
//
// The capture seam has the same shape for the same reason: an adapter that
// cannot see something must say so rather than return a zero that reads as an
// observation. Here the failure is the mirror image — an adapter that cannot
// apply something must say so rather than run anyway and produce a sitting that
// looks like it was run under conditions nobody imposed.
//
// Checked one way, like the capture side: Refuse catches an assignment asking
// for more than the declaration, and nothing catches a declaration whose
// promise is never kept. Per-adapter fixtures are what would catch that.
type Guarantees struct {
	// How strongly each cap is held.
	Budget Enforcement
	Wall   Enforcement
	Turns  Enforcement

	// Whether the corresponding assignment field can be applied at all.
	Model    bool
	Tools    bool
	Settings bool
}

// Sitting is the record of one administration: what the harness did, what it
// cost, and why it stopped.
//
// It carries no assertions and no verdict. Deciding is the Examiner's, and the
// separation is what keeps a sitting replayable by an oracle after the fact.
type Sitting struct {
	// Adapter, Tier and Guarantees travel with the sitting rather than being
	// looked up later, because a report has to name the guarantee behind a
	// verdict and an adapter's declaration can change between the run and the
	// reading.
	Adapter    string
	Tier       Tier
	Guarantees Guarantees

	// Native is the harness's own identifier for the session this produced. It
	// is what a capture adapter resolves — Query.Native — to turn a sitting into
	// a Session, and it is opaque to everything in between.
	Native string

	Stop StopReason

	// Usage is what the sitting consumed, as the harness reported it.
	//
	// Cost is here and is absent from any session the reference capture adapter
	// produces, because a transcript records no price and a headless run reports
	// one. This is the only place a real number reaches a cost band.
	//
	// Usage.Wall is left unset. See Wall below.
	Usage Usage

	// Wall is how long the run took, measured across the process.
	//
	// Its own field rather than Usage.Wall, which means something else: the span
	// between the first and last timestamps a transcript happens to contain. A
	// golden session is usually interactive and its span includes the hours a
	// human spent thinking between records; a sitting is a headless run of
	// minutes. Carrying both on one field would let a diagnostic compare them and
	// report a large, plausible, meaningless number.
	Wall time.Duration

	// Turns is the number of exchanges the harness says it took, and is zero when
	// the harness does not report one.
	//
	// Reported, never derived, and it may exceed the cap it was given: a measured
	// run capped at one turn reported two. Usage carries no turn count because it
	// would duplicate the length of a turn slice; a sitting has no turn slice, and
	// this is the only turn figure available without resolving Native into a
	// Session — which a sitting stopped before it finished may not be able to do.
	Turns int

	// Denials names the tools the harness was refused during the sitting. A
	// non-empty list means the exam's allowlist or isolation tier does not match
	// the work, which is a defect in the exam and not evidence about the model.
	Denials []string

	// ExitCode is the harness's exit status, kept for the report rather than for
	// a decision: two measured cap hits exited 1, and so does a crash. Stop is
	// the field to branch on.
	ExitCode int

	// Stderr is whatever the harness said on the way out, attached so that an
	// ERROR verdict can show the reason in the harness's own words.
	Stderr string
}
