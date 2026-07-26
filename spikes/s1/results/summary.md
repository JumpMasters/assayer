# S1 — variance calibration: results

Measured 2026-07-25. Four exams, thirty-five replays each, fixed conditions.
Raw rows in `sittings.csv`. Every figure below is computed by
`python3 spikes/s1/analyze.py`, so this document is regenerated rather than
transcribed.

An earlier version reported ten replays of exams A, B and C. Those rows are
still in the data; this supersedes the reading taken from them, and
§"What ten replays got wrong" records where they misled. Exam D did not exist
then — its ten-replay figures below are computed retrospectively for comparison,
not corrections of anything published.

## Conditions

| | |
|---|---|
| Model | `claude-opus-4-8` (pinned; every replay recorded its resolved canonical id) |
| Repository | `JumpMasters/tollgate` at `6b0ded78`, one git worktree per replay |
| Isolation | worktree at the pinned commit, empty settings, `--setting-sources project`, no MCP, no skills |
| Tools | a fixed allowlist: file edits, search, `uv`, `python3` |
| Assertions | exam-supplied fail-to-pass tests plus the full 82-file unit suite as pass-to-pass |
| Auth | subscription; costs are the harness's own `total_cost_usd`, notional rather than billed |

## Results

| exam | replays | errors | scored | passed | rate | 95% lower | 95% upper |
|---|---|---|---|---|---|---|---|
| A — bounded fix, pointed | 35 | 0 | 35 | 35 | 100% | **90.110%** | 100% |
| B — feature slice, pointed | 35 | 4 | 31 | 30 | 96.8% | 83.806% | 99.428% |
| C — design latitude | 35 | 1 | 34 | 34 | 100% | 89.848% | 100% |
| D — discovery, unpointed | 35 | 3 | 32 | 32 | 100% | 89.282% | 100% |

140 replays, 131 of 132 scored reproduced, **$48.50** notional in total.

Against the three clauses of the pre-registered bar, on point estimates: every
exam ≥ 90% **met**; no exam below 70% **met**; median cost ≤ 2× golden **met**.

**Only one exam of four establishes the floor.** Bounds are printed to three
decimals deliberately: C's lower bound is 89.848%, which rounds to "90%" and is
not 90%. Rounding a lower bound up is the one direction this measurement cannot
afford, and an earlier draft of this document did exactly that and then wrote
prose treating C as clearing. A point estimate meeting a floor is not the same
as establishing it. What 140 replays establish is that deterministic assertions
reproduce at a high rate; only exam A supports the stronger claim.

## Dispersion: ambiguity dominates effort

Maximum-over-minimum is outlier-driven and was the wrong statistic to lead with.
Quantiles use the exclusive method — other conventions give different p90/p10
figures, so the choice is stated rather than assumed.

| exam | CV | IQR / median | p90 / p10 | max / min |
|---|---|---|---|---|
| A — trivial, pointed | 0.148 | 0.030 | 1.20 | 1.75 |
| B — moderate, pointed | 0.075 | 0.084 | 1.24 | 1.35 |
| C — high latitude | **0.382** | **0.710** | **2.58** | 2.85 |
| D — discovery, high effort | 0.143 | 0.160 | 1.35 | 2.07 |

C is the outlier on all four measures — coefficient of variation two and a half
times the next highest, interquartile range over median more than four times.
A's apparent 1.75× range is a single expensive replay; the body of its
distribution is the tightest of the four on IQR/median and p90/p10, though its
CV is higher than B's, so "tightest" holds for the body and not for every
measure.

**Exam D separates the two candidate explanations.** It costs four times exam A,
touches twice as many files, and takes three times the turns. Its dispersion is
higher than A's on every measure — p90/p10 1.35 against 1.20, IQR/median 0.160
against 0.030 — so effort is not neutral. But the gap between D and A is small
beside the gap between C and everything else.

**Hypothesis this calibration is consistent with, not a result it establishes:**
cost dispersion is driven far more by how many shapes a correct answer can take
than by how much work it takes. The evidence is four exams in one repository on
one afternoon, and exam C is a single point carrying most of the contrast —
remove it and little relationship remains. It is a plausible mechanism worth
testing across more exams, not a finding to build a threshold on.

## The one genuine failure

Replay `b/33` passed its fail-to-pass test and failed the no-regression suite.
The task asked for a new function; that replay also changed an *existing* one
"for consistency", with an error class that did not subclass `ValueError`,
breaking a test that asserts on it.

Three things follow:

- It is a true positive. The instrument caught a real regression caused by
  unrequested scope extension, not by failing the assigned task.
- **The fail-to-pass test alone would have passed it.** Only the broad
  no-regression suite discriminated, which argues for running the whole suite
  rather than a sampled subset.
- **Its file footprint was 2 — identical to all 31 scored B replays.** A scope
  or footprint contract of the kind the design proposes as a tier-2 signal would
  not have caught this. The replay changed the same number of files; it changed
  more inside one of them. Footprint is a weaker signal than the design assumes.

## Instrument errors shrink the sample

Eight replays of 140 (5.7%) ended in a permission stall, recorded as errors and
excluded from the denominators rather than counted as failures. Two causes, both
the allowlist's rather than the model's:

- `sed` was not on the list at all; replays reaching for a shell-based bulk edit
  were denied.
- `uv run python -c` with an embedded newline was denied despite `Bash(uv:*)`
  being allowed, so a replay verifying its own work with an inline snippet was
  blocked.

Errors cost sample size, and sample size is what a lower bound is made of. D lost
three replays and C one. It cannot be said that this is *why* either misses the
floor: an errored replay produced no verdict, so whether it would have passed is
unknowable, and C misses at 89.848% having lost only one. What can be said is
that a 5.7% error rate narrows every interval it touches, for reasons that have
nothing to do with the model.

The allowlist was deliberately not widened mid-run: changing conditions partway
would have split the dataset silently.

## What ten replays got wrong

The first reading used ten replays of exams A, B and C. It was wrong in two ways
that matter, both optimistic:

- **It understated dispersion on every exam.** Max/min over the first ten
  replays against all thirty-five: A 1.08× → 1.75×, B 1.28× → 1.35×,
  C 2.70× → 2.85×. Exam D, computed retrospectively over the same window,
  goes 1.36× → 2.07×. Tails need samples.
- **It observed no failures at all**, and concluded the exams might be uniformly
  too easy. At thirty-five a genuine failure appeared — on B, the moderate exam,
  not on either exam built to be hard.

Both corrections point the same way: a ten-replay series is enough to detect a
gross regression and not enough to characterise a distribution. That is directly
relevant to the escalation policy, which currently treats five as a strong series.

## Consequences for the design

- **Baseline-relative cost bands cannot use one global multiple.** A band sized
  for C would be useless on A, and an exam's band does not appear predictable
  from its cost or difficulty. Calibrate per exam, from that exam's own series.
- **Footprint contracts are weaker than assumed.** The one real regression here
  was invisible to file-count metrics.
- **A single session is a weak cost anchor.** C's golden fell at the 10th
  percentile of its own replays at ten; across thirty-five its ratio settled from
  1.52× to 1.17×. Anchoring on the median of a passing series is materially
  better.
- **The tool allowlist is part of the measurement.** Stalls are recorded as
  errors and shrink the effective sample, which narrows every interval.

## Disclosures

- **The exams were authored, not harvested.** No single-goal session existed in
  the available history — every surviving one was a multi-issue sweep, a
  continuation, or a plan execution, one spanning ten git branches. Exams written
  to be single-goal are cleaner than exams distilled from real work, so these
  figures are an optimistic bound.
- **The golden sessions were agent-steered, not human-steered**, each completing
  without correction. A human-steered session would cost more, loosening the
  cost comparison.
- **Hermeticity was hand-built.** `--bare` forces API-key auth and these replays
  ran on a subscription, so isolation came from an empty settings file,
  `--setting-sources project`, and a clean worktree. Ambient toolchain state was
  not controlled.
- **Costs are notional**, from the harness's own accounting under a
  subscription — consistent with each other, not invoices.
- **One repository, one language, one afternoon, one model build.** These
  figures describe small edits to a well-tested Python domain layer. Multi-file
  refactors, debugging, a weaker suite, or drift across weeks are all unmeasured.
- **Every replay resolved two models** — `claude-opus-4-8` for the work and
  `claude-haiku-4-5` for auxiliary calls. Both are recorded.
