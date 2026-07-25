# Security Policy

## Reporting a vulnerability

Please report security vulnerabilities privately. **Do not open a public issue
for a security report.**

Use GitHub's private vulnerability reporting: open the repository's **Security**
tab and choose **Report a vulnerability**. This creates a private advisory
visible only to the maintainers.

When reporting, please include:

- a description of the issue and its impact,
- the steps or a proof of concept needed to reproduce it,
- the affected version or commit, and
- any suggested remediation, if you have one.

You can expect an acknowledgement within a few days. We will confirm the issue,
keep you informed of progress, and coordinate disclosure once a fix is
available.

## Handling of session data

Assayer is designed to read agent session transcripts, which routinely contain
real source code, real filesystem paths, and sometimes real credentials. Two
commitments follow from that, and they apply to every change:

- **Nothing is transmitted.** Assayer has no server, no telemetry, and no
  hosted component. The only path by which data leaves the machine is an
  explicit, redacting export command run by the user.
- **Redaction gates distillation.** A secret-pattern scan runs before an exam
  is written, and findings block the write until they are resolved or
  explicitly waived.

Neither is implemented yet — no distillation code exists at this stage. They are
recorded here because they are design constraints on the code that will be
written, not features to be added afterwards. A change that weakens either one
needs an architecture decision record explaining why.

## Supported versions

Assayer is in early development and does not publish releases yet. Security
fixes are applied to the `main` branch.
