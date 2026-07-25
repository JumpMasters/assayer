# 0001 — Record architecture decisions

- Status: Accepted
- Date: 2026-07-25

## Context

Assayer makes a number of decisions that are significant and not easily
reversed: what a verdict is allowed to claim, how failures of the tool itself
are distinguished from failures of the thing being measured, where knowledge of
a particular agent harness may live, and what the machine-readable output
promises to callers. Without a durable record of why each choice was made, the
reasoning is lost, and later contributors are left to reconstruct it or
unknowingly undo it.

That risk is sharper here than in most projects. Several of these decisions
deliberately make the tool less impressive: refusing to name a culprit without
running a bisection, refusing to report a regression from a single replay,
excluding errors from pass and fail counts. Each looks, in isolation, like a
limitation worth removing. The record of why it exists is the thing that stops
it from being removed.

## Decision

We keep Architecture Decision Records (ADRs) in `docs/adr`, one Markdown file
per decision, using the format described by Michael Nygard. Each record states
its status, the context that forced a decision, the decision itself, and the
consequences.

Records are immutable once accepted. A decision is changed by adding a new
record that supersedes the previous one, rather than by editing history.

## Consequences

- The motivation behind each significant choice is written down where it can be
  reviewed alongside the code.
- Contributors have a lightweight, consistent place to propose and document
  changes to the architecture.
- A constraint that exists to keep the tool honest can be argued with on the
  record, rather than eroded quietly in a review comment.
- The set of records carries a small maintenance cost: statuses must be kept
  current as decisions are superseded.
