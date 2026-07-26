# Assayer

[![ci](https://github.com/JumpMasters/assayer/actions/workflows/ci.yml/badge.svg)](https://github.com/JumpMasters/assayer/actions/workflows/ci.yml)
[![codeql](https://github.com/JumpMasters/assayer/actions/workflows/codeql.yml/badge.svg)](https://github.com/JumpMasters/assayer/actions/workflows/codeql.yml)
[![codecov](https://codecov.io/gh/JumpMasters/assayer/branch/main/graph/badge.svg)](https://codecov.io/gh/JumpMasters/assayer)
[![Go](https://img.shields.io/github/go-mod/go-version/JumpMasters/assayer)](go.mod)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)

Assayer is a command-line tool for finding out whether a coding agent's
behaviour on *your* work has changed.

You bless a real session — one that went the way you wanted — as the standard.
Assayer distils it into an exam you commit alongside your code, then
re-administers that exam whenever something underneath moves: a new model
version, a harness update, a different backend, an edit to your own
configuration. It reports what happened, as counts under named conditions.

## Status

**There is no examining software here yet.** This repository contains the build,
test, and security gates, six architecture decision records, the enforcement of
the package layout described in ADR-0005, and a binary that can print its own
version. Everything below written in the future tense is unbuilt.

The design is settled and written down. What is *not* settled is whether the
design's central assumption survives contact with measurement, and that is the
current work.

Agents are stochastic. The same task, run twice under identical conditions, does
not produce the same session. So the method only works if run-to-run variance —
measured at the level the assertions actually check, not at the level of the
text — is small enough that a real change stands out from noise within a number
of replays someone would be willing to pay for. That is an empirical question
with a cheap experiment attached, and it is being answered before anything is
built on top of it.

The bar was fixed in advance, before any data was collected:

> Across three calibration exams replayed ten times each under fixed conditions,
> each exam must reproduce its deterministic assertions at least 90% of the time,
> with no exam below 70%, and a median replay must cost no more than twice the
> session it was distilled from.

If the numbers come in under that bar, the design changes — coarser assertions,
more replays by default, a different balance of evidence — before any product
code is written. Moving the bar after seeing the data requires writing down why,
first. A test that cannot fail is not a test, and this project has no business
shipping an instrument if it will not hold its own first measurement to the
standard it asks of everyone else's.

That measurement has now been taken, and the numbers are in [Calibration
numbers](#calibration-numbers) below. Across four exams replayed thirty-five
times each, deterministic assertions reproduced in 131 of 132 scored replays, at
a median cost of $0.15 to $0.57 per replay depending on the task. The bar is met
on every clause as a point estimate. Stated there rather than omitted: only one
of the four exams has a 95% lower bound at or above 90%, so this establishes a
high reproduction rate and not a 90% floor for every exam.

The scaffolding that produced them is committed under `spikes/`. It is not
product code and is held to a lower standard than the rest of this repository,
which its own README says plainly. It is committed anyway, because a published
measurement whose method cannot be inspected is an assertion rather than
evidence.

## The problem

When an agent setup gets worse, there is currently no instrument.

The public record is consistent about this. In September 2025 Anthropic
published a postmortem confirming that overlapping infrastructure bugs had
degraded a share of requests for weeks. In April 2026 an issue on the
`claude-code` tracker, reporting that the tool had become unusable for complex
engineering work, gathered thousands of reactions and hundreds of comments
before a second postmortem confirmed further regressions. In both cases the
users were right. In both cases they had no way to demonstrate it, and the
confirmation arrived from the vendor, afterwards.

The difficulty is that three things can change underneath a setup that used to
work, and they are easy to confuse:

- the **model** — a new version, a quantisation, a routing change;
- the **harness** — the agent CLI, its prompts, its tool definitions;
- **your own configuration** — an edit to `CLAUDE.md`, a new skill, an MCP
  server someone added last week.

A tool that watches your logs cannot separate these, because your work changed
too. A quiet week and a hard week look different for reasons that have nothing
to do with any of the three.

The way out is the ordinary one from any other field that measures things: hold
the task fixed and vary one thing at a time. Replay identical work under the
changed condition and compare outcomes. That requires having the work recorded
in a form that can actually be replayed — which is what most of this design is
about, and where the difficulty genuinely lies.

## What Assayer reports, and what it does not

A tool that overstates a regression manufactures precisely the argument it
exists to end. The limits below are part of the design, not caveats bolted onto
it, and several of them make the output less impressive on purpose.

- **It reports counts under named conditions.** "Twelve of twelve replays passed
  under the previous condition; three of five failed under the new one." Not
  "the model got worse." The report states the conditions, the number of
  replays, the observed spread, and what the replays cost.

- **It does not tell you whose fault it is.** Model, harness, and configuration
  are separable only by varying them one at a time, which is what `assayer
  bisect` will do — deliberately, at a cost printed before it starts. Without
  that, a verdict says something changed and declines to name a culprit.

- **A verdict is a statistical claim, never a certainty.** The published
  calibration numbers are what make the claim readable, which is why they are a
  precondition for the first release rather than a later addition.

- **Assayer's own failures are never regressions.** A crashed harness, a denied
  network call, an exhausted budget, a workspace pin that no longer applies —
  each produces an error or a staleness result, carrying the underlying tool's
  own message. None of them can produce a failure verdict. An instrument that
  blames its own plumbing on the thing it is measuring is worse than no
  instrument.

- **The word "regression" is earned.** One failed replay is not a regression; it
  is a reason to run more replays. Only an escalated series can produce that
  word. If a budget stops the escalation, the result says so in those terms
  rather than resolving to something more confident or falling silent.

- **Nothing leaves your machine.** No server, no telemetry, no account. Exams
  are files in your repository, reviewed like any other code. The single
  exception is an explicit export command for sharing an exam, which re-runs
  redaction on the way out.

- **Every report names the adapters behind it.** How faithfully a session can be
  captured and replayed differs by harness and by method, sometimes
  substantially. Reports state which capture and run adapters produced a verdict
  and what each guarantees, because a measurement whose provenance is concealed
  is not evidence.

## How it will work

### The loop

```
bless → distil → review → run → report
```

**Bless** picks a real, finished session and marks it as the standard. **Distil**
turns it into an exam: the task instruction, a pin of the workspace it started
from, a fingerprint of the environment, a tool allowlist derived from the tools
the session actually used, and a set of *suggested* assertions drawn from
evidence in the session. **Review** is a human step and is not optional — exams
are code, and a suggestion inferred by a language model is a draft, not a fact.
**Run** administers the exam under a condition you name. **Report** states the
outcome with its conditions, counts, and cost.

### An exam

An exam is a directory you commit:

```
.assayer/
  exams/<slug>/
    exam.toml          runner and version at bless, model and backend, workspace
                       pin, environment fingerprint, tool allowlist, caps
    instruction.md     the distilled task — the replayable ask
    workspace.patch    uncommitted state at session start, if there was any
    assertions.toml    tiered assertions, each carrying the evidence it came from
    verify.sh          optional escape hatch with an exit-code contract
  baselines/<slug>/*.json
```

Assertions are evaluated cheapest-first, and a cheap failure short-circuits the
expensive tiers:

1. **Outcome**, deterministic. Tests the blessed session turned green must turn
   green again; a sampled set that was passing must still pass; files, commands,
   and patch similarity.
2. **Trajectory**, deterministic. Files touched stay within the allowed set;
   required tools ran and forbidden ones did not; cost, turns, and wall clock
   stay inside absolute caps and within a band relative to the baseline. The
   same work at twice the price is a regression, and it is checked by default.
3. **Behavioural profile**, diagnostic only. Read-to-edit ratio, turns per task,
   tokens per turn. These annotate a report to describe *how* behaviour moved.
   They never flip a verdict.
4. **Judged**, gated and cached. A rubric evaluated by a model from a *different*
   family than the one under examination, whose agreement rate against a labelled
   set is measured and published. It never carries a verdict alone.

### Verdicts

| Verdict | Meaning |
| --- | --- |
| `PASS` | Assertions held under the stated condition. |
| `FAIL` | An escalated series failed. The only verdict that means regression. |
| `FLAKY` | The exam cannot hold a stable pass rate; quarantined, reported, excluded from counts. |
| `STALE` | The workspace pin or toolchain fingerprint no longer applies. Not a failure. |
| `ERROR` | Assayer or the harness failed. No verdict; excluded from counts. |

Exclusions are always reported, never silently dropped.

## Design constraints

These are the properties the implementation is required to preserve. They are
recorded because each one is individually tempting to remove.

- **The core knows nothing about any specific agent.** Everything downstream of
  a capture adapter works on a neutral representation of a session. Harness
  knowledge lives in adapters and nowhere else, enforced mechanically once the
  package layout lands. Claude Code is the first adapter, not the substrate.
- **Judgment happens at authoring time; CI stays deterministic.** A model
  proposes assertions once, a human accepts or deletes them once, and from then
  on the checks that gate anything are deterministic code.
- **Committed exams are user data.** The exam format and the machine-readable
  output carry explicit versions, and no schema change may strand an exam
  someone already committed without a migration path.
- **The machine interface is the product too.** The event stream, the verdict
  objects, and the verdict exit codes are contracts other tools can build on;
  the human output and any CI integration are thin layers over them.

## Calibration numbers

Measured 2026-07-25. Four exams, thirty-five replays each, fixed conditions:
`claude-opus-4-8`, one git worktree per replay at a pinned commit, exam-supplied
fail-to-pass tests plus an 82-file no-regression suite. Method, raw rows, and
the full reading are under [`spikes/s1/`](spikes/s1/); the table regenerates
with `python3 spikes/s1/analyze.py`.

| Exam | Replays | Errors | Scored | Reproduced | Rate | 95% lower bound | Cost per replay |
| --- | --- | --- | --- | --- | --- | --- | --- |
| A — bounded fix, pointed | 35 | 0 | 35 | 35 | 100% | **90.11%** | $0.147 |
| B — feature slice, pointed | 35 | 4 | 31 | 30 | 96.8% | 83.81% | $0.243 |
| C — design latitude | 35 | 1 | 34 | 34 | 100% | 89.85% | $0.328 |
| D — discovery, unpointed | 35 | 3 | 32 | 32 | 100% | 89.28% | $0.573 |

140 replays cost $48.50 in total. Eight ended in a permission stall and are
recorded as errors, excluded from the counts rather than charged to the model.

**What this supports.** Deterministic assertions reproduced in 131 of 132 scored
replays. The one failure is a true positive: that replay passed its own
fail-to-pass test and broke an existing test by changing code it had not been
asked to touch. Only the broad no-regression suite caught it — its file
footprint was identical to every passing replay of the same exam, so a
footprint-based scope check would have missed it.

**What this does not support.** The pre-registered bar in [Status](#status) is
met on every clause as a point estimate, but only **one** of the four exams has
a 95% lower bound at or above 90%. The bounds are given to two decimals because
C's is 89.85%, which rounds to 90% and is not 90%; rounding a lower bound up is
the one direction this measurement cannot afford. A point estimate meeting a
floor is not the same as establishing it. These numbers establish that
assertions reproduce at a high rate, and only exam A supports the stronger
claim.

**Cost dispersion appears to track ambiguity more than effort.** Exam C, where
many different implementations are correct, is the outlier on every measure: a
coefficient of variation of 0.38 against 0.07–0.15 for the others, and a p90/p10
cost ratio of 2.58 against 1.20–1.35. Exam D is the control — it costs four
times exam A and takes three times the turns, and disperses more than A but far
less than C. Stated as a hypothesis rather than a result: four exams in one
repository is thin evidence, and C is a single point carrying most of the
contrast.

Two consequences follow for the design: a baseline-relative cost band cannot use
one global multiple, and an exam's band does not appear predictable from its
cost or difficulty, so it has to be calibrated from that exam's own series.

An earlier version of this section reported ten replays of three exams. That
sample understated dispersion on every exam and observed no failures at all;
both errors ran optimistic. A ten-replay series is enough to detect a gross
regression and not enough to characterise a distribution.

The exams were written to be single-goal because no such session existed in the
available history, so these figures are an optimistic bound. The full list of
disclosures is in [`spikes/s1/results/summary.md`](spikes/s1/results/summary.md).

## Distilling a real session

The calibration exams above were written by hand, which leaves the method's
central assumption untested: that a real session can be turned into a replayable
exam at all. Measured 2026-07-26 on one session from this machine's own history
— nine human turns over twelve and a half hours, two goals, and a mid-session
exchange that changed what got built. Method and raw rows are under
[`spikes/s3/`](spikes/s3/); the protocol and pass bar were committed before any
replay ran.

The distilled exam reproduced in **20 of 20 replays** across two arms, at a
median of $1.91 per replay. The second arm exists because the first proved
less than it appeared to: a replay runs in a git worktree of the real
repository, so the commit the exam was distilled from was one `git show` away.
The second arm removed that history and changed nothing else. It reproduced
just as well, which is what makes the first arm's result readable.

**Where the instruction turned out to live.** The session opens with the two
words `plan 15`, pointing at a file that is gitignored and no longer exists.
What actually got built was something else, because four minutes in the agent
asked a question and the human answered `1`. Both are recoverable — but from
tool inputs and tool results, not from anything the user typed. Across 54
sessions on this machine, 39% of human turns are four words or fewer, and the
median session's median turn is seven words. Steering is mostly pointers and
approvals; the content sits elsewhere in the transcript.

**What this does not show.** That distillation can be automated. The instruction
here was written by hand, and one session in one repository is not a sample.
What it establishes is narrower and was worth establishing first: a real
session's work can be posed as a replayable exam, and everything that posing it
required was present in the artefacts a tool would actually hold. The
disclosures are in
[`spikes/s3/results/summary.md`](spikes/s3/results/summary.md).

## Reading real transcripts

Both measurements above run through the layer that reads a harness's own
transcripts, so that layer is upstream of them. Measured 2026-07-26 against
1,681 local transcript files spanning 18 harness versions; method and figures
are under [`spikes/s2/`](spikes/s2/).

One parser read **all 1,681 files across all 18 versions**, refusing none. The
format does churn, but additively: every field the parser needs was present in
every version, and the changes land in metadata a replay does not read. Tool
calls and their results paired exactly, with none unmatched across 35,510.

Three assumptions did not survive. `session_id` is not another spelling of
`sessionId` — it names the session a transcript was resumed from, so treating
them as the same merges unrelated sessions. Some sessions move between working
directories partway through, so pinning one workspace per session is not always
right. And a small share of subagent transcripts cannot be traced back to the
call that started them, which has to be reported rather than quietly dropped.

**Redaction is not ready, and that is the honest limit here.** Exams are meant to
be shareable, which means a secret scanner that can be trusted. The one built
here catches every planted secret, and also flags one local file in two —
mostly on ordinary code, because a variable named `..._per_token` looks like a
credential to a pattern matcher. A scanner that noisy either blocks every export
or teaches you to ignore it. No shareable fixtures were produced for that
reason, and the criterion it was written against — catching planted secrets —
turned out to measure the easy half of the problem.

## Roadmap

Ordered by dependency. No dates: the order is a commitment, the schedule is not.

| Phase | What it delivers | Done when |
| --- | --- | --- |
| **0 — Calibration** | Variance measurement; transcript-parser fixtures across harness versions; one messy real session distilled and replayed end to end | The pre-registered bar in [Status](#status) is met, or the design changes in response to missing it |
| **1 — Bless and replay** | The loop end to end on a neutral core; the first capture and run adapters; distillation with mandatory review; deterministic assertion tiers; terminal reports; versioned machine contracts | A real session from a real repository replays and verdicts correctly, and the core provably contains no harness-specific code |
| **2 — Trust** | Escalating replays, flake quarantine, staleness detection, cross-family judging with published calibration, redaction | A deliberately degraded configuration is caught blind, with receipts |
| **3 — CI and watch** | CI integration with a sticky pull-request comment; triggers on model release, harness update, and configuration change; a zero-cost drift heartbeat | Running in real repositories, with the calibration numbers published here |
| **4 — Matrix and bisect** | Condition matrices; bisection across model, harness, backend, and configuration | Bisection isolates a planted regression across a sixteen-condition space within a stated budget |
| **5 — Any agent** | Adapters for further harnesses, plus a universal capture-and-replay path for agents with no adapter at all | One exam, authored once, runs on three harnesses with comparable verdicts |

## Related work

Assayer is one answer to a problem several projects approach from other angles,
and it is worth being clear about the differences.

- **[cc-canary](https://github.com/delta-hq/cc-canary)** computes
  statistics over session logs you have already produced and flags drift. It
  needs no replays and costs nothing to run, and it is looking at your real
  work rather than a fixed task. Assayer is the other trade: it spends money
  replaying a controlled task in order to remove the workload as a variable.
  The two answer different halves of the same question.
- **[promptfoo](https://github.com/promptfoo/promptfoo)** is a mature and much
  broader evaluation harness that can drive coding agents and assert on their
  trajectories. Its test cases are written by hand. Assayer's premise is that
  the sessions you already had are a better starting point than a suite you have
  to sit down and author.
- **[terminal-bench](https://github.com/harbor-framework/terminal-bench)** and
  its Harbor runner are a public benchmark with real containerised
  infrastructure. Their tasks are shared and hand-authored; Assayer's are yours
  and derived. Export to their format is on the roadmap — it is a useful
  destination, not a competitor.
- **Session capture and provenance tools** record what happened and who wrote
  what. That is a genuinely different job from re-running it under new
  conditions.

## Working on the repository

There is nothing to install yet. What follows is for developing Assayer itself.

Requires [Go](https://go.dev/dl/) 1.26 or newer.

```sh
make verify     # everything CI enforces, in one command

# or individually:
make build      # compile all packages
make vet        # go vet
make fmt-check  # fail if any file is not gofmt -s clean
make test       # unit tests
make race       # unit tests under the race detector
make cover      # tests plus the coverage threshold
make lint       # golangci-lint
make vuln       # govulncheck
```

`make verify` runs the same gates as CI: build, `go vet`, a `gofmt -s`
cleanliness check, tests under the race detector with at least 80% statement
coverage across `internal/...`, `golangci-lint`, and `govulncheck`. CI
additionally runs [CodeQL](.github/workflows/codeql.yml). The toolchain is Go
1.26 and golangci-lint v2.12.2.

## Project layout

```
cmd/assayer          command entrypoint, one line
internal/cli         command dispatch, output, and exit codes
internal/assay       the harness-neutral representation of a session
internal/port        the interfaces components are wired against
internal/adapter     capture adapters and the conformance kit they must pass
internal/buildinfo   version reporting for the running binary
internal/arch        the architecture test that enforces ADR-0005
docs/adr             architecture decision records
scripts/coverage.sh  race tests and the coverage gate
```

Small, and honestly so. The shape the rest of it will take is recorded in
[ADR-0005](docs/adr/0005-internal-package-layout.md), which also describes how
that shape is enforced: a package that appears without classifying itself does
not build, and no package outside an adapter may name a harness.

## Design decisions

Significant or hard-to-reverse choices are recorded as
[architecture decision records](docs/adr) in Michael Nygard's format:

- [0001 — Record architecture decisions](docs/adr/0001-record-architecture-decisions.md)
- [0002 — Name and module path](docs/adr/0002-name-and-module-path.md)
- [0003 — Go, a single static binary, driving harness CLIs](docs/adr/0003-go-single-binary-driving-harness-clis.md)
- [0004 — The conditions the S1 calibration was measured under](docs/adr/0004-s1-calibration-measurement-conditions.md)
- [0005 — The internal package layout and how it is enforced](docs/adr/0005-internal-package-layout.md)
- [0006 — The neutral session representation and the capture port](docs/adr/0006-session-representation.md)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for the workflow and the checks every
pull request must pass, and [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) for
expectations of conduct. Security issues should be reported privately as
described in [SECURITY.md](SECURITY.md).

Given the state of the repository, the most useful contributions today are
tooling fixes, corrections to these documents, and argument with the decisions
in `docs/adr` — before they harden.

## License

Licensed under the [Apache License 2.0](LICENSE).
