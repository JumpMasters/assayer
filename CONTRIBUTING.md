# Contributing

Thanks for your interest in Assayer. This document describes the development
workflow and the checks every change must pass.

## Before you start

Assayer is early. The design is settled but most of it is unbuilt, and the
current focus is the variance calibration described in the README — measuring
whether run-to-run noise is small enough for drift to be detectable at an
affordable number of replays. Until that produces numbers, changes that add
product surface are likely to be premature.

The most useful contributions right now are the unglamorous ones: build and
tooling fixes, documentation that is wrong or unclear, and design feedback on
the decisions recorded in [`docs/adr`](docs/adr). If you are planning anything
larger, please open an issue first so the approach can be agreed before you
spend time on it.

## Prerequisites

- [Go](https://go.dev/dl/) 1.26 or newer.
- [golangci-lint](https://golangci-lint.run) and
  [govulncheck](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck) for the
  lint and vulnerability gates:

  ```sh
  go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
  go install golang.org/x/vuln/cmd/govulncheck@latest
  ```

## Workflow

1. Create a branch from `main`.
2. Make your change, with tests.
3. Run `make verify` and make sure it passes.
4. Open a pull request. CI must be green before a change can merge; `main` is
   protected and does not accept direct pushes or force-pushes.

```sh
make verify   # build, vet, gofmt check, race tests, coverage, lint, govulncheck
```

## Standards

- **Tests.** New code comes with tests. The suite must hold at least 80%
  statement coverage across `internal/...`; CI enforces this.
- **Formatting.** Code must be `gofmt -s` clean. Run `make fmt` to format.
- **Linting.** `golangci-lint` must pass with the repository configuration in
  [`.golangci.yml`](.golangci.yml).
- **Commits.** Write clear, imperative commit messages that explain the change
  and its motivation.

## Two rules that are not style preferences

Both exist because the failure they prevent would quietly destroy the point of
the tool. Treat a change that weakens either as needing an ADR, not a review
comment.

- **No harness or vendor names outside an adapter.** Everything downstream of a
  capture adapter works on a neutral representation of a session. If a fix looks
  easier with a check for a particular harness in shared code, the fix is in the
  wrong place. This is mechanically enforced once the package layout lands.
- **Assayer's own failures are never regressions.** A crashed harness, a denied
  network call, an exhausted budget, or a workspace pin that no longer applies
  must produce an error or a staleness verdict, never a failure verdict. An
  instrument that blames its own plumbing on the thing it is measuring is worse
  than no instrument.

## Architecture decisions

Significant or hard-to-reverse decisions are recorded as Architecture Decision
Records under [`docs/adr`](docs/adr), in Michael Nygard's format. If your change
makes such a decision, add an ADR for it. Records are immutable once accepted:
supersede them rather than editing them.

## Reporting security issues

Please follow [SECURITY.md](SECURITY.md) for vulnerabilities rather than opening
a public issue.
