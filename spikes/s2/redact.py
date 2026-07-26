#!/usr/bin/env python3
"""Secret detection for transcript fixtures, and its measurement.

No fixture can ship unredacted. The design claims a planted-secret corpus is caught at
100%; this builds that corpus and measures the claim, then scans the real local store to
see whether the problem is hypothetical.

An honest limitation up front: a planted corpus written by the same hand as the patterns
mostly measures whether the patterns catch their own author's imagination. The plants
below therefore include shapes chosen to be awkward for the detector — secrets inside
JSON string escapes, inside env dumps, split across a line boundary, and base64-wrapped —
and the real-corpus scan is reported alongside, because that is the part the author did
not get to choose.

    python3 spikes/s2/redact.py --measure   # planted-corpus catch rate + false positives
    python3 spikes/s2/redact.py --scan      # counts of hits in the local store (no values)
"""

from __future__ import annotations

import base64
import collections
import json
import os
import pathlib
import re
import sys

ROOT = pathlib.Path(os.path.expanduser("~/.claude/projects"))

# Ordered most-specific first. Each is a vendor-documented prefix or an unambiguous
# structural form; generic high-entropy matching is deliberately absent, because on
# transcripts full of code and hashes it produces more noise than signal.
PATTERNS: list[tuple[str, re.Pattern]] = [
    ("anthropic-key", re.compile(r"sk-ant-(?:api|oat)\w{2}-[\w\-]{20,}")),
    # \b matters: without it the word "risk-" satisfies `sk-` + 32 word characters, and
    # every "2026-03-27-risk-disclosure.md" in the corpus scored as an API key.
    ("openai-key", re.compile(r"\bsk-(?:proj-)?[A-Za-z0-9_\-]{32,}")),
    ("github-token", re.compile(r"gh[pousr]_[A-Za-z0-9]{36,}|github_pat_[A-Za-z0-9_]{50,}")),
    ("aws-access-key", re.compile(r"\b(?:AKIA|ASIA)[0-9A-Z]{16}\b")),
    ("google-api-key", re.compile(r"\bAIza[0-9A-Za-z_\-]{35}\b")),
    ("slack-token", re.compile(r"\bxox[baprs]-[0-9A-Za-z\-]{10,}")),
    ("stripe-key", re.compile(r"\b[rs]k_live_[0-9a-zA-Z]{20,}")),
    ("private-key", re.compile(r"-----BEGIN (?:RSA |EC |OPENSSH |PGP )?PRIVATE KEY-----")),
    ("jwt", re.compile(r"\beyJ[A-Za-z0-9_\-]{10,}\.eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}")),
    ("url-credentials", re.compile(
        r"\b[a-z][a-z0-9+.\-]*://[^\s/:@$]+:(?![^\s/@]*[$])[^\s/@]{3,}@")),
    # Assignment form: the catch-all for env dumps and config echoes. The value class is
    # any run of non-space, non-quote characters, because real passwords contain
    # punctuation — an earlier, narrower class missed `Tr0ub4dor&3…` by stopping at the
    # ampersand. Prose is excluded by requiring a `:` or `=` immediately after the
    # keyword and a value of at least 12 characters; placeholders are excluded below.
    # The leading `[A-Za-z0-9_]*` matters: env vars bury the keyword inside a longer
    # identifier (`AWS_SECRET_ACCESS_KEY`), where a plain `\b` before the keyword never
    # matches because `_` is a word character. Only the prefix is permissive — the
    # trailing `\b` is kept, so `TOKENIZER=` and `MY_PASSWORD_HASH=` do not match.
    ("assigned-secret", re.compile(
        r"(?i)\b[A-Za-z0-9_]*(?:api[_\-]?key|secret[_\-]?key|secret|password|passwd|"
        r"token|access[_\-]?key|private[_\-]?key|auth[_\-]?token)\b[ \t]*[:=][ \t]*"
        r"[\"']?([^\s\"'=][^\s\"']{11,})[\"']?")),
]

# A secret can arrive base64-wrapped — a config blob, an encoded env file. Detecting it
# means decoding first. Bounded to blobs long enough to carry one and short enough not
# to spend the scan decoding every image in the corpus.
# No trailing \b: `=` is a non-word character, so anchoring after the padding excluded it
# from the match and left a blob whose length was not a multiple of four to decode.
B64 = re.compile(r"\b[A-Za-z0-9+/]{32,4096}={0,2}")

# Values that look like secrets but are conventions. Matching these would train a user to
# waive findings, which is worse than missing one.
PLACEHOLDERS = re.compile(
    r"(?i)^(?:x{3,}|\.{3,}|<[^>]+>|your[_\-]?\w+|example|changeme|"
    r"placeholder|redacted|dummy|test|none|null|true|false|\d+"
    # Shell parameter expansion in any form — `${VAR}`, `${VAR:?msg}`, `$VAR` — is a
    # reference to a secret, not one. The corpus is full of compose files and scripts
    # where treating these as findings would bury the real ones.
    r"|\$\{?[A-Za-z_][A-Za-z0-9_]*(?:[:?!#%\-+][^}]*)?\}?"
    # A path is not a credential, however secret-shaped its name — but the rule has to be
    # narrow. An earlier version matched any `word/word`, which swallowed
    # `wJalrXUtnFEMI/K7MDENG/…`: an AWS secret key contains slashes and looks exactly like
    # a relative path. So require a leading separator or a trailing file extension.
    r"|(?:[./~]|\.\.)/[\w./\-]*"
    r"|[\w.\-]+(?:/[\w.\-]+)+\.[A-Za-z]{1,6}"
    r")$")


def _direct_findings(text: str) -> list[tuple[str, int]]:
    hits = []
    for name, pattern in PATTERNS:
        for m in pattern.finditer(text):
            captured = m.group(1) if m.groups() else m.group(0)
            if PLACEHOLDERS.match(captured):
                continue
            hits.append((name, m.start()))
    return hits


def _b64_findings(text: str) -> list[tuple[str, int, str]]:
    """(pattern name, offset of the blob, the blob) for secrets hidden inside base64."""
    hits = []
    for m in B64.finditer(text):
        blob = m.group(0)
        try:
            # Pad rather than reject: blobs are often stored unpadded.
            padded = blob + "=" * (-len(blob) % 4)
            decoded = base64.b64decode(padded, validate=True).decode("utf-8", "strict")
        except (ValueError, UnicodeDecodeError):
            continue
        for name, _ in _direct_findings(decoded):
            hits.append((f"base64:{name}", m.start(), blob))
    return hits


def findings(text: str) -> list[tuple[str, int]]:
    """(pattern name, offset) for each hit. Values are never returned or logged."""
    return _direct_findings(text) + [(n, o) for n, o, _ in _b64_findings(text)]


def redact(text: str) -> tuple[str, int]:
    """Replace every finding with a typed marker. Returns (text, count)."""
    count = 0
    # A base64 blob concealing a secret is replaced whole: rewriting inside the encoding
    # would leave a blob that no longer decodes, which is worse than removing it.
    for name, _, blob in _b64_findings(text):
        if blob in text:
            text = text.replace(blob, f"<redacted:{name}>")
            count += 1
    for name, pattern in PATTERNS:
        def sub(m: re.Match) -> str:
            nonlocal count
            captured = m.group(1) if m.groups() else m.group(0)
            if PLACEHOLDERS.match(captured):
                return m.group(0)
            count += 1
            if m.groups():
                return m.group(0).replace(captured, f"<redacted:{name}>")
            return f"<redacted:{name}>"
        text = pattern.sub(sub, text)
    return text, count


def planted_corpus() -> list[tuple[str, str]]:
    """(label, text) pairs, each containing exactly one secret that must be caught.

    Shapes are chosen to be awkward on purpose: escaped inside JSON, buried in an env
    dump, wrapped in base64, and adjacent to prose.
    """
    fake_jwt = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dBjftJeZ4CVPmB92K27uhbUJU1p1r_wW1gFWFOEjXk"
    plants = [
        ("anthropic in prose", "the key is sk-ant-api03-" + "A1b2C3d4" * 6 + " keep it safe"),
        ("anthropic oauth", "export ANTHROPIC_AUTH_TOKEN=sk-ant-oat01-" + "Z9y8X7w6" * 6),
        ("openai", "OPENAI_API_KEY=sk-proj-" + "k" * 48),
        ("github classic", "remote add origin https://ghp_" + "b" * 36 + "@github.com/x/y"),
        ("github fine-grained", "token: github_pat_" + "c" * 60),
        ("aws pair", "AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE"),
        ("google", "AIza" + "d" * 35),
        ("slack", "SLACK_BOT_TOKEN=" + "xox" + "b-1234567890-abcdefghijklmno"),
        ("stripe", "sk_live_" + "e" * 24),
        ("private key", "-----BEGIN RSA PRIVATE KEY-----\nMIIEow...\n"),
        ("jwt", f"Authorization: Bearer {fake_jwt}"),
        ("db uri", "DATABASE_URL=postgres://admin:h0rs3batterY@db.internal:5432/app"),
        ("json-escaped", json.dumps({"env": {"API_KEY": "sk-ant-api03-" + "F5g6H7j8" * 6}})),
        ("env dump", "PATH=/usr/bin\nHOME=/root\nSECRET_KEY=s3cr3tV4lu3Longer\nSHELL=/bin/sh"),
        ("assignment with quotes", 'password: "Tr0ub4dor&3xxxxxxxx"'),
        ("base64-wrapped", "cfg=" + base64.b64encode(
            b"AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY").decode()),
        ("split across lines", "token=\nghp_" + "f" * 36),
    ]
    return plants


def benign_corpus() -> list[tuple[str, str]]:
    """Text that must NOT trip the detector. False positives train users to waive."""
    return [
        ("placeholder", "ANTHROPIC_API_KEY=<your-key-here>"),
        ("env var ref", "export TOKEN=$GITHUB_TOKEN"),
        ("xxx redaction", "password: xxxxxxxxxxxx"),
        ("git sha", "commit d84d3ad180b4613fdb24bd6dc7fdbeb01439778e"),
        ("prose", "The password must be rotated every ninety days."),
        ("docs", "Set api_key to the value from the console."),
        ("code", "if token == None: raise ValueError('token required')"),
        ("hash", "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"),
        ("uuid", "sessionId 5d05e0b9-348b-466b-940c-d0b4c56c2fc2"),
        ("base64 blob", "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8"),
        # Everything below was a real false positive found by scanning the local store.
        # They are kept because a benign corpus the author invented alone did not contain
        # a single one of them, and a detector is only as honest as its hard cases.
        ("shell default", "DATA_SOURCE_NAME: postgresql://svc:${POSTGRES_PASSWORD:?must be set}@db:5432/app"),
        ("templated url", "POSTGRES_READ_URL: postgres://svc:${POSTGRES_PASSWORD}@postgres:5432/app"),
        ("risk in a filename", "docs/superpowers/specs/2026-03-27-risk-disclosure-monitoring.md"),
        ("task in a tag", "<observation>\n  <type>task-completion-summary</type>\n"),
        ("htpasswd path", "mount ./broker/nginx/metrics.htpasswd into the container"),
        ("env var reference", "POSTGRES_PASSWORD=${POSTGRES_PASSWORD}"),
    ]


def measure() -> int:
    plants = planted_corpus()
    caught = [(label, bool(findings(text))) for label, text in plants]
    n_caught = sum(1 for _, hit in caught if hit)
    print(f"planted secrets: {n_caught}/{len(plants)} caught")
    for label, hit in caught:
        if not hit:
            print(f"    MISSED: {label}")

    benign = benign_corpus()
    fps = [(label, findings(text)) for label, text in benign]
    n_fp = sum(1 for _, hits in fps if hits)
    print(f"benign samples: {n_fp}/{len(benign)} false positives")
    for label, hits in fps:
        if hits:
            print(f"    FALSE POSITIVE: {label} -> {[h[0] for h in hits]}")

    # Redaction must actually remove the value, not merely flag it.
    leaked = []
    for label, text in plants:
        cleaned, _ = redact(text)
        for _, pattern in PATTERNS:
            for m in pattern.finditer(text):
                captured = m.group(1) if m.groups() else m.group(0)
                if PLACEHOLDERS.match(captured):
                    continue
                if captured in cleaned:
                    leaked.append(label)
        # A base64-concealed secret must be gone from the output too, blob and all.
        for _, _, blob in _b64_findings(text):
            if blob in cleaned:
                leaked.append(label)
    if leaked:
        print(f"    LEAK AFTER REDACTION: {sorted(set(leaked))}")
    else:
        print("redaction removed every caught value from the text")

    ok = n_caught == len(plants) and n_fp == 0 and not leaked
    print(f"\nclause 4 — {'MET' if ok else 'NOT MET'}")
    return 0 if ok else 1


def scan() -> int:
    """Counts of findings in the real local store. No values are printed or stored."""
    counts: collections.Counter = collections.Counter()
    files_with = 0
    scanned = 0
    for path in sorted(ROOT.glob("*/*.jsonl")) + sorted(ROOT.glob("*/*/*/*.jsonl")):
        try:
            text = path.read_text(errors="replace")
        except OSError:
            continue
        scanned += 1
        hits = findings(text)
        if hits:
            files_with += 1
            for name, _ in hits:
                counts[name] += 1
    print(f"scanned {scanned} transcript files")
    print(f"files with at least one finding: {files_with} "
          f"({100 * files_with / scanned:.1f}%)")
    for name, n in counts.most_common():
        print(f"    {name:20} {n}")
    print("\n(counts only — no matched value is printed, logged or committed)")
    return 0


if __name__ == "__main__":
    if "--scan" in sys.argv:
        raise SystemExit(scan())
    raise SystemExit(measure())
