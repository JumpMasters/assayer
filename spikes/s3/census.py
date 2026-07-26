#!/usr/bin/env python3
"""Distillability census over this machine's own Claude Code transcripts.

S3 rehearses one session end to end. This asks how representative that session is,
on the axes the design's distillability score proposes: how much steering a session
carries, how much of that steering is a pointer or an approval rather than a task
statement, how often the agent had to ask a question mid-session, and where the
instruction's content actually lives.

Counts only. No transcript content is written to disk or to stdout — the output is
numbers and file-path *categories*, never text the user wrote. Run:

    python3 spikes/s3/census.py            # table to stdout
    python3 spikes/s3/census.py out.json   # plus a JSON dump of the per-session rows
"""

from __future__ import annotations

import glob
import json
import os
import re
import statistics
import sys

ROOT = os.path.expanduser("~/.claude/projects")

# Transcript directories that are not a human working on a repo: the observer's own
# sessions, throwaway scratchpads, and the per-replay worktrees S1 and S3 create.
SKIP_DIR = re.compile(r"^-private-|scratchpad|-wt$|Test-temp")

# User records that are not something a person typed: slash-command echoes, background
# task notifications, hook output, and the harness's own reminders.
NOISE = re.compile(
    r"^\s*<(command-name|local-command-stdout|command-message|task-notification"
    r"|system-reminder|user-prompt-submit-hook|bash-input|bash-stdout|ide_)"
)

# Where an instruction's content can live besides the transcript. The first two are
# gitignored in the repos they belong to; the third is outside any repo.
PROVENANCE = {
    "assistant-memory": re.compile(r"/\.claude/projects/[^/]+/memory/"),
    "gitignored-docs": re.compile(r"(docs/superpowers/|/\.superpowers/)"),
}

# A turn at or below this many words is treated as a pointer or an approval rather
# than a task statement: "1", "go", "plan 15", "pr admin merge", "squash-merge and
# clean up". The threshold is arbitrary and the full distribution is printed too, so
# the reading does not rest on it.
POINTER_WORDS = 4


def human_turns(records: list[dict]) -> list[str]:
    """The messages a person actually typed, in order."""
    out: list[str] = []
    for m in records:
        if m.get("type") != "user" or m.get("isMeta") or m.get("isCompactSummary"):
            continue
        content = (m.get("message") or {}).get("content")
        if isinstance(content, list):
            parts = [
                b.get("text", "")
                for b in content
                if isinstance(b, dict) and b.get("type") == "text"
            ]
            if not parts:  # a tool_result only — not a typed message
                continue
            content = "\n".join(parts)
        if not isinstance(content, str):
            continue
        text = content.strip()
        if not text or NOISE.match(text) or text.startswith("Caveat:"):
            continue
        if text.startswith("[Request interrupted"):
            continue
        out.append(text)
    return out


def scan(path: str) -> dict | None:
    try:
        with open(path) as fh:
            records = [json.loads(line) for line in fh if line.strip()]
    except (OSError, ValueError):
        return None

    turns = human_turns(records)
    if not turns:
        return None

    asks = edits = tools = 0
    provenance = {k: 0 for k in PROVENANCE}
    versions: set[str] = set()
    cwds: set[str] = set()
    for m in records:
        if m.get("version"):
            versions.add(m["version"])
        if m.get("cwd"):
            cwds.add(m["cwd"])
        if m.get("type") != "assistant":
            continue
        for block in (m.get("message") or {}).get("content") or []:
            if not (isinstance(block, dict) and block.get("type") == "tool_use"):
                continue
            tools += 1
            name = block.get("name")
            if name == "AskUserQuestion":
                asks += 1
            if name in ("Edit", "Write", "NotebookEdit"):
                edits += 1
            blob = json.dumps(block.get("input", ""))
            for label, pattern in PROVENANCE.items():
                if pattern.search(blob):
                    provenance[label] += 1

    words = [len(t.split()) for t in turns]
    return {
        "session": os.path.basename(path)[:8],
        "cwd": sorted(cwds)[0] if cwds else "",
        "turns": len(turns),
        "pointer_turns": sum(1 for w in words if w <= POINTER_WORDS),
        "median_words": statistics.median(words),
        "asks": asks,
        "edits": edits,
        "tools": tools,
        "versions": sorted(versions),
        **{f"prov_{k}": v for k, v in provenance.items()},
    }


def main() -> None:
    rows = []
    for project in sorted(os.listdir(ROOT)):
        if SKIP_DIR.search(project):
            continue
        for path in glob.glob(os.path.join(ROOT, project, "*.jsonl")):
            row = scan(path)
            # Sessions that never edited a file are conversations, not work.
            if row and row["edits"] > 0:
                rows.append(row)

    if not rows:
        print("no coding sessions found", file=sys.stderr)
        raise SystemExit(1)

    rows.sort(key=lambda r: -r["turns"])
    total_turns = sum(r["turns"] for r in rows)
    total_pointer = sum(r["pointer_turns"] for r in rows)

    print(f"{len(rows)} coding sessions (>=1 file edit) across "
          f"{len({r['cwd'] for r in rows})} working directories\n")
    print(f"{'repo':28} {'sess':9} {'turns':>5} {'ptr':>4} {'med':>4} {'ask':>4} {'edit':>5} {'mem':>4} {'ign':>4}")
    for r in rows[:25]:
        print(f"{os.path.basename(r['cwd'])[:28]:28} {r['session']:9} {r['turns']:5} "
              f"{r['pointer_turns']:4} {r['median_words']:4.0f} {r['asks']:4} "
              f"{r['edits']:5} {r['prov_assistant-memory']:4} {r['prov_gitignored-docs']:4}")
    if len(rows) > 25:
        print(f"... {len(rows) - 25} more")

    med_words = statistics.median([r["median_words"] for r in rows])
    print(f"\nhuman turns: {total_turns}; of those {total_pointer} "
          f"({100 * total_pointer / total_turns:.0f}%) are <= {POINTER_WORDS} words")
    print(f"median session's median turn length: {med_words:.0f} words")
    print(f"sessions where the agent asked a question: "
          f"{sum(1 for r in rows if r['asks'] > 0)} of {len(rows)}")
    print(f"sessions touching assistant memory: "
          f"{sum(1 for r in rows if r['prov_assistant-memory'] > 0)}")
    print(f"sessions touching gitignored design docs: "
          f"{sum(1 for r in rows if r['prov_gitignored-docs'] > 0)}")
    versions = sorted({v for r in rows for v in r["versions"]})
    print(f"harness versions represented: {len(versions)} "
          f"({versions[0]} … {versions[-1]})")

    if len(sys.argv) > 1:
        with open(sys.argv[1], "w") as fh:
            json.dump(rows, fh, indent=1)
        print(f"\nper-session rows written to {sys.argv[1]}")


if __name__ == "__main__":
    main()
