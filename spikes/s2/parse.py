#!/usr/bin/env python3
"""A version-keyed transcript parser, and the conformance run over the local corpus.

This is a measuring instrument, not the product's capture adapter. It extracts exactly
what S3 showed the Distiller needs — human turns, tool calls with their inputs, tool
results, model identity, working directory, and the link from a subagent sidecar back to
the tool call that spawned it — and nothing else.

It exists to answer S2's question: does schema churn across harness releases break a
single parser, and can it fail closed rather than silently misparse? The bar it is read
against is in README.md and was committed before this ran.

No transcript content is printed or written. Output is counts, ids, versions and file
paths. Values from the user's sessions stay in memory and are dropped.

    python3 spikes/s2/parse.py                  # conformance run + invariant report
    python3 spikes/s2/parse.py --selftest       # fail-closed behaviour, no corpus needed
"""

from __future__ import annotations

import collections
import dataclasses
import json
import os
import pathlib
import sys

ROOT = pathlib.Path(os.path.expanduser("~/.claude/projects"))

# The versions this parser claims support for. Fail-closed by design: a release not on
# this list is refused rather than guessed at, which is the design's UNSUPPORTED-VERSION
# rule. The operational cost of that rule is measured in results/summary.md.
KNOWN_VERSIONS = frozenset({
    "2.1.142", "2.1.149", "2.1.156", "2.1.170", "2.1.177", "2.1.181",
    "2.1.190", "2.1.191", "2.1.197", "2.1.198", "2.1.199", "2.1.201",
    "2.1.209", "2.1.210", "2.1.211", "2.1.217", "2.1.219", "2.1.220",
})

# Records that carry the session's content. Every one of these was observed to carry a
# `version` in all 18 versions surveyed, without exception.
CONTENT_TYPES = frozenset({"assistant", "user", "attachment", "system"})

# Records that never carry a version. These are bookkeeping, not content: skipping them
# is correct, and refusing them as UNSUPPORTED-VERSION would reject every real
# transcript. Listed explicitly rather than inferred from a missing field, so that a
# genuinely new versionless *content* type is a loud failure and not a silent skip.
BOOKKEEPING_TYPES = frozenset({
    "queue-operation", "last-prompt", "ai-title", "mode", "pr-link",
    "file-history-snapshot", "bridge-session", "permission-mode",
    "file-history-delta", "worktree-state", "custom-title", "fork-context-ref",
})


class UnsupportedVersion(Exception):
    """A content record from a release this parser does not claim to support."""

    def __init__(self, version: str, path: pathlib.Path) -> None:
        super().__init__(f"UNSUPPORTED-VERSION {version} in {path}")
        self.version = version
        self.path = path


class UnknownRecordType(Exception):
    """A record type that is neither known content nor known bookkeeping.

    Deliberately fatal. A new versionless content type would otherwise be skipped
    silently and produce a session that looks complete and is not.
    """


@dataclasses.dataclass
class Session:
    path: pathlib.Path
    versions: set[str] = dataclasses.field(default_factory=set)
    session_ids: set[str] = dataclasses.field(default_factory=set)
    cwds: set[str] = dataclasses.field(default_factory=set)
    models: set[str] = dataclasses.field(default_factory=set)
    human_turns: int = 0
    tool_uses: dict[str, str] = dataclasses.field(default_factory=dict)   # id -> name
    tool_results: list[str] = dataclasses.field(default_factory=list)     # tool_use ids
    uuids: set[str] = dataclasses.field(default_factory=set)
    parents: list[tuple[str, str]] = dataclasses.field(default_factory=list)
    sidechain_records: int = 0
    skipped_bookkeeping: int = 0
    records: int = 0


def parse(path: pathlib.Path) -> Session:
    """Read one transcript. Raises rather than returning a partial session."""
    s = Session(path=path)
    with path.open() as fh:
        for lineno, line in enumerate(fh, 1):
            if not line.strip():
                continue
            try:
                rec = json.loads(line)
            except ValueError as exc:
                raise ValueError(f"{path}:{lineno} unparseable JSON") from exc
            if not isinstance(rec, dict):
                raise ValueError(f"{path}:{lineno} record is not an object")

            rtype = rec.get("type")
            if rtype in BOOKKEEPING_TYPES:
                s.skipped_bookkeeping += 1
                continue
            if rtype not in CONTENT_TYPES:
                raise UnknownRecordType(f"{path}:{lineno} unknown record type {rtype!r}")

            version = rec.get("version")
            if version not in KNOWN_VERSIONS:
                raise UnsupportedVersion(str(version), path)

            s.records += 1
            s.versions.add(version)
            for key in ("sessionId", "session_id"):
                if rec.get(key):
                    s.session_ids.add(rec[key])
            if rec.get("cwd"):
                s.cwds.add(rec["cwd"])
            if rec.get("isSidechain"):
                s.sidechain_records += 1
            if rec.get("uuid"):
                s.uuids.add(rec["uuid"])
            if rec.get("parentUuid"):
                s.parents.append((rec["uuid"], rec["parentUuid"]))

            message = rec.get("message") or {}
            if message.get("model"):
                s.models.add(message["model"])
            content = message.get("content")
            if rtype == "user" and _is_human_turn(rec, content):
                s.human_turns += 1
            if isinstance(content, list):
                for block in content:
                    if not isinstance(block, dict):
                        continue
                    if block.get("type") == "tool_use":
                        s.tool_uses[block.get("id", "")] = block.get("name", "")
                    elif block.get("type") == "tool_result":
                        s.tool_results.append(block.get("tool_use_id", ""))
    return s


def _is_human_turn(rec: dict, content) -> bool:
    """A message a person typed, as distinct from a tool result wearing the user role."""
    if rec.get("isMeta") or rec.get("isCompactSummary"):
        return False
    if isinstance(content, list):
        return any(
            isinstance(b, dict) and b.get("type") == "text" for b in content
        )
    return isinstance(content, str) and bool(content.strip())


def check_invariants(s: Session) -> list[str]:
    """Structural claims the Session IR would want to rest on. Reported, never assumed."""
    problems = []
    dangling = [t for t in s.tool_results if t and t not in s.tool_uses]
    if dangling:
        problems.append(f"tool_result without matching tool_use: {len(dangling)}")
    unresolved = [c for c, p in s.parents if p not in s.uuids]
    if unresolved:
        problems.append(f"parentUuid not resolvable in-file: {len(unresolved)}")
    if len(s.session_ids) > 1:
        problems.append(f"sessionId/session_id disagree: {len(s.session_ids)} distinct")
    if len(s.cwds) > 1:
        problems.append(f"multiple cwds in one transcript: {len(s.cwds)}")
    return problems


def subagent_links(session_file: pathlib.Path) -> tuple[int, int]:
    """(sidecars, sidecars whose spawning tool call is present in the parent)."""
    sidecar_dir = session_file.with_suffix("") / "subagents"
    if not sidecar_dir.is_dir():
        return (0, 0)
    try:
        parent_tool_ids = set(parse(session_file).tool_uses)
    except Exception:
        return (0, 0)
    total = linked = 0
    for meta in sidecar_dir.glob("*.meta.json"):
        total += 1
        try:
            if json.loads(meta.read_text()).get("toolUseId") in parent_tool_ids:
                linked += 1
        except (OSError, ValueError):
            pass
    return (total, linked)


def selftest() -> int:
    """Clause 3, both directions, without touching the corpus."""
    tmp = pathlib.Path(os.environ.get("TMPDIR", "/tmp")) / "s2-selftest.jsonl"
    ok = True

    def write(records: list[dict]) -> None:
        tmp.write_text("".join(json.dumps(r) + "\n" for r in records))

    known = sorted(KNOWN_VERSIONS)[0]
    base = {"type": "user", "version": known, "uuid": "u1",
            "message": {"role": "user", "content": "hello"}}

    # A content record from an unseen future release must be refused by name.
    write([dict(base, version="9.9.9")])
    try:
        parse(tmp)
        print("FAIL: unknown version was parsed as current"); ok = False
    except UnsupportedVersion as exc:
        print(f"pass: unknown version refused — {exc}")

    # A versionless bookkeeping record must not trigger that refusal.
    write([{"type": "queue-operation", "id": 1}, base])
    try:
        s = parse(tmp)
        assert s.skipped_bookkeeping == 1 and s.records == 1, s
        print("pass: versionless bookkeeping record admitted and skipped")
    except Exception as exc:
        print(f"FAIL: versionless bookkeeping record rejected — {exc}"); ok = False

    # A versionless record of an unknown type must be loud, not skipped.
    write([{"type": "something-new", "id": 1}])
    try:
        parse(tmp)
        print("FAIL: unknown record type silently skipped"); ok = False
    except UnknownRecordType:
        print("pass: unknown record type refused rather than skipped")

    # A dangling tool_result is reported by the invariant check, not dropped.
    write([dict(base, uuid="u2", message={"role": "user", "content": [
        {"type": "tool_result", "tool_use_id": "missing"}]})])
    problems = check_invariants(parse(tmp))
    if any("tool_result without matching" in p for p in problems):
        print("pass: dangling tool_result reported")
    else:
        print("FAIL: dangling tool_result not reported"); ok = False

    tmp.unlink(missing_ok=True)
    return 0 if ok else 1


def main() -> None:
    if "--selftest" in sys.argv:
        raise SystemExit(selftest())

    files = sorted(ROOT.glob("*/*.jsonl")) + sorted(ROOT.glob("*/*/*/*.jsonl"))
    parsed = 0
    by_version: collections.Counter = collections.Counter()
    failures: collections.Counter = collections.Counter()
    problem_counts: collections.Counter = collections.Counter()
    problem_files = 0
    totals: collections.Counter = collections.Counter()

    for path in files:
        try:
            s = parse(path)
        except UnsupportedVersion as exc:
            failures[f"UNSUPPORTED-VERSION {exc.version}"] += 1
            continue
        except UnknownRecordType:
            failures["unknown record type"] += 1
            continue
        except (ValueError, OSError) as exc:
            failures[type(exc).__name__] += 1
            continue
        parsed += 1
        for v in s.versions:
            by_version[v] += 1
        totals["records"] += s.records
        totals["bookkeeping skipped"] += s.skipped_bookkeeping
        totals["human turns"] += s.human_turns
        totals["tool uses"] += len(s.tool_uses)
        totals["tool results"] += len(s.tool_results)
        totals["sidechain records"] += s.sidechain_records
        problems = check_invariants(s)
        if problems:
            problem_files += 1
            for p in problems:
                problem_counts[p.split(":")[0]] += 1

    print(f"S2 — parser conformance over {len(files)} transcript files\n")
    print(f"  parsed        {parsed}  ({100 * parsed / len(files):.2f}%)")
    print(f"  refused       {sum(failures.values())}")
    for reason, n in failures.most_common():
        print(f"      {reason}: {n}")

    print("\n  extracted:")
    for k, v in totals.most_common():
        print(f"      {k:22} {v}")

    print(f"\n  files with an invariant violation: {problem_files} of {parsed}")
    for p, n in problem_counts.most_common():
        print(f"      {p}: {n} files")

    print(f"\n  versions read: {len(by_version)}")
    for v in sorted(by_version, key=lambda x: tuple(int(p) for p in x.split("."))):
        print(f"      {v:10} {by_version[v]:5} files")

    total_side = total_linked = 0
    for path in sorted(ROOT.glob("*/*.jsonl")):
        t, l = subagent_links(path)
        total_side += t
        total_linked += l
    if total_side:
        print(f"\n  subagent sidecars: {total_side}, of which {total_linked} "
              f"({100 * total_linked / total_side:.1f}%) resolve to a tool call "
              f"in the parent transcript")


if __name__ == "__main__":
    main()
