# S2 — parser conformance: results

Measured 2026-07-26 against the local transcript store: 198 project directories,
1,681 transcript files, 323,801 records, 18 harness versions (`2.1.142` …
`2.1.220`). Every figure is produced by `schema-census.py`, `parse.py` and
`redact.py`, so this document is regenerated rather than transcribed.

The protocol and the four-clause bar were committed in `../README.md` before the
parser ran.

## Against the pre-registered bar

| clause | bar | result |
|---|---|---|
| 1 — coverage | ≥ 99% of files parsed, refusals named | **100.00%** (1681/1681), 0 refused — met |
| 2 — no silent misparse | invariants hold or are reported with version | 44 files reported, 0 silent — met |
| 3 — fail closed on the unknown, open on the versionless | both directions | both — met |
| 4 — redaction | planted corpus caught at 100% | 17/17, 0 false positives — met, **and the clause was too weak**; see below |

## Clause 1 — one parser spans every version

All 18 versions, every file, no refusals. The reason is visible in the census:
the four content-bearing record types (`assistant`, `user`, `attachment`,
`system`) appear in 18 of 18 versions, and **every key the parser reads is
present in every version for every record type**. Churn is real but additive and
peripheral — `system` carries 33 version-dependent keys, `user` 13,
`assistant` 13 — and none of it touches the core.

Extracted across the corpus: 209,890 content records, 35,498 tool calls, 35,510
tool results, 23,528 human turns, 63,277 sidechain records.

## Clause 2 — what the invariants actually hold

**Tool-call pairing is exact.** Zero `tool_result` blocks lacked a matching
`tool_use`, across 35,510 results. The Session IR can rest on that.

Three violation classes did appear, and the per-version split separates a
standing property of the format from release churn:

| violation | files | versions | reading |
|---|---|---|---|
| multiple `cwd`s in one transcript | 29 | 15 of 18 | standing property — sessions move between directories |
| `sessionId` / `session_id` disagree | 15 | 4 of 18 | release churn — the second field is newer |
| `parentUuid` unresolvable in-file | 2 | 2 of 18 | rare; resumed sessions reference a parent elsewhere |

**`session_id` is not a duplicate of `sessionId`.** In all 15 disagreeing files
the filename is among the ids *and* the other id names a sibling transcript in
the same project directory. It is a fork or resume pointer. An adapter that
treats the two as synonyms will merge or misattribute sessions — and S3's
subject session was one of these files, so this is not hypothetical.

**Subagent attribution is not total.** Of 932 subagent sidecars, 891 (95.6%)
resolve to a tool call in their parent transcript. The remaining 41 resolve to
no tool call in *any* transcript in their project. They span 5 versions and 2
projects, and compaction accounts for at most 19 of them, so there is no single
cause.

## Clause 3 — and the cost of taking it literally

Both directions hold: a content record bearing an unseen version is refused as
`UNSUPPORTED-VERSION`, a versionless bookkeeping record is admitted and skipped,
and an unknown record *type* is refused rather than skipped.

That last one matters. **35% of all records (113,911) carry no `version` field**
— twelve bookkeeping types that never carry one, against four content types that
always do, with no type inconsistent. The design selects parsers by "the
`version` field each record carries"; implemented literally, that rejects every
real transcript in the corpus. The parser therefore enumerates the versionless
types explicitly rather than inferring them from a missing field, so a genuinely
new versionless *content* type fails loudly instead of being skipped into a
session that looks complete and is not.

**The operational cost is the finding.** The 18 versions span 66 days — a new
harness version every **3.9 days** — and they overlap: `2.1.190` and `2.1.191`
both first appear on the same day, so even one user runs several concurrently.
Strict per-version allowlisting means capture breaks about twice a week until
someone adds a fixture.

## Clause 4 — met, and mis-specified

The planted corpus is caught 17 of 17, no benign sample trips the detector, and
every caught value is verifiably gone from the redacted text. That is the clause
as written, and it passes.

The clause is the wrong gate. Recall against secrets the author planted is easy;
the detector reached 17/17 only after three corrections, each found by the
plants being deliberately awkward — a password containing `&`,
`AWS_SECRET_ACCESS_KEY=` where `\bsecret\b` cannot match a keyword buried in an
env-var name, and a base64 blob whose padding was excluded from the match.

What the clause never measured is precision, and precision is where it fails.
Scanning the real store fires on **846 of 1,681 files (50.3%)**. A masked sample
showed the largest contributors are code, not credentials: in a 200-file sample
the three most common matched keys were `input_micro_per_token=`,
`output_micro_per_token=` and `cached_input_micro_per_token=` — a pricing
module's field names, matched because they end in `token`. Alongside them sat
genuine literal secrets in compose files. Both are present; the ratio is not
quantified here, because labelling at scale means reading values, which this
spike will not do.

Three false-positive classes were found and fixed against the real corpus — the
word "ri**sk-**disclosure" satisfying an OpenAI key pattern, `${VAR:?must be
set}` shell defaults, and templated `${VAR}@host` connection strings — and each
is now a permanent case in the benign corpus, because the invented benign set
contained not one of them.

**A redactor with this precision is unusable as specified.** The design blocks
distillation on findings; at one finding per two files, a user either cannot
distil or learns to waive findings wholesale, which removes the control
entirely.

**One further finding, from an error made while measuring.** The sampling tool
masked the matched value but not its surrounding context, and a *different*
credential adjacent to a match was printed in the clear. Redaction has to be a
whole-document operation. A per-finding redactor that rewrites matches in place
will export a fixture with the secret next to the one it removed.

**And one from trying to commit the corpus.** The planted secrets were first
written as literals, and GitHub's push protection refused the push — correctly,
on the Slack plant. A committed planted-secret corpus fights every scanner it
meets: the host's push protection, the repository's own secret scanning, and
whatever the developer runs locally. The only ways through are to allowlist real
detections or to disable scanning, and both are worse than the corpus. The
plants are now assembled from fragments at run time, so no contiguous string in
the file matches a scanner, and the detector finds nothing in its own source.
The design proposes exactly this corpus running in CI; it has to be synthesised
at test time and never committed as literals.

## Consequences for the design

- **Fail closed on shape and record type, not on the version string.** Every key
  the parser needs is present in all 18 versions, while the version changes every
  3.9 days. Refusing unknown versions buys little and costs capture twice a week.
  The safer and more operable rule: parse an unknown version, validate that the
  records carry the keys the IR needs, refuse on an unknown record type or a
  missing core key — and mark the session's capture fidelity as unverified, which
  the design already has vocabulary for. That admits a class of silent semantic
  change that version-pinning would catch; the trade should be made deliberately
  rather than by default.

- **The IR must carry session lineage, not a session id.** `session_id` names the
  session a transcript was resumed from. Recording one id, or preferring the
  wrong field, misattributes 15 of 1,681 files in this corpus.

- **Workspace pinning cannot assume one working directory per session.** 29 files
  across 15 of 18 versions move between directories mid-session.

- **Unattributed subagent work must be reported, not dropped.** 4.4% of sidecars
  resolve to no tool call. Silently dropping them understates what the golden
  session did, and the Distiller derives the tool allowlist and footprint from
  exactly that.

- **Gate redaction on precision, not on planted recall.** Replace the "100% on a
  planted corpus" criterion with a two-sided one: 100% recall on plants *and* a
  false-positive rate low enough that a finding means something on a real
  transcript. The plants should be contributed adversarially rather than by the
  author of the patterns.

- **Redact whole documents.** Per-match rewriting leaves adjacent secrets intact.

- **Synthesise the planted corpus; never commit it.** Literal plants are refused
  by push protection and flagged by repository scanning, and the escape hatches
  are worse than the corpus.

## Disclosures

- **One machine, one user.** 18 versions over 66 days, all `2.1.x`. The design
  mentions a "2.0 inline-sidechain era"; no 2.0 transcripts exist in this store,
  so the oldest era the adapter claims to span is entirely unmeasured here.
  Community corpora were not used.

- **The parser is not the capture adapter.** It extracts what the Distiller was
  shown to need and nothing else, and produces no Session IR. Clause 1 says one
  parser can read the fields; it does not say the full IR survives every version.

- **Precision is characterised, not quantified.** The false-positive discussion
  rests on a masked sample of a 200-file subset, read by hand. No labelled
  precision figure is claimed.

- **No fixture corpus was produced.** Clause 4 gates it, and while clause 4 is
  met as written, its precision problem means the redactor is not yet trustworthy
  enough to export from. The committed conformance corpus of §13.1 stays Phase 1
  work.

- **The subagent sidecar count disagrees with this spike's own pre-registration,
  and neither figure can now be recovered.** The README states 962 sidecars
  against 719 project-root transcripts, which sums to the 1,681 files used
  everywhere else; this summary states 932 against the same 1,681 total, which
  does not. A census on 2026-07-26 found 2,485 files (725 project-root, 1,760
  sidecars), so the store has grown too much for a re-run to settle which
  snapshot was right. The 95.6% attribution rate is reported as measured, but
  its denominator is uncertain by about 3%, and no conclusion here should be
  read as resting on the exact count. Recorded rather than corrected: choosing
  one of two irreproducible numbers would be worse than saying they disagree.

- **The store is live.** It grows and is pruned while being measured; record
  counts differ by a few hundred between the census and the conformance run for
  that reason. The figures are a snapshot on the measurement date and the scripts
  are committed so the method outlives the corpus.

- **No transcript content is committed.** Outputs are counts, versions, record
  types and key names. Raw transcripts never leave the machine.
