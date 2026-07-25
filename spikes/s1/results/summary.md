# S1 — variance calibration: results

Measured 2026-07-25. Three exams, ten sittings each, fixed conditions.
Raw rows in `sittings.csv`; regenerate this table with `python3 spikes/s1/analyze.py`.

## Conditions

| | |
|---|---|
| Model | `claude-opus-4-8` (pinned; every sitting recorded its resolved canonical id) |
| Repository | `JumpMasters/tollgate` at `6b0ded78`, one git worktree per sitting |
| Isolation | worktree at the pinned SHA, empty settings, `--setting-sources project`, no MCP, no skills |
| Assertions | one fail-to-pass node plus the full 82-file unit suite as pass-to-pass |
| Auth | subscription; costs are the harness's own `total_cost_usd`, notional rather than billed |

## Results

| exam | scored | errors | tier-1/2 pass | cost median | golden | ratio | turns | touched files |
|---|---|---|---|---|---|---|---|---|
| A — bounded fix | 10 | 0 | 100% | $0.1466 | $0.1488 | 0.99 | 6 every time | 2 every time |
| B — feature slice | 9 | 1 | 100% | $0.2341 | $0.2368 | 0.99 | 6–8 | 2 every time |
| C — design latitude | 10 | 0 | 100% | $0.4256 | $0.2805 | 1.52 | 10–19 | 2 every time |

Total notional cost of all 30 sittings: **$8.65**.

Against the three clauses of the pre-registered bar, on point estimates:
every exam ≥ 90% **met**; no exam below 70% **met**; median cost ≤ 2× golden **met**.

## What the numbers say, and what they do not

**Deterministic assertions reproduced perfectly. Effort did not.** Every scored
sitting passed. Meanwhile cost spread widened monotonically with how much
latitude the task allowed — 1.08× on A, 1.28× on B, **2.70×** on C — and turn
counts on C ranged 10 to 19. The file footprint was identical across all 30.

This is the design's own assumption (§7) meeting evidence: variance is small at
the outcome and contract level and large at the token level. It is the result
the method needs, and it arrived without a pivot.

**Ten sittings cannot establish a 90% floor.** A 10/10 result has a Wilson 95%
interval of **[72%, 100%]**. The point estimate clears the bar; the data cannot
exclude a true pass rate below it. What these numbers support is "no failure
observed in ten replays," not "at least 90%." Distinguishing 90% from 99% needs
more sittings than a spike should buy, and the honest reading is the weaker one.

**The cost clause rests on a single draw.** The bar compares a ten-sitting
median against one golden session — itself one sample from the same
distribution. Exam C's golden sits at the **10th percentile** of its own
sittings, so most of that 1.52× is the anchor being cheap rather than the
replays being expensive. Exam A's golden sits at the 90th, flattering its ratio
the other way. On a high-variance exam the ratio is noisy in both directions.

**One error, correctly classified.** Exam B sitting 5 hit two permission denials
and was recorded `is_error=1`, excluded from the denominator rather than counted
as a failed assertion. Under an earlier version of the runner it would have been
recorded as a regression that never happened.

## Consequences for the design

- **Baseline-relative cost bands (§5 tier 2) cannot use one global `k`.** A band
  wide enough for C would be uselessly loose on A, where natural spread is 1.08×.
  The band belongs per exam, calibrated from that exam's own sittings, and the
  distiller's suggested `k` should come from a measured series rather than from
  the single golden session.
- **A golden session is a weak cost anchor.** Adopting the first passing series'
  median as the baseline — which §7's baseline policy already allows — is
  materially better than anchoring on one run.
- **Effort is the sensitive signal.** Correctness was saturated at this
  difficulty; cost and turns carried all the discrimination. That supports
  budget contracts being first-class rather than opt-in, and it is a caution
  that outcome assertions alone may be too coarse to detect early drift.

## Disclosures

These bear on how far the numbers generalise, and are stated with them rather
than in a footnote.

- **The exams were authored, not harvested.** The surviving local corpus
  contained no single-goal session — every one was a multi-issue sweep, a
  continuation, or a plan execution, with one spanning ten git branches. Exams
  written to be single-goal are cleaner than exams distilled from real work, so
  this is an optimistic bound.
- **The golden sessions were agent-steered, not human-steered**, each completing
  in one turn without correction. A human-steered session would cost more, which
  would loosen the 2× comparison.
- **Hermeticity was hand-built.** `--bare` forces API-key auth and these sittings
  ran on a subscription, so isolation came from an empty settings file,
  `--setting-sources project`, and a clean worktree. Ambient toolchain state was
  not controlled. An earlier run of exam A's golden was discarded after its
  transcript showed user-level plugins had loaded, including one that injects a
  growing store of past observations — that would have been recorded as model
  variance.
- **Costs are notional.** Both sides of every ratio come from the harness's own
  accounting under a subscription, so they are consistent with each other but
  are not billed amounts.
- **Every sitting resolved two models** — `claude-opus-4-8` for the work and
  `claude-haiku-4-5` for auxiliary calls. Both are recorded. Reading only the
  first would have attributed these results to the wrong model.
