# 0007 — The exit-code contract and the discipline for machine-readable output

- Status: Accepted
- Date: 2026-07-26

## Context

Assayer is meant to be driven by other programs. A continuous-integration job
decides whether to block a merge; a wrapper decides whether to retry; a person
writes twenty lines of shell around it. Those callers need surfaces that are
promises rather than output, and a promise made carelessly is worse than none,
because it is discovered by the people who relied on it.

The design names three such surfaces: verdict exit codes, a streamed event
format, and machine-readable verdict objects. Only the exit codes have anything
producing them today. Designing the other two now would mean inventing shapes to
fill a schema rather than deriving them from something that emits them — and a
published contract is far more expensive to retract than an internal interface.

There is also a smaller question that has to be settled before the first
document is written rather than after: how a consumer knows what it is holding,
and how that answer survives the shape changing.

## Decision

### Exit codes

| Code | Meaning |
|---|---|
| 0 | Everything held, or a command outside the verdict path did what was asked |
| 1 | A failure that is not a verdict: unreadable configuration, a missing exam, a corrupt ledger |
| 2 | A malformed command line |
| 80 | A regression verdict at any scope |
| 81 | Drift suspected, including an escalation stopped by a budget |
| 82 | No verdict could be reached |
| 83 | A pin that no longer applies, under `--strict-stale` only |

Three rules go with the table.

**Only the commands that administer examinations may return 80 to 83.**
Everything else returns 0, 1, or 2, so a caller can tell which invocations are
entitled to say anything about the thing being measured. A tool that cannot
distinguish a typo from a regression is not an instrument.

**When several conditions hold at once, the most severe wins: 82, then 80, then
81, then 83, then 0.** Simultaneity is routine rather than exotic — a suite can
carry a failing exam, a rotted pin and an errored one together — and without a
stated order two correct implementations of this contract disagree. A caller's
branch would then get a different answer depending on which condition the
implementation happened to test first, and the worst case is a real regression
resolving to the code for a stale pin and being waved through. Code 82 outranks
a regression because it reports that the other answers could not be trusted, and
a regression computed from a run that half failed asserts more than was
measured.

**A verdict code will not be returned until a machine-readable verdict object
ships in the same release.** An exit code says something regressed; without a
document it gives no way to learn what. The only remaining option is to scrape
human output, and those scrapers become an unversioned contract that breaks the
first time a table gains a column. The alarm and the readout ship together.

**The range is 80 to 83, not 70 to 73.** Earlier drafts specified 70 to 73 on
the stated grounds that the range was clear of `sysexits.h`. It is not:
`EX_SOFTWARE` is 70, `EX_OSERR` 71, `EX_OSFILE` 72 and `EX_CANTCREAT` 73, four
consecutive values inside a block running from 64 to 78. The collision at 70 is
the damaging one, because the convention reserves it for an internal error in
the program itself — which is what this program means by 82, not by 80. A
wrapper that speaks that convention and dies of its own fault would have exited
70 and been reported as a regression that never happened: the manufactured
verdict this design forbids everywhere else, arriving through the exit code
rather than through any verdict logic. The correction was free because nothing
had been published; it would not have been later.

### Machine-readable documents

Every document carries the identifier of its kind and major version, a revision
number, its stability tier, and the list of schemas the running build can
produce.

The revision exists because the identifier cannot do the job alone. While the
contract is experimental its shape may change without the major version moving,
so a consumer matching only the identifier cannot tell one revision from another
and receives a parsing error where it wanted a clean rejection. A monotonic
number beside the identifier lets it ask whether the field it needs is present.

The list of schemas is what lets a caller discover what a binary supports
without running a command that costs money and time.

Documents are UTF-8, compact, one object per line, newline-terminated, and do
not escape HTML characters — Go escapes `<`, `>` and `&` by default, which is
harmless in a version document and corrupts anything carrying a shell command or
a diff. Fixing that once, before any such document exists, is cheaper than
finding it in the first one that does.

**Identifiers are rooted at this repository, not at a domain.** An earlier draft
used a project-named domain, on the stated assumption that nothing was served
there. That was not checked, and it is false: the domain is registered and
serving an unrelated product with the same name. Every document would have
pointed consumers at a third party's address, which the project cannot publish
to and cannot prevent from serving a different schema at the same path.

### Schemas are generated, not maintained alongside

The committed schemas are produced from the types that produce the documents,
and a test fails when the two disagree, printing what to write.

The alternative — a hand-written schema checked by an external validator —
leaves two descriptions of one thing that can be wrong together. A field
renamed in the type regenerates its sample output and still validates, because a
schema that does not forbid unknown properties accepts the new name and does not
notice the missing one. Generating the schema makes that impossible, needs no
dependency, and is the arrangement this repository already uses for its linter
configuration.

## Consequences

- The exit codes are documented publicly rather than only in source. A contract
  a caller is expected to write into their scripts, documented only inside a
  package they cannot import, is not a contract.
- Renumbering an exit code is now a breaking change to code this project cannot
  see. A test pins the values so it cannot happen inadvertently.
- The precedence rule constrains the implementation of every future command that
  reaches a verdict. That is the intent: it is a property of the contract, not
  of any one command.
- Tying the verdict codes to the existence of a verdict document delays part of
  the contract. That is deliberate — the alternative gives early users a reason
  to build on human output, which is the more expensive thing to break later.
- Generating schemas means every emitted field needs a type the generator
  understands. A field of an unmapped type fails the build rather than producing
  a schema that quietly describes nothing.
- **The event stream and verdict objects are still unspecified.** They are the
  larger part of what a driving program eventually needs, and they land before
  this phase closes, with the components that produce them. The record says so
  rather than leaving the omission to be read as completeness.
- **The exam format's schema is not decided here.** It arrives with the command
  that first writes one. It carries a question this record does not answer: the
  format is TOML, the standard library cannot parse TOML, and validating it
  therefore needs either this module's first third-party dependency or a
  different format. That decision belongs to the record that ships the writer.
- Committed schemas sit outside the package tree, so they are brought inside the
  architecture guard's scan explicitly. A schema enumerating adapter names would
  otherwise be a public artifact naming harnesses in the one place nothing was
  looking.
