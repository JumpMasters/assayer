# S3 — distillation rehearsal: protocol and pass bar

**Pre-registered.** This file was written and committed before any replay was run.
The results land in `results/summary.md` and are read against the bar below,
whichever way they come out. Moving the bar afterwards requires writing down why,
first, in the same file.

## The question

S1 measured whether a *hand-authored* exam reproduces its assertions under fixed
conditions. It does — 131 of 132 scored replays. But S1's own disclosures record
that the exams were authored rather than harvested, because no single-goal
session existed in the available history to distil from.

That leaves the load-bearing assumption untested. The method is: bless a real
session, distil it into a replayable exam. If real sessions cannot be distilled,
a high reproduction rate on authored exams says little about the product.

S3 asks: **can one real, messy, multi-turn session be folded into a single
headless instruction that replays?**

## What was already known when this bar was written

Stated so the pre-registration is not mistaken for more than it is. Before
writing the bar, the local transcript store had already been surveyed and the
subject session chosen, so these were known:

- how many human turns the candidate sessions carry, and what they contain;
- which commit the chosen session's first goal produced, and its diff;
- the golden session's own token usage.

What was **not** known, and is what the bar is about: whether the distilled exam
reproduces the golden outcome when replayed.

## Subject

One session from this machine's own history, against
`JumpMasters/tollgate`:

| | |
|---|---|
| Session | `5d05e0b9`, 2026-07-19 17:38 → 2026-07-20 06:21 local |
| Shape | 9 human turns over 12.5 hours; two goals; a mid-session design negotiation that changed what got built; one approval-by-number |
| Goal under test | the session's first goal, which landed as commit `d84d3ad` |
| Golden diff | 28 files, +370/−18 — a new token class threaded from wire through domain, pricing, persistence and SDK |
| Base SHA | `fc5110f` (the commit's parent) |

The session is agent-steered rather than human-steered, which S1 disclosed as a
limitation and which this spike inherits. It was chosen because its repository
is small, hermetically replayable, and already pinned by S1's runner; the most
human-steered session available lives in a polyglot monorepo that is not
replayable at spike cost.

## Distillation procedure

By hand — there is no product code, and writing some to answer whether the
product should exist would be backwards. The point is to record what a distiller
would have to do, not to build one.

1. Reconstruct the instruction from the session. Fold in the answer the human
   gave to the agent's mid-session question, per the design's rule that
   mid-session answers become part of the instruction.
2. Record, clause by clause, where each piece of the instruction came from.
3. Pin the workspace at the base SHA.
4. Derive the assertions from the golden commit's own tests.

**Assertions.** Fail-to-pass is the eight unit tests the golden commit added:

```
tests/unit/test_pricing.py   — 5 tests
tests/unit/test_commit.py    — 2 tests
tests/unit/test_grace.py     — 1 test
```

Pass-to-pass is `tests/unit` entire, as in S1. The commit's integration tests
need Postgres and are excluded; that narrows what the exam checks, and the
results say so.

Tests are injected after the harness runs, from a patch, with any agent edits
under `tests/` discarded first — the same shape S1 used, so a replay can neither
name a node id it never saw nor pass by writing its own test.

## Replay conditions

S1's runner (`spikes/s1/run-sitting.sh`), unmodified, under S1's conditions:
`claude-opus-4-8` pinned, one git worktree per replay at the base SHA, empty
settings, `--setting-sources project`, no MCP, no skills, subscription auth,
costs from the harness's own `total_cost_usd`.

Two deltas from S1, both because this exam is larger:

- `CAP_USD=8.00`. Deliberately generous. A cap hit is recorded as an error, and
  errors shrink the sample a confidence bound is made of — S1 lost 5.7% of its
  replays that way and said so.
- `TIMEOUT_S=1800`, against S1's 900.

**N = 10.** S1 established that ten replays detect a gross problem and do not
characterise a distribution. Ten is the right size for a go/no-go and the wrong
size for a confidence claim; the results will not make one.

Projected cost is $15–$30 at the observed per-replay range for work of this size,
with the cap as the ceiling.

## The pass bar

S3 passes only if all three clauses hold.

1. **Reproduction.** At least 8 of 10 scored replays pass both the fail-to-pass
   set and the no-regression suite. Set below S1's 90% deliberately and in
   advance: the instruction here is distilled from a real session rather than
   written to be replayable, and that is the harder case by construction.

2. **Non-interactivity.** No replay stalls asking a clarifying question. Replay
   is non-interactive by doctrine; a sitting that asks is a defect in the
   distillation, not a signal about the model.

3. **Recoverability.** Every clause of the distilled instruction traces to
   either the session transcript or the repository at the base SHA — the two
   artefacts a tool would actually have at bless time. Any clause that traces
   only to the author's memory of the session is recorded as such, and if
   removing it changes the outcome, this clause fails.

Clause 3 is the one most likely to fail, and it is the one that matters. A tool
that can only distil sessions whose author is standing next to it is not the
tool described.

Recorded but **not** gating: cost, turn count, and file footprint per replay.
The golden session's cost is not a usable baseline here — it spans a design
negotiation, a pull request, and a CI wait that the exam does not ask for — and
saying so is one of the things this spike is for.

## Amendment, recorded before the first replay

Building the exam surfaced a scoring flaw, fixed before any replay was run and
written down here rather than discovered afterwards in the numbers.

The golden commit's unit tests construct `ModelPrice(...)` in module-level
fixtures. A replay that does not add the new field — or adds it under a
different name — therefore fails at *collection*: pytest exits 4 for a single
node id and 2 for a directory. S1's runner maps every code other than 0 and 1 to
`ERROR`, deliberately, because a missing node id normally means a rotted pin and
a rotted pin must never read as a regression.

Here it would have meant the opposite of what the bar intends. Naming divergence
is the failure mode this spike most wants to observe, and it would have been
recorded as an error, excluded from the denominator, and quietly made clause 1
easier to pass the worse the distillation performed.

The fix, in `s1/run-sitting.sh` behind `COLLECT_ERROR_IS_FAIL`, off by default so
S1's own semantics are untouched: when the exam supplies its own tests via
`tests.patch` — already applied successfully by the runner — and those tests are
verified to collect and pass against the golden solution, the tests are certainly
present, so a collection error is the subject failing rather than Assayer's
plumbing. That is a `FAIL`. Internal errors, timeouts, permission stalls and cap
hits stay `ERROR`. `s1/test-runner.sh` covers both policies.

Both preconditions were checked before enabling it, at the base commit and at the
golden commit under the exam's own conditions:

| | |
|---|---|
| Golden solution, 8 fail-to-pass node ids | all pass |
| Golden solution, `tests/unit` | 419 passed |
| Base commit, 8 fail-to-pass node ids | all fail — the exam is not vacuous |
| Base commit, an untouched unit file | passes — the failure is the pin, not the environment |

## If it fails

A failure is a result, not a setback, and it lands before Phase 1 rather than
during it. The candidate responses, in the order they would be considered:

- **Clause 1 fails** — the exam format needs more than an instruction and a base
  SHA to make real work replayable.
- **Clause 2 fails** — distillation needs to detect and resolve ambiguity at
  bless time, and the distillability score needs to gate on it.
- **Clause 3 fails** — blessing cannot infer the instruction; it has to ask the
  human for it, and the design's claim that the distiller suggests while the
  human reviews shifts materially toward the human.
