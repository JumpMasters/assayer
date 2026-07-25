# spikes

Calibration scaffolding for measurements taken before Assayer has product code.

Everything here is deliberately minimal and is **not** covered by the quality bar that
applies to `internal/` and `cmd/`. It is committed rather than kept local because the
numbers it produces are published in the README, and a published measurement whose
method cannot be inspected is an assertion rather than evidence.

- `s1/` — variance calibration. Answers whether run-to-run variance at the assertion
  level is small enough for drift to be detectable at an affordable number of replays.
  Protocol and pass bar are fixed in advance; see the README's Status section.

Raw session transcripts and raw harness output are never committed.
