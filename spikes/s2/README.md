# S2 — parser conformance: protocol and pass bar

**Pre-registered.** This file was written and committed before the parser was run
against the corpus. The results land in `results/summary.md` and are read against
the bar below, whichever way they come out. Moving the bar afterwards requires
writing down why, first, in the same file.

## The question

The design's capture adapter parses a harness's own transcripts, with "parsers
versioned per harness release and selected by the `version` field each record
carries", and degrades to a named `UNSUPPORTED-VERSION` rather than producing a
wrong exam. That is a bet on two things: that the format is stable enough for one
adapter to span releases, and that the version key is actually there to key on.

S2 asks: **does transcript schema churn across harness releases break a single
parser, and can it be made to fail closed rather than silently misparse?**

S1 established that assertions reproduce. S3 established that a real session can
be distilled — and that the Distiller needs tool inputs and tool *results*, not a
flattened chat. Both of those run through the capture adapter, so the adapter's
ability to read every version is upstream of both.

## What the survey already established

Written down so the pre-registration is not mistaken for more than it is. The
schema census (`schema-census.py`) was run before this bar and its findings are
inputs to it, not results of it:

- 198 project directories, 719 project-root transcripts, 962 subagent sidecar
  transcripts, 323,430 records, **18 harness versions** (`2.1.142` … `2.1.220`).
- Four content-bearing record types — `assistant`, `user`, `attachment`,
  `system` — appear in **18 of 18** versions, each with a stable core key set
  present in every version.
- Five content block types (`text`, `tool_use`, `tool_result`, `thinking`, and a
  bare string form) appear in 18 of 18 versions. `image` appears in 6 and
  `fallback` in 3.
- Churn is additive and concentrated in the periphery: `system` carries 33
  version-dependent keys, `user` 13, `assistant` 13. None of the version-dependent
  keys are in the core sets.
- **35% of records (113,791) carry no `version` field at all.** They are twelve
  bookkeeping types — `queue-operation`, `last-prompt`, `ai-title`, `mode`,
  `pr-link`, and others. The split is exact: the four content types *always*
  carry a version, the twelve bookkeeping types *never* do, and **no type is
  inconsistent**.

That last point is the one that shapes the bar. Per-record version selection
works for the records that matter, but a fail-closed rule that rejects any record
without a recognised version would reject every real transcript in the corpus.
Whether the parser gets that right is not yet measured.

## What is being built

A parser, not a product. It extracts exactly what the Distiller was shown to need
in S3 — human turns, tool calls with their inputs, tool results, model identity,
working directory, and the link from a subagent transcript back to the tool call
that spawned it — and nothing else. It is a measuring instrument in the same
sense as S1's sitting runner, and it is explicitly outside the production bar
that governs `internal/` and `cmd/`.

Phase 1 owns the real capture adapter and the committed conformance corpus. S2
owns the question of whether that is a sound thing to build.

## Protocol

The parser is keyed on the `version` field of content records and run over every
transcript in the local store: 719 project-root files and 962 subagent sidecars,
across all 18 versions.

Structural invariants are checked on every parsed session rather than assumed:

- every `tool_result` block resolves to a `tool_use` block that precedes it;
- every record's `parentUuid` resolves within the file, or the record is a root;
- every subagent sidecar resolves to the `tool_use` that spawned it, via the
  `toolUseId` in its `meta.json`;
- `sessionId` and the separately-present `session_id` are reported when they
  disagree, rather than one being silently preferred.

Nothing is written outside the spike. No transcript content is committed: the
corpus stays local, and only counts, shapes and the writeup ship.

## The pass bar

S2 passes only if all four clauses hold.

1. **Coverage.** The parser reads at least 99% of transcript files across all 18
   versions. Any file it cannot read is reported as a named error identifying the
   file and version — never a partial or a silently-truncated session.

2. **No silent misparse.** On every parsed session the structural invariants
   above either hold, or are reported with file and version. A violation is a
   finding to publish, not a record to drop. The failure this clause exists to
   catch is a parser that returns a plausible, wrong session.

3. **Fail-closed on the unknown, open on the versionless.** A synthetic content
   record bearing an unseen future version is refused with a named
   `UNSUPPORTED-VERSION` rather than parsed as current. A versionless bookkeeping
   record does **not** trigger that refusal. Both directions are required: a
   parser that fails closed on everything is as useless as one that never does.

4. **Redaction before fixtures.** Fixtures cannot ship unredacted, and the design
   claims a planted-secret corpus is caught at 100%. A planted corpus is built
   and the catch rate measured. Below 100% and no fixture is written, whatever
   the other clauses say.

## If it fails

- **Clause 1 fails** — one adapter cannot span the observed releases, and the
  design's per-release parser versioning is load-bearing sooner than Phase 1
  assumes.
- **Clause 2 fails** — the invariants the Session IR wants to rest on are not
  properties of real transcripts, and the IR has to represent the messiness
  rather than assert it away.
- **Clause 3 fails** — `UNSUPPORTED-VERSION` is not implementable as specified,
  and the fail-closed promise needs restating in terms of what the format
  actually guarantees.
- **Clause 4 fails** — no conformance corpus can be committed at all until
  redaction is stronger, which pushes §13.1 out of Phase 1.
