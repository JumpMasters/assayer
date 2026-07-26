# 0005 — The internal package layout and how it is enforced

- Status: Accepted
- Date: 2026-07-26

## Context

Assayer's usefulness rests on one property: nothing it does may depend on which
coding agent produced a session. A result that only holds for one vendor's tool
is not a measurement of that tool, it is a measurement of our coupling to it.
Everything except an adapter therefore has to operate on a neutral
representation and stay ignorant of where that representation came from.

That is easy to state and easy to erode. Erosion does not usually arrive as a
deliberate decision; it arrives as a shortcut under time pressure, in a diff
that looks small. A single branch on a harness name in a shared package will
work, will pass every test, and will quietly make the second supported harness a
rewrite rather than an addition.

So the property needs a mechanism, and the mechanism has to be in place before
there is any code to protect. Until now there was none: the package graph did
not exist, so the linter rules that constrain it could not be written, and
`.golangci.yml` has carried a comment saying so.

A second, smaller consideration. This repository gates test coverage across
`internal/...`, so program assembly and the mapping from a result to a process
exit code — which live in `package main` by Go convention — sit outside the
measured set. They are cheap to move inside it.

## Decision

### The graph

The internal graph is a star. Packages are assigned to one of six classes, and
a package's permitted internal imports follow from its class rather than from a
per-package permission:

| Class | Packages | May import (internal) |
|---|---|---|
| leaf | `assay` | nothing |
| ports | `port` | `assay` |
| component | `registrar`, `distiller`, `invigilator`, `examiner`, `adjudicator`, `ledger`, `reporter` | `assay`, `port` |
| adapter | `adapter/**` | `assay`, `port`, `adapter/shared`; in `_test.go` also `adapter/conformance` |
| root | `app` | any of the above |
| root | `cli` | `app`, `assay`, `buildinfo` |
| standalone | `buildinfo`, `arch` | nothing |

`cmd/assayer` imports `internal/cli` and nothing else.

`assay` holds the neutral session representation and the other domain types.
`port` holds the interfaces over them, so it is leaf-plus-one rather than a
second leaf; splitting the two keeps the package everything imports from
becoming the place shared types collect.

Adapters are grouped by harness first and by seam second — capture and run are
separate packages under a shared harness directory. The harness is the outer
directory because it is the unit that would be extracted if adapters ever ship
out of tree. The seams are separate packages because a transcript parser and a
subprocess runner share nothing but a name. Code genuinely common to several
adapters lives in `adapter/shared`, which only adapters may import.

Because components cannot call each other, the sequence a command performs —
load an exam, select an adapter, administer a sitting, evaluate it, record it,
report it — can only be assembled at the root. The star therefore guarantees the
root carries the program's control flow, so the root is split by job: `app`
constructs the object graph and owns that sequence, knowing nothing about
arguments; `cli` owns flag parsing, human-readable output, and the exit-code
mapping. `cli.Run` returns an `int`, not an `error`, because a verdict is a
successful run reporting a result — exit codes 70 to 73 are not failures, and an
`error` return cannot express them. `cmd/assayer/main.go` is one line calling
it. The whole 0 / 2 / 70–73 mapping is then inside the coverage gate.

**Today only `buildinfo`, `cli`, `arch`, and `cmd/assayer` exist.** The rest of
the table is intent, and each package arrives with its own change. The
classification is the enforced configuration; a package absent from it does not
build.

### Enforcement

**depguard**, in `.golangci.yml`, using `list-mode: lax` with a `deny` on this
module's own import prefix and `allow` exceptions per class. A strict allow-list
was tried and rejected: it denies the standard library, so every package
importing `fmt` fails. The rules for the adapter class exclude `_test.go` from
the broad rule, because overlapping depguard rules are combined rather than
overridden and a narrower test rule cannot otherwise widen a broader one.

**An architecture test** in `internal/arch`, which parses the tree and fails on:

1. any import — internal or external — outside an explicit allowance;
2. any package under `internal/` absent from the classification;
3. any vendor word appearing in a file under `internal/` or `cmd/`, outside
   the adapter tree, `internal/app/adapters.go`, and `internal/arch` itself;
4. a package-level variable or an `init` function in `assay` or `port`, and any
   exported field there typed `any`, `interface{}`, `map[string]any`, or
   `json.RawMessage`;
5. any control-flow statement in `internal/app/adapters.go`;
6. any difference between the depguard block in `.golangci.yml` and the same
   block rendered from the classification.

Check 6 renders rather than parses. The classification lives in Go, the linter
configuration is generated from it between sentinel comments, and the test
asserts the generated text appears verbatim on disk. Parsing the configuration
instead would have required a YAML library — this module has no third-party
dependencies, and acquiring the first one to support a test is a poor trade.

## Consequences

- The two mechanisms are not redundant. depguard evaluates the rules someone
  wrote and permits, by default, any file no rule matches. The architecture test
  is default-deny: a package added without being classified fails. That is the
  check that survives being added to in a hurry.
- Permissions follow from a class rather than from a row per package. A row per
  package would make widening the star a thirty-second append that satisfies
  every check — declare the package, list the extra edge, and the two files
  still agree. Requiring a class means widening is an edit to a rule that reads
  like a decision.
- Scanning source text for vendor vocabulary, and not only imports, is
  deliberate: the failure this exists to prevent — comparing against a harness
  name in a shared package — imports nothing and satisfies any import-based
  rule. The scan covers every file type, not only Go source, because embedded
  configuration and templates are ordinary Go and would otherwise be an
  unwatched surface beside core.
- Matching is case-insensitive substring, which is both simpler and stricter
  than matching whole words or split segments. Segment matching was specified
  first and rejected: it accepts `claudecode`, which is this project's own
  spelling for the directory holding its reference adapter, while rejecting
  `claude-code`. A rule that gives opposite answers to two spellings of one word
  is worse than no rule, because it reads as protection.
- The word list therefore holds only strings with no ordinary programming
  meaning, and grows when work on a vendor's adapter begins rather than when it
  merges — the window in which coupling can be introduced opens before the code
  lands. Words that are also ordinary nouns, `cursor` most obviously, stay off
  the list and are matched as vendor-qualified pairs when that adapter is
  written. Here a false positive breaks a build rather than adding noise to a
  report, and this project has already seen what an unmeasured detector costs:
  the phase 0 redaction work caught every planted secret and still could not be
  used as a gate, because it fired on one local file in two, mostly on ordinary
  code. Its precision was characterised rather than measured, and no figure for
  it is claimed here or there.
- **The guard checks topology and vocabulary. It cannot see a vendor-shaped
  neutral representation.** A field named `SidechainUUID`, or a `Meta` map read
  by a component, uses no vendor word and crosses no illegal edge, and would
  make core single-harness while every check passes. The structural rules above
  close the generic-escape-hatch version of this; the named-field version is
  governed by review, by the record that specifies the representation, and by a
  second deliberately impoverished implementation in the adapter conformance kit
  — a stub harness with no subagents, no tool results, no version field and one
  working directory, so an interface only one harness can satisfy fails at build
  time rather than when the second adapter is attempted. That kit, not this
  guard, is the instrument that demonstrates neutrality.
- The root may import everything, so moving code into `app` is always a legal
  way to make two components cooperate. Nothing mechanical prevents the product
  from accumulating there. This is the cost of forbidding component-to-component
  imports and it is accepted knowingly rather than solved.
- The root must name the adapters it wires, so a complete ban on vendor
  vocabulary is not achievable. The exemption is one file containing a
  registration table, and that file is additionally forbidden from branching, so
  it can hold names without holding decisions.
- The scan targets accident and drift, not evasion. Assembling a vendor name
  from concatenated constants defeats it, as it would defeat any check short of
  full type evaluation. That is a review problem and is not claimed otherwise.
- The architecture test can only see this module. An adapter's own dependency on
  a vendor's client library is invisible to it and is governed by review.
- Test files are held to every rule. The only test-scoped relaxation is an
  adapter's import of the conformance kit. A core package that couples to one
  harness in its tests has failed the same property in the most expensive place
  to discover it.
