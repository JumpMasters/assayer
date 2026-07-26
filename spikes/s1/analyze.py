"""Summarise the S1 sittings against the pre-registered bar.

The bar was fixed in advance and is reproduced in the README's Status section:

    Across three calibration exams replayed ten times each under fixed
    conditions, each exam must reproduce its deterministic assertions at least
    90% of the time, with no exam below 70%, and a median replay must cost no
    more than twice the session it was distilled from.

Its three clauses are reported separately, so a partial miss is visible rather
than averaged away. Errors are excluded from the pass-rate denominator and
counted on their own line: a harness crash, a spend cap, or a broken
environment is not evidence about the model.

Every statistic published in the README or in `results/summary.md` is computed
here, so the documents can be regenerated rather than transcribed. Confidence
bounds print to three decimal places on purpose: a lower bound rounded up to a
whole percent reads as clearing a floor it does not clear.

Standard library only, by design — this runs anywhere the spike does.
"""

from __future__ import annotations

import csv
import json
import math
import pathlib
import statistics
import sys

SPIKE = pathlib.Path(__file__).parent
PER_EXAM_FLOOR = 0.90
ABSOLUTE_FLOOR = 0.70
COST_MULTIPLE = 2.0
Z = 1.96


def wilson(k: int, n: int) -> tuple[float, float]:
    """Wilson score interval. Preferred over the normal approximation, which
    returns a degenerate zero-width interval when every trial passes."""
    if n == 0:
        return (0.0, 0.0)
    p = k / n
    denom = 1 + Z * Z / n
    centre = (p + Z * Z / (2 * n)) / denom
    half = Z * math.sqrt(p * (1 - p) / n + Z * Z / (4 * n * n)) / denom
    return (max(0.0, centre - half), min(1.0, centre + half))


def dispersion(costs: list[float]) -> dict[str, float]:
    """Robust spread measures. Max-over-min is reported last because a single
    outlying replay moves it; it is not a description of the distribution.

    Quantiles use the exclusive method (the standard library default). Other
    conventions give different p90/p10 figures, so the choice is stated rather
    than assumed."""
    quart = statistics.quantiles(costs, n=4, method="exclusive")
    pcts = statistics.quantiles(costs, n=100, method="exclusive")
    med = statistics.median(costs)
    return {
        "cv": statistics.stdev(costs) / statistics.mean(costs),
        "iqr_med": (quart[2] - quart[0]) / med,
        "p90_p10": pcts[89] / pcts[9],
        "max_min": max(costs) / min(costs),
    }


def main() -> int:
    rows = list(csv.DictReader((SPIKE / "results/sittings.csv").open()))
    if not rows:
        print("no sittings recorded", file=sys.stderr)
        return 2

    exams = sorted({r["exam"] for r in rows})
    clause_90 = clause_70 = clause_cost = True
    clearing_floor = []

    print("REPRODUCTION")
    print(
        f"  {'exam':<6}{'replays':<9}{'err':<5}{'scored':<8}{'passed':<8}"
        f"{'rate':<9}{'95% lower':<11}{'95% upper'}"
    )
    for exam in exams:
        ex = [r for r in rows if r["exam"] == exam]
        # ERROR is not FAIL. An errored replay produced no verdict, so it leaves
        # the denominator rather than counting against the model.
        scored = [r for r in ex if r["is_error"] != "1"]
        passed = [r for r in scored if r["f2p_pass"] == "1" and r["p2p_pass"] == "1"]
        rate = len(passed) / len(scored) if scored else 0.0
        lo, hi = wilson(len(passed), len(scored))
        if lo >= PER_EXAM_FLOOR:
            clearing_floor.append(exam)
        if rate < PER_EXAM_FLOOR:
            clause_90 = False
        if rate < ABSOLUTE_FLOOR:
            clause_70 = False
        print(
            f"  {exam:<6}{len(ex):<9}{len(ex) - len(scored):<5}{len(scored):<8}"
            f"{len(passed):<8}{rate:<9.1%}{lo:<11.3%}{hi:.3%}"
        )

    print()
    print("COST")
    print(
        f"  {'exam':<6}{'median':<10}{'golden':<10}{'ratio':<9}"
        f"{'CV':<9}{'IQR/med':<10}{'p90/p10':<10}{'max/min'}"
    )
    for exam in exams:
        scored = [r for r in rows if r["exam"] == exam and r["is_error"] != "1"]
        costs = [float(r["cost_usd"]) for r in scored]
        median = statistics.median(costs)
        golden = json.loads((SPIKE / f"exams/{exam}/golden.json").read_text())["cost_usd"]
        ratio = median / golden if golden else float("inf")
        if ratio > COST_MULTIPLE:
            clause_cost = False
        d = dispersion(costs)
        print(
            f"  {exam:<6}{median:<10.4f}{golden:<10.4f}{ratio:<9.2f}"
            f"{d['cv']:<9.3f}{d['iqr_med']:<10.3f}{d['p90_p10']:<10.2f}{d['max_min']:.2f}"
        )

    scored_all = [r for r in rows if r["is_error"] != "1"]
    passed_all = [r for r in scored_all if r["f2p_pass"] == "1" and r["p2p_pass"] == "1"]
    errors = len(rows) - len(scored_all)
    total = sum(float(r["cost_usd"]) for r in rows)

    print()
    print(
        f"  {len(rows)} replays, {errors} errors ({errors / len(rows):.1%}), "
        f"{len(passed_all)} of {len(scored_all)} scored reproduced, ${total:.2f} total"
    )

    print()
    print("THE PRE-REGISTERED BAR, on point estimates")
    print(f"  every exam >= 90% ......... {'MET' if clause_90 else 'MISSED'}")
    print(f"  no exam below 70% ......... {'MET' if clause_70 else 'MISSED'}")
    print(f"  median cost <= 2x golden .. {'MET' if clause_cost else 'MISSED'}")
    print()
    print(
        f"  Exams whose 95% LOWER BOUND also reaches 90%: "
        f"{len(clearing_floor)} of {len(exams)}"
        + (f" ({', '.join(clearing_floor)})" if clearing_floor else "")
    )
    print("  A point estimate meeting the floor is not the same as establishing it.")

    print()
    if clause_90 and clause_70 and clause_cost:
        print("BAR MET (point estimates)")
        return 0
    print("BAR MISSED — the design pivots before any product code is written")
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
