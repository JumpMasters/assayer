# spikes

Calibration scaffolding for measurements taken before Assayer has product code.

Everything here is deliberately minimal and is **not** covered by the quality bar that
applies to `internal/` and `cmd/`. It is committed rather than kept local because the
numbers it produces are published in the README, and a published measurement whose
method cannot be inspected is an assertion rather than evidence.

- `s1/` — variance calibration. Answers whether run-to-run variance at the assertion
  level is small enough for drift to be detectable at an affordable number of replays.
  Protocol and pass bar are fixed in advance; see the README's Status section.
- `s3/` — distillation rehearsal. Answers whether one real, messy, multi-turn session
  can be folded into a single headless instruction that replays. S1 measured
  hand-authored exams; this measures a distilled one. Protocol and pass bar are fixed
  in advance in `s3/README.md`, and it reuses `s1/`'s sitting runner unmodified.

Raw session transcripts and raw harness output are never committed.
