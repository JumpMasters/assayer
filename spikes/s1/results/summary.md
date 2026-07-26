# S1 — variance calibration: results

Measured 2026-07-25. Four exams, thirty-five replays each, fixed conditions.
Raw rows in `sittings.csv`; regenerate the headline table with
`python3 spikes/s1/analyze.py`.

An earlier version of this document reported ten replays of three exams. Those
rows are still in the data; this supersedes the reading taken from them, and
§"What ten replays got wrong" records where they misled.

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

| exam | replays | errors | scored | passed | pass rate | 95% interval | cost median | vs golden |
|---|---|---|---|---|---|---|---|---|
| A — bounded fix, pointed | 35 | 0 | 35 | 35 | 100% | [90%, 100%] | $0.147 | 0.99× |
| B — feature slice, pointed | 35 | 4 | 31 | 30 | 96.8% | [84%, 99%] | $0.243 | 1.03× |
| C — design latitude | 35 | 1 | 34 | 34 | 100% | [90%, 100%] | $0.328 | 1.17× |
| D — discovery, unpointed | 35 | 3 | 32 | 32 | 100% | [89%, 100%] | $0.573 | 0.85× |

140 replays, **$48.50** notional in total.

Against the three clauses of the pre-registered bar, on point estimates: every
exam ≥ 90% **met**; no exam below 70% **met**; median cost ≤ 2× golden **met**.

**The interval is the honest reading, and it is weaker than the point estimate.**
Only A and C have a 95% lower bound at or above 90%. D falls one point short at
89%, and B at 84% because it recorded a genuine failure. What this measurement
establishes is that deterministic assertions reproduce at a high rate; it does
not establish a 90% floor for every exam.

## Dispersion: ambiguity, not effort

Maximum-over-minimum is outlier-driven and was the wrong statistic to lead with.
By robust measures:

| exam | CV | IQR / median | p90 / p10 | max / min |
|---|---|---|---|---|
| A — trivial, pointed | 0.148 | 0.030 | 1.20 | 1.75 |
| B — moderate, pointed | 0.075 | 0.084 | 1.24 | 1.35 |
| C — high latitude | **0.382** | **0.710** | **2.58** | 2.85 |
| D — discovery, high effort | 0.143 | 0.160 | 1.35 | 2.07 |

C is the outlier on every robust measure — coefficient of variation two and a
half times the next highest, interquartile range over median more than four
times. A's apparent 1.75× range is one expensive replay; its body is the
tightest of the four.

**D is the control that separates the two candidate explanations.** It costs
four times exam A, touches twice as many files, and takes three times the turns,
yet its p90/p10 is 1.35 against A's 1.20. Effort does not drive dispersion.
What distinguishes C is that many different implementations are correct — one
error class or several, inline checks or a shared validator — while D, for all
its work, has one correct outcome that simply has to be found.

The practical statement: **cost variance tracks how many shapes a correct
answer can take, not how much work it takes.**

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
- **Its file footprint was 2 — identical to every passing B replay.** A scope
  or footprint contract of the kind the design proposes as a tier-2 signal would
  not have caught this. The replay modified the same number of files; it changed
  more inside one of them. Footprint is a weaker signal than the design assumes.

## Instrument errors cost a claim

Eight replays of 140 (5.7%) ended in a permission stall and were recorded as
errors, excluded from the denominators above rather than counted as failures.
Two causes, both the allowlist's fault rather than the model's:

- `sed` was not on the list at all; replays reaching for a shell-based bulk edit
  were denied.
- `uv run python -c` with an embedded newline was denied despite `Bash(uv:*)`
  being allowed, so a replay verifying its own work with an inline snippet was
  blocked.

The cost is precise: D's three errors reduced its scored sample from 35 to 32,
which moved its lower bound from 90% to 89%. **The instrument's own limitation,
not the model's behaviour, is why one exam misses the floor.** The allowlist was
deliberately not widened mid-run — changing conditions partway would have split
the dataset silently.

## What ten replays got wrong

The first reading of this measurement used ten replays per exam. It was wrong in
two ways that matter, both in the optimistic direction:

- **It understated dispersion on every exam**: A 1.08× → 1.75×, B 1.28× → 1.35×,
  C 2.70× → 2.85×, D 1.46× → 2.07×. Tails need samples.
- **It observed no failures at all**, and concluded the exams might be uniformly
  too easy. At 35 replays a genuine failure appeared — on B, the moderate exam,
  not on the two built to be hard.

Both corrections point the same way: a ten-replay series is enough to detect a
gross regression and not enough to characterise a distribution. That is directly
relevant to the escalation policy, which currently treats 5 as a strong series.

## Consequences for the design

- **Baseline-relative cost bands cannot use one global multiple.** A band sized
  for C would be useless on A. The band belongs per exam, calibrated from that
  exam's own series, and — since dispersion tracks ambiguity — an exam's band
  cannot be predicted from its cost or difficulty.
- **Footprint contracts are weaker than assumed.** The one real regression here
  was invisible to file-count metrics.
- **A single session is a weak cost anchor.** C's golden fell at the 10th
  percentile of its own replays at n=10; across 35 its ratio settled to 1.17×
  from 1.52×. Anchoring on the median of a passing series is materially better.
- **The tool allowlist is part of the measurement.** A stall is recorded as an
  error and shrinks the effective sample, which can cost a claim outright.

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
