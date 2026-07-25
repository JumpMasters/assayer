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

Standard library only, by design — this runs anywhere the spike does.
"""

from __future__ import annotations

import csv
import json
import pathlib
import statistics
import sys

SPIKE = pathlib.Path(__file__).parent
PER_EXAM_FLOOR = 0.90
ABSOLUTE_FLOOR = 0.70
COST_MULTIPLE = 2.0


def main() -> int:
    rows = list(csv.DictReader((SPIKE / "results/sittings.csv").open()))
    if not rows:
        print("no sittings recorded", file=sys.stderr)
        return 2

    print(
        f"{'exam':<6}{'n':<4}{'err':<5}{'tier1/2':<10}"
        f"{'cost med':<11}{'golden':<11}{'ratio':<8}{'spread':<9}{'turns':<9}{'touched'}"
    )

    clause_90 = True
    clause_70 = True
    clause_cost = True

    for exam in sorted({r["exam"] for r in rows}):
        ex = [r for r in rows if r["exam"] == exam]
        errored = [r for r in ex if r["is_error"] == "1"]
        # ERROR is not FAIL. An errored sitting produced no verdict, so it
        # leaves the denominator rather than counting against the model.
        scored = [r for r in ex if r["is_error"] != "1"]
        passed = [r for r in scored if r["f2p_pass"] == "1" and r["p2p_pass"] == "1"]

        rate = len(passed) / len(scored) if scored else 0.0
        costs = [float(r["cost_usd"]) for r in scored]
        median = statistics.median(costs) if costs else 0.0
        golden = json.loads((SPIKE / f"exams/{exam}/golden.json").read_text())["cost_usd"]
        ratio = median / golden if golden else float("inf")
        turns = sorted(int(r["num_turns"]) for r in scored)
        touched = sorted({r["touched_files"] for r in scored})
        spread = max(costs) / min(costs) if costs and min(costs) else 0.0

        turn_range = f"{turns[0]}" if turns[0] == turns[-1] else f"{turns[0]}-{turns[-1]}"
        print(
            f"{exam:<6}{len(ex):<4}{len(errored):<5}{rate:<10.0%}"
            f"{median:<11.4f}{golden:<11.4f}{ratio:<8.2f}"
            f"{spread:<9.2f}{turn_range:<9}{','.join(touched)}"
        )

        if rate < PER_EXAM_FLOOR:
            clause_90 = False
        if rate < ABSOLUTE_FLOOR:
            clause_70 = False
        if ratio > COST_MULTIPLE:
            clause_cost = False

    def verdict(ok: bool) -> str:
        return "MET" if ok else "MISSED"

    print()
    print(f"  every exam >= 90% ......... {verdict(clause_90)}")
    print(f"  no exam below 70% ......... {verdict(clause_70)}")
    print(f"  median cost <= 2x golden .. {verdict(clause_cost)}")
    print()

    if clause_90 and clause_70 and clause_cost:
        print("BAR MET")
        return 0
    print("BAR MISSED — the design pivots before any product code is written")
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
