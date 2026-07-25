# 0002 — Name and module path

- Status: Accepted
- Date: 2026-07-25

## Context

The project was designed under the working name *Proctor*. Publishing fixes a
Go module path, and a module path is expensive to change afterwards: it is
embedded in every import, in installation instructions, in any package index
that has cached it, and in whatever links accumulate. The name had to be settled
before the first public commit, not after it.

*Proctor* had three problems.

- **Package registries.** The name is taken on PyPI by an unrelated test runner
  and has been held on npm since 2014. A command-line tool that may later want
  either channel would start blocked.
- **Same-domain collision.** `indeedeng/proctor` is a well-known A/B *testing*
  framework. A tool whose subject is testing, sharing a name with another
  testing tool, produces exactly the confusion a name exists to prevent. The
  same objection removed *Touchstone* from the list: `taoensso/touchstone` is
  also an A/B testing library.
- **Connotation.** In ordinary use a proctor watches an examinee for
  misconduct. Assayer measures its own operator's workflow, on their machine,
  with nothing transmitted anywhere. A surveillance reading is the opposite of
  what the tool does, and it is the first reading most people have.

## Decision

The project is named **Assayer**, and the module path is
`github.com/JumpTechCode/assayer`.

An assayer determines the composition of a sample by controlled test and issues
a certificate stating the result in figures. That is a close description of what
this tool does: administer a controlled replay and report what was measured,
with the conditions and counts attached. The verb "assay" is available and
accurate; "proctor" as a verb was neither.

The examination vocabulary the design is built on — blessing a session, an exam,
a sitting, a series, a verdict — is unaffected and is retained in full.

Alternatives considered and rejected:

- **Proctor** (keep the working name): the three problems above, none of which
  improve with time.
- **Invigilator**: registry is clear and the semantics are exact, but it is
  literally the word for a person who supervises an examination, which
  intensifies the connotation problem rather than resolving it. Five syllables
  at every call site is a real cost for a command typed daily.
- **Touchstone**: pleasant metaphor, but taken by a widely used JavaScript
  project and by an A/B testing library, reproducing the collision that ruled
  out the working name.

## Consequences

- The module path is fixed before publication, which is the only cheap moment to
  fix it.
- Internal design notes written under the working name are inconsistent with the
  published name until they are revised. Public documentation uses Assayer
  throughout.
- The binary, the configuration directory (`.assayer/`), and the environment
  variable prefix (`ASSAYER_`) all follow from this decision and change with it,
  so a later rename would be broader than a module path alone. This is the
  record that should be superseded if the name is ever revisited.
