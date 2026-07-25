# Architecture Decision Records

This directory records the significant, hard-to-reverse decisions made while
building Assayer. Each record captures the context, the decision, and its
consequences, so the reasoning is available later even when the people change.

The format follows Michael Nygard's
[Documenting Architecture Decisions](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions.html).

Records are added when a decision is made and acted on, not in advance of one.
A short index today reflects a young repository, not an undocumented one.

## Index

- [0001 — Record architecture decisions](0001-record-architecture-decisions.md)
- [0002 — Name and module path](0002-name-and-module-path.md)
- [0003 — Go, a single static binary, driving harness CLIs](0003-go-single-binary-driving-harness-clis.md)

## Adding a record

Copy the structure of an existing record, give it the next number, and set its
status. Records are immutable once accepted: to change a decision, add a new
record that supersedes the old one and update the older record's status.
