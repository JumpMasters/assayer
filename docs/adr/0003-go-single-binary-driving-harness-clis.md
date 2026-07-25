# 0003 — Go, a single static binary, driving harness CLIs

- Status: Accepted
- Date: 2026-07-25

## Context

Assayer has to run in two places that tolerate very little: a developer's
machine, where a language runtime and a dependency tree are friction that
prevents installation, and a CI runner, where every second of setup is paid on
every job. It also has to drive coding agents — starting them non-interactively,
reading what they did, and measuring what they spent.

There are two ways to reach an agent. One is a vendor's software development
kit. The other is the agent's own command-line interface and the
machine-readable output it already emits.

An SDK is the more obvious choice and the wrong one here. Assayer's central
constraint is that no component outside an adapter may know which agent produced
a session or ran a replay; a vendor SDK is a compile-time dependency on exactly
that knowledge, and taking one for the first supported harness would make the
second harness a rewrite rather than an addition. It is also unnecessary:
transcripts, streamed events, and result objects — the things capture and
measurement actually need — are already emitted by the CLIs, and are the same
surfaces a user can inspect by hand when they want to check our work.

## Decision

Assayer is written in Go and distributed as a single static binary, with no
runtime dependency beyond the binary itself. It interacts with agent harnesses
by executing their command-line interfaces and consuming their documented
machine-readable output. It takes no vendor SDK dependency.

Every harness integration is therefore an adapter over a subprocess and its
output, which is a shape any harness can satisfy — including one whose vendor
has never heard of this project.

## Consequences

- Installation is a binary. Cross-compilation to the platforms developers and CI
  runners actually use is a build matrix, not a packaging project.
- Adding a harness means writing an adapter against its CLI, which does not
  touch shared code and cannot be blocked by an SDK's release schedule.
- Assayer inherits whatever guarantees the CLI exposes and no more. Where a
  harness offers a native spend cap or a flag that ignores ambient
  configuration, the adapter uses it; where it does not, Assayer falls back to
  what it can enforce from outside — wall clock, and process control. These
  differences are real, they vary per harness, and every report has to state
  which guarantees were in force rather than implying a uniform standard.
- Anything an SDK exposes but a CLI does not is unavailable. This is accepted:
  the capture and measurement surfaces needed here are all present in CLI
  output, and the alternative costs the neutrality the project is built on.
- Subprocess handling — timeouts, orphan-proof termination, output that arrives
  faster than it is consumed — becomes our problem rather than a library's, and
  needs to be correct rather than approximately correct.
