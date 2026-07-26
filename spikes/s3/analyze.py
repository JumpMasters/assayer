"""Summarise the S3 replays against the pre-registered bar.

The bar was fixed in advance in `README.md` and has three clauses:

    1. Reproduction   — at least 8 of 10 scored replays pass both the
                        fail-to-pass set and the no-regression suite.
    2. Non-interactivity — no replay stalls asking a clarifying question.
    3. Recoverability — every clause of the distilled instruction traces to the
                        transcript or to the repository at the base SHA.

Clauses 1 and 2 are computed here. Clause 3 is a documented trace, not a
statistic; it lives in `exam/provenance.md` and is reported here only as a
pointer, so nobody mistakes this script for having checked it.

Unlike S1's analyzer there are no confidence bounds. Ten replays are a go/no-go,
not a distribution, and the pre-registration says so; printing an interval here
would dress up a sample that cannot carry one.

Errors are excluded from the pass-rate denominator and counted separately — a
harness crash, a spend cap or a timeout is not evidence about distillation. What
counts as an error is narrower here than in S1: see README.md ("Amendment").

Standard library only, by design.
"""

from __future__ import annotations

import csv
import json
import pathlib
import statistics
import sys

SPIKE = pathlib.Path(__file__).parent
# Two arms of the same exam. The main arm pins a worktree of the real repository, where
# the golden commit is reachable; the control arm pins a history-stripped copy where it
# is not. See README.md ("The control arm") — the conclusion rests on the control.
ARMS = {
    "main (golden commit reachable)": ("sittings.csv", "s3"),
    "control (history stripped)": ("control.csv", "s3ctl"),
}
# The runner derives its own directory for raw harness output, so S3's lands under S1.
RAW = SPIKE.parent / "s1" / "results" / "raw"

REPRODUCTION_FLOOR = 8   # of 10 scored replays
PLANNED_N = 10

# A headless replay cannot call AskUserQuestion — it is not on the allowlist — so a
# sitting that wants clarification ends its turn asking in prose instead. These are a
# screen, not a verdict: every hit is read by hand before it is reported.
QUESTION_MARKERS = (
    "?",
    "could you clarify",
    "let me know",
    "which would you",
    "should i",
    "before i proceed",
    "please confirm",
)


def rows(csv_name: str) -> list[dict]:
    path = SPIKE / "results" / csv_name
    if not path.exists():
        return []
    with path.open() as fh:
        return list(csv.DictReader(fh))


def screen_for_questions(sittings: list[dict], exam_id: str) -> list[tuple[str, str]]:
    """Replays whose final message looks like a question back to the user.

    Reads local-only raw harness output. Returns (replay, short excerpt) so the
    finding can be confirmed by hand; the excerpt is for the operator's terminal
    and is never written into a committed document.
    """
    hits = []
    for row in sittings:
        path = RAW / f"{exam_id}-{row['n']}.json"
        if not path.exists():
            continue
        try:
            result = (json.loads(path.read_text()).get("result") or "").strip()
        except (OSError, ValueError):
            continue
        tail = result[-400:].lower()
        if any(marker in tail for marker in QUESTION_MARKERS):
            hits.append((row["n"], result[-200:].replace("\n", " ")))
    return hits


def describe(values: list[float], label: str, fmt: str = ".3f") -> None:
    if not values:
        print(f"  {label:<10} —")
        return
    med = statistics.median(values)
    lo, hi = min(values), max(values)
    spread = f"{hi / lo:.2f}x" if lo > 0 else "—"
    print(f"  {label:<10} median {med:{fmt}}  min {lo:{fmt}}  max {hi:{fmt}}  max/min {spread}")


def report(label: str, csv_name: str, exam_id: str) -> int:
    """Print one arm's reading. Returns the number of replays that passed both sets."""
    sittings = rows(csv_name)
    if not sittings:
        print(f"\n=== {label} ===\n  not run")
        return -1
    errored = [r for r in sittings if r["is_error"] == "1"]
    scored = [r for r in sittings if r["is_error"] != "1"]
    passed = [r for r in scored if r["f2p_pass"] == "1" and r["p2p_pass"] == "1"]
    f2p_only = [r for r in scored if r["f2p_pass"] == "1" and r["p2p_pass"] != "1"]

    print(f"\n=== {label} — {len(sittings)} replays (planned {PLANNED_N}) ===\n")
    print(f"  scored      {len(scored)}")
    print(f"  errored     {len(errored)}"
          + (f"  ({', '.join(sorted({r['stop_reason'] for r in errored}))})" if errored else ""))
    print(f"  passed both {len(passed)}")
    print(f"  f2p only    {len(f2p_only)}   (fail-to-pass green, no-regression suite red)")

    print("\nClause 1 — reproduction")
    verdict = "MET" if len(passed) >= REPRODUCTION_FLOOR else "NOT MET"
    print(f"  {len(passed)} of {len(scored)} scored replays passed both sets; "
          f"bar is {REPRODUCTION_FLOOR} of {PLANNED_N} → {verdict}")
    if len(scored) < PLANNED_N:
        print(f"  note: {PLANNED_N - len(scored)} replay(s) produced no verdict, so the "
              f"denominator is short of the planned {PLANNED_N}")

    print("\nClause 2 — non-interactivity")
    hits = screen_for_questions(sittings, exam_id)
    if not hits:
        print("  no replay's final message matched the clarifying-question screen")
    else:
        print(f"  {len(hits)} replay(s) matched the screen — confirm each by hand:")
        for n, excerpt in hits:
            print(f"    replay {n}: …{excerpt}")

    print("\nRecorded, not gating")
    describe([float(r["cost_usd"]) for r in scored], "cost usd")
    describe([float(r["num_turns"]) for r in scored], "turns", ".0f")
    describe([float(r["touched_files"]) for r in scored], "touched", ".0f")
    describe([float(r["wall_ms"]) / 1000 for r in scored], "wall s", ".0f")
    total = sum(float(r["cost_usd"]) for r in sittings)
    print(f"\n  notional spend, this arm: ${total:.2f}")
    return len(passed)


def main() -> None:
    print("S3 — distillation rehearsal")
    results = {
        label: report(label, csv_name, exam_id)
        for label, (csv_name, exam_id) in ARMS.items()
    }

    print("\nClause 3 — recoverability")
    print("  not a statistic; see exam/provenance.md for the clause-by-clause trace")

    main_n, ctl_n = (results.get(k, -1) for k in ARMS)
    print("\nReading")
    if ctl_n < 0:
        print("  control arm not run — the main arm alone cannot separate a distilled")
        print("  instruction carrying the work from a replay reading the golden commit")
    elif ctl_n >= REPRODUCTION_FLOOR:
        print(f"  the control arm passed {ctl_n}, so the result does not depend on the")
        print("  golden commit being reachable; the main arm's {} is consistent with it"
              .format(main_n))
    else:
        print(f"  main arm {main_n}, control arm {ctl_n}: the gap is the measure of how")
        print("  much of the main arm was the reachable solution rather than the")
        print("  instruction. The control arm is the result.")

    total = sum(
        float(r["cost_usd"])
        for csv_name, _ in ARMS.values()
        for r in rows(csv_name)
    )
    print(f"\n  total notional spend, both arms: ${total:.2f}")


if __name__ == "__main__":
    main()
