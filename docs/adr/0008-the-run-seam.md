# 0008 — The run seam, and the guarantees an adapter is allowed to make

- Status: Accepted
- Date: 2026-07-26

## Context

Assayer has two seams. The first, capture, turns a harness's own record of a
session into the neutral representation, and is settled in 0006. The second,
run, administers a fresh sitting: it drives a coding agent through work it has
already done once, so that what comes back can be compared against what came
back before.

The seam has to be designed before the component that uses it, because the
question it settles is not "how do we start a process". It is: what may a
component downstream believe about a sitting that has been run?

Three facts make that question sharp, and none of them are apparent from a
harness's documentation.

**A cap is not a ceiling.** The reference harness accepts `--max-budget-usd`.
A run capped at $0.001 cost $0.0047. The cap is a threshold the harness notices
having crossed, after the turn that crossed it has been billed. A run capped at
one turn reported having taken two. Anything printing either as a limit is
describing something nobody measured.

**A cap hit is indistinguishable from a crash by exit status.** Both measured
cap hits exited 1. So did a harness that fell over. The only field that
separates them is the harness's own account of why it stopped, and a component
that branched on the exit code would classify a budget stop as a failing run —
which is precisely the manufactured regression the whole design exists to
prevent.

**Documentation and binary disagree.** An earlier draft of this decision
declared turn caps unenforceable, on the evidence that `claude --help` does not
list `--max-turns`. The flag exists and works; the help omits it. The draft would
have shipped an adapter refusing a cap it could have kept, and would have been
defensible right up until someone tried it.

There is also a smaller question. A sitting produces a session, and turning the
harness's identifier for that session into a `Session` means asking the paired
capture adapter for it. The two adapters may not import each other — 0005 is
explicit — and guessing at the newest transcript in a directory is a race as soon
as two sittings share a workspace.

## Decision

### The interface

`port.Run` has four methods: `ID`, `Tier`, `Guarantees`, and
`Sit(ctx, *assay.Assignment) (assay.Sitting, error)`. It is a separate interface
from `port.Capture` rather than four more methods on it, because the halves are
separately implementable: a harness whose transcripts can be read but which
cannot be driven headlessly still has a working capture adapter, and an agent
with no transcripts at all can still be run from a command template.

An `Assignment` carries what changes a run and nothing else: the instruction, a
prepared working directory, a model, a tool allowlist, a settings overlay, and
caps. It is not the exam. Assertions, provenance and baselines are no business of
a runner.

Isolation is not here. Preparing a worktree at a pinned revision, a sandbox, or a
container is a decision about the sitting rather than about the harness, and an
adapter that made it would make it once per harness.

### Guarantees are declared, and asking for more is refused

`Guarantees` states, per cap, whether the adapter can hold it — not at all,
supervised by ending the process it owns, or natively by the harness's own flag —
and, per assignment field, whether it can apply it at all. The universal
command-template runner named in the roadmap can do almost none of it.

An assignment asking for something undeclared is **refused before a process
starts**, by a shared `port.Refuse` rather than by each adapter's own diligence.
Dropping a cap silently is the failure this prevents: the sitting would run, come
back looking clean, and the budget the user set would be one nobody kept.

The check runs one way, exactly as the capability check does on the capture side.
An adapter asking for more than it declared is caught. An adapter declaring a
guarantee it never keeps is not, and cannot be without watching it run.

### One stop reason may reach a verdict

`Sitting.Stop` is why a run ended, and `StopReason.Decides()` is true for exactly
one value: the harness finished the work and said so. A cap hit, a permission
stall, a crash, and output that could not be read are all ERROR — Assayer's
plumbing, the environment, or the harness, never evidence about the model. This
is `ERROR ≠ FAIL` moved out of the reviewer's memory and into the type.

Errors returned from `Sit` are reserved for runs that did not happen:
`ErrRefused` and `ErrUnavailable`. Anything that ran returns a sitting, however
badly it went. A caller cancelling gets its context's error and no sitting, so
that a cancelled suite does not fill a ledger with sittings nobody administered.

### `Query.Native` closes the loop

`port.Query` gains one narrowing: the harness's own identifier for a session, the
value an adapter puts in `Lineage.ID`. A sitting reports it; the paired capture
adapter resolves it. The seams stay orthogonal, 0005 is untouched, and no adapter
imports another.

The reference implementation matches on the transcript's file name, because the
store names a transcript for the session it holds — measured across 400 sampled
transcripts with no exception — and matching on contents instead would mean
opening every transcript in the store: 741 on the machine measured, out of 2,485
files once the sidecars `Discover` already skips are counted. A transcript whose
name disagreed with its contents would not be found; the narrowing fails closed,
returning nothing rather than the wrong session.

### The conformance kit stops at the process boundary

`VerifyRun` calls `Sit` only with assignments a conforming adapter refuses
before it starts anything. Reading a transcript is free, so the capture kit loads
one and inspects it. Administering a sitting is not: it starts a coding agent
against a real model on somebody's key. A kit that spent a contributor's money to
prove an adapter well behaved would be a kit deleted from the test run.

The distinction matters and is stated rather than glossed: the kit depends on the
adapter calling `Refuse` first, which is the thing it is testing. An adapter that
forgot would run those assignments for real. Making that structurally impossible
needs a dry-run seam this design does not have, so adapters point `Bin` at a path
that does not exist while under test, and the residual risk is written down here.

So the kit checks declarations and refusals — everything that terminates before a
process starts — and says so in its own documentation. Whether an adapter drives
its harness correctly is proved in that adapter's own tests, against recorded
output and a stub executable, and the reference adapter's fixtures are verbatim
results from real runs rather than structs written from a schema.

### What the reference adapter guarantees

Budget and turns natively; wall clock supervised, since the harness has no
timeout flag and the adapter owns the process. Model, tool allowlist and settings
overlay all applied. `--bare` plus an empty `--setting-sources` is the
hermeticity keystone: ambient configuration is skipped entirely, and the
assignment states what applies instead. `--fallback-model` is never passed — it
is opt-in, and it would let the harness answer with a model other than the one
under examination.

The pinned reliance on `--max-turns` is deliberate and recorded here: a flag
absent from a program's help is a flag nobody promised to keep.

## Consequences

A component downstream can ask what a sitting is worth before spending anything
on it, and can be told no. A report can name the guarantee behind a verdict,
because the sitting carries the adapter's declaration rather than a lookup that
may have changed since the run.

The honesty is one-directional and stays that way until per-adapter fixtures
exist for it — the same open edge as on the capture side, tracked as issue #39.

The measured facts above have a shelf life. Both caps and their reported subtypes
were established against one release, on one machine, on one account; a future
release could change the overshoot, the exit status, or the flag's existence. The
fixtures pin this adapter against what was recorded, and the release they came
from is named beside them in `testdata/README.md` — but nothing re-runs the
harness, so a release that renames a subtype degrades to an unclassified stop,
which is ERROR and therefore safe, and silent. Noticing it needs the scheduled
re-measurement §13.5 of the design describes, which does not exist yet.

Two couplings are declared here rather than left to be discovered, in the way
0006 declared the workspace and cost couplings. **`Assignment.Tools` is a list of
tool names**, and tool names are a harness's vocabulary: an exam distilled against
one harness carries names a second will not recognise, and the neutral `ToolKind`
that exists precisely to avoid this cannot express an allowlist. An adapter that
declares `Tools: true`, applies the list faithfully and matches nothing is
conforming and useless. **`Assignment.Settings` is a string** whose content only
the adapter understands, which is an escape hatch no import rule or vocabulary
scan can see. Both are the "vendor-shaped neutral representation" 0005 says only
review catches. Neither is fixable without a vocabulary this project has not
earned yet; both are the reason the universal runner will declare `Tools: false`.

**`Caps` can bound money, and most harnesses do not report money.** 0006 records
that the reference harness reports no cost at all in its transcripts and that
comparisons are denominated in tokens, which every candidate reports. A token cap
is the one most adapters could actually keep, and it is absent. It is additive to
both `Caps` and `Guarantees`, so this is a gap rather than a mistake — but a
second adapter will hit it before a second user does.

Nothing consumes this seam yet. The Invigilator, which will, is where isolation
tiers, escalation and pacing land — and it is deliberately not designed here,
because the last time this project designed a runner's guarantees without running
one, it got two of the three wrong.
