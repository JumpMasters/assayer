# 0004 — The conditions the S1 calibration was measured under

- Status: Accepted
- Date: 2026-07-25

## Context

Assayer's method rests on one empirical claim: that natural run-to-run variance
of a coding agent is small enough, at the level assertions actually check, for a
real change to stand out within an affordable number of replays. The bar for
that measurement was fixed and published before any data was collected, and no
product code was to be written until it was met.

Taking the measurement required decisions the design had not anticipated. Each
of them affects how far the resulting numbers generalise, so they are recorded
here rather than left implicit in a script.

**The available session history contained nothing usable.** The design assumed
calibration exams would be distilled from real sessions already on the machine.
They were not there to distil. Two repositories' transcripts had already been
deleted by the harness's thirty-day retention default, and every surviving
session was a campaign rather than a task: a sweep across many issues, a
continuation of earlier context, or the execution of an external plan document.
One spanned ten git branches. The tidy single-purpose commits in those
repositories' histories were produced *by* those sprawling sessions; the
tidiness lives in the git history, not in the transcripts.

**Hermeticity could not be obtained the way the design assumed.** The design
names the harness's minimal mode as the keystone that keeps ambient
configuration out of a replay. That mode forces authentication by API key. These
replays ran on a subscription, so it was unavailable.

## Decision

**The calibration exams were authored, not harvested.** Three tasks were written
deliberately single-goal, spanning a difficulty range from a bounded fix with one
correct answer to a task where competent implementations diverge structurally.

**Each exam supplies its own verification tests**, applied after the replayed
agent has finished and with the agent's own edits to test files discarded first.
The agent cannot influence the verdict by writing or editing a test. Those tests
import the symbol under test inside the test body, so an absent implementation
produces an assertion failure rather than a collection error — a collection
error would be classified as an instrument error and drop out of the pass-rate
denominator, silently inflating the measured rate.

**Hermeticity was rebuilt from flags**: a git worktree at a pinned commit, an
empty settings file, no MCP servers, no skills, and — the load-bearing one —
restricting settings to project scope. Supplying a settings file only *adds*
settings; it does not displace the user's own. Without the scope restriction,
user-level plugins loaded into every replay, including one that injects a
growing store of past observations. That store changes between runs, so it would
have been recorded as model variance. A golden session taken before this was
found was discarded.

**The scaffolding is committed** under `spikes/`, exempt by an explicit
narrowing of this repository's production-quality rule, and covered by its own
README.

## Consequences

The published variance figure is an **optimistic bound**. Exams authored to be
single-goal are cleaner than exams distilled from real work, and the golden
sessions were agent-steered and completed without correction, where a
human-steered session would cost more and loosen the cost comparison.

Isolation is **weaker than the design's first tier**. Configuration drift is
excluded; ambient toolchain and network state are not. Any user who will not
authenticate with a metered API key gets this weaker tier, which the design does
not currently acknowledge and which reports will have to name.

Costs on both sides of every ratio are **notional rather than billed**, taken
from the harness's own accounting under a subscription. They are consistent with
each other and are not invoices.

The production-quality rule now carries a **scoped exception**. It narrows to
`spikes/` and never to the product packages or to CI. The exception exists
because these numbers are published, and a published measurement whose method
cannot be inspected is an assertion rather than evidence.

Two findings from the measurement bear on design decisions not yet reached, and
are recorded in the internal specification rather than here, since no code has
arrived at them: that sessions in practice are campaign-shaped rather than
task-shaped, which makes splitting one session into several exams the common
path rather than an edge case; and that a baseline-relative cost band cannot use
a single global multiple across exams of differing latitude.
