# 0006 — The neutral session representation and the capture port

- Status: Accepted
- Date: 2026-07-26

## Context

Everything Assayer does downstream of reading a session operates on one
representation. Distilling an exam, replaying it, judging the result and
reporting a verdict all read the same types, and none of them may know which
coding agent produced the session. That representation is the contract, so
getting its vocabulary wrong couples the whole program to one vendor no matter
how carefully the package boundaries are drawn.

ADR-0005 put mechanical limits on where a harness may be named and what may
import what. Those limits cannot see the problem this record addresses. A field
named for one agent's internal concept crosses no import boundary and contains
no vendor's name; it compiles, it passes every check, and it makes the program
work for exactly one tool.

The difficulty is that only one harness has been measured. A representation
derived from that corpus alone would be shaped by it, with neutral-sounding
names over single-vendor concepts. Two rules therefore govern what follows: a
field exists because a named consumer downstream needs it rather than because a
transcript happened to contain it, and the representation is exercised by a
second implementation shaped deliberately unlike the first.

Some of the first draft of this decision was written from a survey that had
skipped the record types most relevant to it. Re-running the measurement over
the local corpus changed three decisions, and those changes are noted below
where they apply. The lesson is recorded rather than tidied away: the survey had
already collected the evidence that a session's model is not singular, and had
not looked at it.

## Decision

### What the representation holds

A session is an ordered sequence of turns, each with the text produced, the
model that produced it, and the tool calls made. A tool call carries the tool's
name, a neutral classification of what kind of thing it did, the command as
executed, what was put into it, the files it changed with their content before
and after, and its result. A session also carries its identity and its parent,
the working directories and version-control state it ran against, what it
consumed, the work it delegated, and a statement of how it was captured.

Several of those are less obvious than they look, and each was chosen against a
measured alternative:

**The model belongs to a turn, not to a session.** A census of local transcripts
found 54 of 2,459 sessions using more than one model and three using three. An
earlier draft recorded one model per session, which would have compared a
recorded model identity against a session that had used two.

**Identity is two fields.** A session that resumes or forks another has both its
own identity and a pointer to its parent, and the reference harness stores them
separately. Recording one, or preferring the wrong one, attributes a session's
work to its predecessor.

**Working directories are a list.** Sessions move between directories partway
through — 29 transcripts across 15 of 18 measured releases did.

**File changes carry content.** Comparing a replay's changes against the golden
session's by similarity is arithmetically impossible from paths alone.

**Tool inputs are kept.** Distilling one real session found the decisive content
in a question the agent asked and in the body of a file it wrote, not in
anything the human typed. A representation holding only the conversation
reproduces that failure.

**A tool result carries how the call ended, not whether it "failed".** Three
different facts would otherwise land on one boolean: a command that ran and
exited non-zero, a call the harness refused or that hit a cap, and a call that
never returned. Only the first may carry an assertion to a failure verdict; the
others are Assayer's own plumbing or the environment, and a regression must
never be manufactured from either. The exit code is kept for the same reason —
test runners use different non-zero codes for "the tests failed" and "what you
named does not exist", and the second is a pin that has rotted.

**Delegated work is flat.** Nesting sub-agent work under the call that spawned
it, with a side bucket for work belonging to no call, encodes one harness's
storage arrangement. Agents that record delegated work as siblings with a parent
pointer cannot produce an orphan at all, and several record no delegation.

Every enumeration reserves its zero value for "unknown", and only the names are
ever written down. A meaningful variant at zero means an unfilled struct makes a
claim invisibly — an earlier draft's default fidelity was the most trusted one —
and integer values on disk would silently renumber if a variant were ever
inserted, reinterpreting artifacts with nothing to detect the change.

### Observability is declared, and the declaration is binding

Every adapter states what it can observe, and every session states what was
observed for it and what was observed only partly. Nothing else in the
representation distinguishes "this did not happen" from "this adapter cannot
see it", and conflating those turns Assayer's own blind spot into the model's
regression — the one verdict this tool must never invent.

Partial observation is ordinary rather than exceptional: sessions get compacted,
large results are shortened, and a recorder watching API traffic sees the edits
an agent requested but not the files a shell command rewrote underneath it. A
capability declared whole when it is partial fails an assertion on evidence that
was never complete, so partial is its own state.

The declaration is enforced rather than documented. A capability governs every
field, a test fails if any field is governed by none, and an adapter that
reports something it did not declare fails the shared kit. That check is
default-deny: a field added later is uncovered until somebody decides what
observing it means.

Money is separated from tokens deliberately. Of the agents this project intends
to support, one reports a price; most report token counts and no cost at all,
and one reports a cost field that is always zero. Declaring an observed price
that was really derived from a table turns a cost comparison into a comparison
against a number nobody measured.

### The capture port

A capture adapter names itself, states its guarantee and capabilities,
discovers sessions under a query, and loads one into the representation.
Discovery takes a query because it is not free — one machine held 2,485 session
records after a few months — and a signature that could only mean "everything"
would have to be broken later to add a filter.

Loading distinguishes three failures: a reference naming nothing, because stores
are live and a session can be pruned between discovery and loading; a source
recognised but unreadable; and a source that is damaged. They reach different
verdicts, and one error would force every caller to guess.

Sentinel errors are constants of a defined string type rather than package-level
variables. ADR-0005 forbids package state in these packages, and that rule is
load-bearing: it prevents a registry of harnesses from being assembled at
startup through entirely legal imports. An earlier draft proposed relaxing it,
which was unnecessary — constants keep error matching working, because a defined
string type is comparable — and improper, since this project changes accepted
records by superseding them rather than by amending them in passing.

### A second implementation, shaped differently

The shared conformance kit ships with a reference capture adapter for a harness
that does not exist. It is not a simplified version of the measured one. It has
no turn structure, so its tool calls arrive in a turn that never existed; it
emits consecutive agent turns with no human turn between; it changes model
partway through; it stores delegated work as a sibling that no call references;
it reports tokens and no money; its commands arrive already split into
arguments; and it has no version control.

Every one of those is a real property of an agent this project intends to
support. An implementation that was merely poorer would have exercised nothing:
leaving fields empty satisfies any shape, and shape is where a single-vendor
assumption hides.

## Consequences

- An adapter that cannot observe something says so, and a caller can ask before
  spending anything on a replay whether a question is answerable at all.
- The capability list is finer-grained than the concepts it describes, because
  it has to match fields. An adapter watching API traffic sees token counts but
  no price, and result bodies but no exit status; a coarser capability would
  force it either to claim something it cannot see or to discard something it
  can.
- Adding a field to the representation now requires deciding what observing it
  means, or the shared kit fails. This is friction, and it is the point.
- **The turn is a reconstruction, and this is the weakest assumption here.**
  Neither harness examined stores sessions as turns; both are flat streams of
  records. Every adapter therefore decides where turn boundaries fall, and that
  decision silently determines any metric that counts turns. The alternative — a
  flat list of events — moves the same reconstruction into every consumer rather
  than doing it once at the boundary, which is why it was not taken.
- **The workspace type assumes a local filesystem and a version control system
  with content-addressed revisions.** That coupling is stated rather than hidden
  behind neutral names. Sessions that ran in a container, on a remote host, or
  against a server-held workspace are not capturable this way in this phase, and
  their adapters will say so rather than filling the fields with something local.
- **Cost is close to a single-vendor feature.** Absolute spending caps apply
  where an agent reports a price; comparisons against a previous run are
  denominated in tokens, which every candidate agent reports.
- **Neutrality is not demonstrated by this record.** It rests on one measured
  harness, a reading of a second's format, and a reference implementation
  written by the same hand as the representation it tests. That is a check, not
  a proof. The proof is an adapter for a format nobody here chose, and until one
  exists the claim should be read as an intention with evidence behind it rather
  than as a result.
- The shared kit checks the contract, not correctness. Whether an adapter reads
  its harness faithfully needs a fixture corpus per adapter, which arrives with
  each adapter and depends on a redaction gate that is not yet trustworthy. An
  adapter that passes the kit is well behaved and has not thereby been shown to
  parse anything correctly.
- Because the redaction gate lands after the first commands that write exam
  artifacts, those commands write only to paths excluded from version control
  until it arrives, and the in-memory session type is never what gets persisted.
