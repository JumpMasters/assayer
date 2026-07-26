# S3 — distillation rehearsal: results

Measured 2026-07-26. One exam, distilled by hand from one real session, replayed
ten times in each of two arms. Raw rows in `sittings.csv` (main) and
`control.csv` (control). Every figure here is computed by
`python3 spikes/s3/analyze.py`, so this document is regenerated rather than
transcribed.

The protocol and the pass bar were fixed in `../README.md` before any replay ran.
The control arm was not; it was added after the main arm returned a clean sweep,
and that is recorded there and again below.

## What was being asked

S1 measured whether a hand-authored exam reproduces. It does. But S1 disclosed
that its exams were authored rather than harvested, because no single-goal
session existed in the available history to distil from — which left the method's
load-bearing assumption untested. S3 tests it: **can one real, messy, multi-turn
session be folded into a single headless instruction that replays?**

## Conditions

| | |
|---|---|
| Session | `5d05e0b9`, 2026-07-19 → 07-20, 9 human turns over 12.5 hours, two goals |
| Goal under test | the first, which landed as tollgate `d84d3ad` (28 files, +370/−18) |
| Base | `fc5110f`, the commit's parent |
| Model | `claude-opus-4-8` pinned; every replay recorded its resolved canonical id |
| Isolation | one git worktree per replay, empty settings, `--setting-sources project`, no MCP, no skills |
| Assertions | 8 fail-to-pass unit tests from the golden commit; `tests/unit` entire as pass-to-pass |
| Runner | S1's `run-sitting.sh`, unmodified apart from a documented scoring narrowing |
| Auth | subscription; costs are the harness's own `total_cost_usd`, notional rather than billed |

## Results

| arm | replays | errors | scored | passed both | median cost | median turns | touched |
|---|---|---|---|---|---|---|---|
| main — golden commit reachable | 10 | 0 | 10 | **10** | $1.95 | 39 | 11 |
| control — history stripped | 10 | 0 | 10 | **10** | $1.91 | 39 | 11 |

20 replays, 20 of 20 reproduced, **$39.24** notional in total.

### Against the pre-registered bar

| clause | bar | result |
|---|---|---|
| 1 — reproduction | ≥ 8 of 10 scored replays pass both sets | 10 of 10, both arms — **met** |
| 2 — non-interactivity | no replay stalls asking a clarifying question | none did — **met** |
| 3 — recoverability | every instruction clause traces to transcript or repo at base | it does; see `../exam/provenance.md` — **met** |

**All three clauses are met, and the third is the one that matters.** The first
two say the machinery works on a real task. The third says the instruction could
have been recovered by a tool holding only what a tool would hold.

## The control arm, and why the headline number is not the main arm

Ten from ten is a reason to look for the way it could be spurious. There was one.

A replay pins a git *worktree* of the real repository. A worktree shares the
object database, the golden commit is a descendant of the base commit on `main`,
and `Bash` is unrestricted. From inside a replay, `git show d84d3ad` returns the
complete solution — verified by running it, not assumed. A clean sweep is
consistent with the instruction carrying the work and equally consistent with a
replay reading the answer.

The control arm removes the reachability and changes nothing else: the tree at
the base commit exported with `git archive` and re-committed as the single commit
of a fresh repository, no branches, no remotes, no descendant. It aborts if the
original history is still reachable, so it cannot quietly measure nothing.

It also passed 10 of 10, at a median cost within 2% of the main arm and the same
median turn count. **The result does not depend on the solution being
reachable.** Had the two arms disagreed, the control arm would have been the
result and the main arm evidence of contamination.

This matters beyond the spike, and that is the more useful half: a user's exam
pins a commit in a repository whose history contains the session it was blessed
from, so the same reachability exists in the product as designed.

## What the distillation actually required

The instruction was not in the human turns, and this is the finding with the most
carry.

The turn that began the work is the two words **`plan 15`**. Its referent is
gitignored in that repository and the file no longer exists — read literally, the
ask is unrecoverable. It is also the wrong ask: four minutes in the agent asked a
sequencing question, the human answered **`1`**, and that answer re-scoped the
work to something else, which is what the session then built.

Both are recoverable, but only from the parts of the transcript that are not user
messages:

| what | where it lives | recoverable |
|---|---|---|
| the original pointer | user turn | yes; its target is gone |
| the re-scoping question | `AskUserQuestion` tool input | yes |
| the human's `1` | user turn | yes, and useless alone |
| **`1` resolved to a named option and its spec** | the **tool result**, verbatim | **yes, mechanically** |
| the full implementation spec | a `Write` tool input — a 41 KB plan the agent wrote in-session | yes |
| the pattern the new field mirrors | repo at the base commit | yes |

A distiller reading only human turns recovers `1`. A distiller reading tool
results recovers the selected option together with the identifiers the tests bind
to, with no inference from `1` to an ordinal position required.

### How representative that is

`census.py` scores every coding session in this machine's transcript store on the
axes the design's distillability score proposes. 54 sessions, 605 human turns:

| | |
|---|---|
| human turns of 4 words or fewer | 236 of 605 (39%) |
| median session's median turn length | 7 words |
| sessions where the agent asked a clarifying question | 38 of 54 |
| sessions touching assistant memory, outside any repo | 46 of 54 |
| sessions touching gitignored design docs | 32 of 54 |
| distinct harness versions represented | 17 (`2.1.142` … `2.1.219`) |

The subject session is not an outlier. Terse pointer-and-approval steering is the
normal shape, and most sessions reach for instruction content that lives outside
the repository's tracked history.

## Consequences for the design

- **The Distiller must read tool results, not just user turns.** On this corpus a
  human-turn-only distiller recovers approvals and pointers for two turns in five.
  Everything load-bearing here was in an `AskUserQuestion` result and a `Write`
  input. This is a requirement on the Session IR: it has to carry tool inputs and
  results, not a flattened chat.

- **Approval-by-number resolves mechanically, and should.** The harness records
  the selected option and its text in the tool result. The distiller should fold
  that resolved option in, per the design's rule about mid-session answers, and
  never try to interpret a bare `1`.

- **The golden session's cost is not the exam's cost.** The design makes the
  golden session's cost profile the exam's first cost baseline. Here the golden's
  goal-1 span cost **$45–52** notional at list rates against **$1.95** per replay
  — 23× to 27× — because the session also negotiated a design, wrote a plan,
  opened a pull request and waited for CI, none of which the exam asks for. A
  baseline-relative cost band anchored on the golden would be meaningless. S1
  found a single session a weak cost anchor; S3 finds it the wrong quantity.
  Anchor on the median of a passing series.

- **Exam isolation has to consider history reachability.** Pinning a base SHA in
  a repository that also contains the golden commit leaves the solution one
  `git show` away. Tier-1 isolation as described does not address this. Whether
  it should be closed or merely disclosed is a design decision, but it cannot
  stay unnoticed.

- **Footprint should record the set of files, not the count.** All 20 replays
  touched 11 files, and the golden's non-documentation footprint is also 11 (10
  under `src/`, 1 migration; the golden additionally wrote 2 ADR documents the
  instruction never asked for). The runner records a count, so the identity is
  suggested and not established. S1 found count-footprint too weak to catch a
  real regression; S3 finds it too weak to confirm a match. Same fix.

- **Distillation must separate the ask from the oracle.** The plan the agent
  wrote in-session contains the golden's test bodies verbatim. A distiller that
  inlines them into the instruction would measure transcription rather than
  reproduction, and would look excellent doing it.

## Disclosures

- **One exam, one session, one goal, one repository, one afternoon.** The session
  had a second goal and the corpus has 53 other sessions; none were distilled.
  The goal chosen is the one that mapped most cleanly to a single commit.

- **This does not show that distillation is automatable.** It shows that a
  careful distillation of a real session replays, and that everything that
  distillation needed was present in the artefacts a tool would hold. The
  instruction was written by hand. Whether the Phase-1 Distiller can produce one
  of this quality is a separate question this spike does not answer, and it is
  the question Phase 1 actually turns on.

- **Ten replays per arm is a go/no-go, not a distribution.** No confidence
  interval is quoted, deliberately. S1 established that ten replays detect a
  gross problem and do not characterise a tail.

- **The exam is unusually well-served by its session.** The agent produced an
  explicit written specification mid-session, which is exactly what made recovery
  clean. 38 of 54 sessions contain a clarifying question, so this is common — but
  a session that never generates a written spec is the harder case and is
  unmeasured.

- **The assertions are narrower than the golden commit.** The commit's
  integration tests need Postgres and were excluded; fail-to-pass is the eight new
  unit tests and pass-to-pass is `tests/unit`. Whether the migration and the
  price-book column are correct is unasserted, though the instruction asks for
  them.

- **The scoring rule was narrowed mid-build**, before any replay ran, and is
  recorded in `../README.md`. Without it a replay that named the field
  differently would have scored `ERROR`, been dropped from the denominator, and
  made the bar easier to pass the worse the distillation performed.

- **The census reads a live store.** `census.py` scores whatever transcripts are
  on this machine when it runs, and that store grows and is pruned. The figures
  above are a snapshot taken on the measurement date; re-running it later will
  not reproduce them exactly, and the script is committed so the method survives
  even though the corpus will not.

- **Costs are notional**, from the harness's own accounting under a subscription
  — consistent with each other, not invoices. The golden session's $45–52 is a
  reconstruction from its recorded token usage at published list rates, and
  depends on a cache-TTL assumption it states.

- **Every replay resolved two models** — `claude-opus-4-8` for the work and
  `claude-haiku-4-5` for auxiliary calls. Both are recorded.
