#!/usr/bin/env python3
"""Transcript schema census across harness versions.

S2 asks whether transcript schema churn across harness releases threatens the capture
adapter. This measures the churn: for every harness version present in the local store,
what record types appear, what keys each type carries, how sidechains and subagents are
represented, and how tool results are offloaded.

Shape only. The output is key names, type names and counts — never a value from anyone's
transcript. Key names are schema, not content.

    python3 spikes/s2/schema-census.py            # table to stdout
    python3 spikes/s2/schema-census.py out.json   # plus the full per-version shape
"""

from __future__ import annotations

import collections
import json
import os
import pathlib
import sys

ROOT = pathlib.Path(os.path.expanduser("~/.claude/projects"))

# Keys whose presence marks a structural era rather than a per-record detail.
ERA_MARKERS = ("isSidechain", "isMeta", "isCompactSummary", "subtype", "parentUuid")


def version_key(v: str) -> tuple:
    """Sortable form of a dotted harness version."""
    try:
        return tuple(int(p) for p in v.split("."))
    except ValueError:
        return (0,)


class Shape:
    def __init__(self) -> None:
        self.records = 0
        self.files = set()
        self.type_keys: dict[str, set[str]] = collections.defaultdict(set)
        self.type_counts: collections.Counter = collections.Counter()
        self.block_types: collections.Counter = collections.Counter()
        self.era: collections.Counter = collections.Counter()
        self.tool_result_refs = 0
        self.inline_sidechain = 0

    def add(self, rec: dict, path: pathlib.Path) -> None:
        self.records += 1
        self.files.add(path)
        rtype = rec.get("type", "<none>")
        self.type_counts[rtype] += 1
        self.type_keys[rtype] |= set(rec.keys())
        for marker in ERA_MARKERS:
            if marker in rec:
                self.era[marker] += 1
        if rec.get("isSidechain"):
            self.inline_sidechain += 1
        content = (rec.get("message") or {}).get("content")
        if isinstance(content, list):
            for block in content:
                if isinstance(block, dict):
                    self.block_types[block.get("type", "<none>")] += 1
        elif isinstance(content, str):
            self.block_types["<string>"] += 1
        # An offloaded tool result is referenced rather than inlined.
        blob = rec.get("toolUseResult")
        if isinstance(blob, dict) and any(
            k in blob for k in ("file", "filePath", "outputFile")
        ):
            self.tool_result_refs += 1


def sidecar_shape() -> collections.Counter:
    """What sits beside a transcript: subagent sidecars, offloaded tool results."""
    kinds: collections.Counter = collections.Counter()
    for project in ROOT.iterdir():
        if not project.is_dir():
            continue
        for entry in project.iterdir():
            if entry.is_dir():
                kinds["session-dir"] += 1
                for sub in entry.iterdir():
                    if sub.is_dir():
                        kinds[f"session-dir/{sub.name}"] += 1
    return kinds


def main() -> None:
    by_version: dict[str, Shape] = collections.defaultdict(Shape)
    unversioned = Shape()
    files = sorted(ROOT.glob("*/*.jsonl")) + sorted(ROOT.glob("*/*/*/*.jsonl"))
    bad_lines = 0

    for path in files:
        try:
            with path.open() as fh:
                for line in fh:
                    if not line.strip():
                        continue
                    try:
                        rec = json.loads(line)
                    except ValueError:
                        bad_lines += 1
                        continue
                    if not isinstance(rec, dict):
                        bad_lines += 1
                        continue
                    v = rec.get("version")
                    (by_version[v] if v else unversioned).add(rec, path)
        except OSError:
            continue

    versions = sorted(by_version, key=version_key)
    print(f"{len(files)} transcript files, "
          f"{sum(s.records for s in by_version.values()) + unversioned.records} records, "
          f"{len(versions)} harness versions\n")

    print(f"{'version':10} {'files':>6} {'records':>9} {'inline-sc':>10} {'offload':>8}  record types")
    for v in versions:
        s = by_version[v]
        types = ",".join(f"{t}:{n}" for t, n in s.type_counts.most_common(4))
        print(f"{v:10} {len(s.files):6} {s.records:9} {s.inline_sidechain:10} "
              f"{s.tool_result_refs:8}  {types}")
    if unversioned.records:
        print(f"{'<none>':10} {len(unversioned.files):6} {unversioned.records:9} "
              f"{unversioned.inline_sidechain:10} {unversioned.tool_result_refs:8}  "
              + ",".join(f"{t}:{n}" for t, n in unversioned.type_counts.most_common(4)))

    # The question the adapter actually turns on: does the record shape differ between
    # versions in ways a single parser would have to branch on?
    print("\nrecord types, union across all versions:")
    all_types: collections.Counter = collections.Counter()
    for s in by_version.values():
        all_types.update(s.type_counts)
    for t, n in all_types.most_common():
        present = sum(1 for v in versions if t in by_version[v].type_counts)
        print(f"  {t:24} {n:9} records, present in {present}/{len(versions)} versions")

    print("\ncontent block types:")
    all_blocks: collections.Counter = collections.Counter()
    for s in by_version.values():
        all_blocks.update(s.block_types)
    for t, n in all_blocks.most_common():
        present = sum(1 for v in versions if t in by_version[v].block_types)
        print(f"  {t:24} {n:9} blocks,  present in {present}/{len(versions)} versions")

    print("\nkeys per record type — union, and how many versions carry each:")
    for rtype in sorted(all_types):
        keys: collections.Counter = collections.Counter()
        for v in versions:
            for k in by_version[v].type_keys.get(rtype, ()):
                keys[k] += 1
        universal = sorted(k for k, c in keys.items() if c == len(versions))
        partial = sorted((k, c) for k, c in keys.items() if c < len(versions))
        print(f"\n  [{rtype}]")
        print(f"    in every version: {', '.join(universal) or '—'}")
        if partial:
            print("    version-dependent: "
                  + ", ".join(f"{k}({c}/{len(versions)})" for k, c in partial))

    print("\nsidecars on disk:")
    for kind, n in sidecar_shape().most_common():
        print(f"  {kind:28} {n}")

    if bad_lines:
        print(f"\nunparseable lines: {bad_lines}")

    if len(sys.argv) > 1:
        dump = {
            v: {
                "files": len(by_version[v].files),
                "records": by_version[v].records,
                "type_counts": dict(by_version[v].type_counts),
                "type_keys": {t: sorted(k) for t, k in by_version[v].type_keys.items()},
                "block_types": dict(by_version[v].block_types),
                "inline_sidechain": by_version[v].inline_sidechain,
                "tool_result_refs": by_version[v].tool_result_refs,
            }
            for v in versions
        }
        pathlib.Path(sys.argv[1]).write_text(json.dumps(dump, indent=1))
        print(f"\nper-version shape written to {sys.argv[1]}")


if __name__ == "__main__":
    main()
