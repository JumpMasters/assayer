# Where each clause of the instruction came from

Clause 3 of the pre-registered bar (`../README.md`) asks whether the distilled
instruction traces to artefacts a tool would actually hold at bless time. Those
are two: the **session transcript** (`~/.claude/projects/…/5d05e0b9-….jsonl`,
including tool-call inputs and tool results, not only the human turns) and the
**repository at the base SHA** (`fc5110f`).

A third source is disqualified by construction and named here so its absence is
checkable: the author's memory of having been in the session.

## The chain the instruction was recovered through

The human turn that started this work is the two words **`plan 15`**. Its
referent, `docs/superpowers/plans/…`, is gitignored in tollgate, and the file it
pointed at no longer exists on disk. Read literally, the ask is unrecoverable.

It also turns out to be the wrong ask. Four minutes in, the agent asked a
sequencing question and the human answered `1`. That answer re-scoped the work
from plan 15 to a precursor — the cache-creation token class — and that
precursor is what the session then built.

Both facts are recoverable from the transcript without inference:

| Step | Where it lives | Recoverable? |
|---|---|---|
| Human says `plan 15` | user turn | yes, but its referent is gone |
| Agent asks how to sequence cache-creation | `AskUserQuestion` tool input | yes |
| Human answers `1` | user turn | yes, and ambiguous alone |
| **`1` resolved to a named option and its spec** | the **tool result**, which records the selected label and its preview text verbatim | **yes, mechanically** |
| Full implementation spec | a `Write` tool input — the agent authored a 41 KB plan file in-session | yes |
| Existing `cached_input_*` chain the new field mirrors | repo at `fc5110f` | yes |

The load-bearing one is the fourth row. A distiller reading only human turns
recovers `1`. A distiller reading tool results recovers
`"Precursor mini-plan, before plan 15"` together with the option's own preview
text, which already names both new identifiers. No inference from `1` to an
ordinal position is required.

## Clause by clause

`T` = transcript, `R` = repo at the base SHA, `H` = exam-format boilerplate that
is not a claim about the session.

| Instruction clause | Source | Note |
|---|---|---|
| Cost model prices input, output, cached input today | R | `src/tollgate/domain/pricing.py` |
| Add a fourth class, cache creation / cache-write | T | question header; plan Goal |
| Anthropic bills it; Tollgate undercounts today | T | question text; plan Architecture |
| Thread it along the chain `cached_input_*` follows — price-book column, price repo, cost model, command/usage types, wire schemas, SDK, commit and grace handlers and their fingerprints | T | plan Architecture, near-verbatim; the selected option's preview lists the same set more briefly |
| Names `cache_creation_micro_per_token` and `cache_creation_tokens` | T | both appear verbatim in the **selected option's preview**, before the plan file existed |
| The rate may exceed the input rate; the cached-input constraint must not apply to it | T | plan Goal and Architecture |
| Rates stay non-negative, consistent with the existing check | R | existing `ModelPrice.__post_init__` |
| The count is disjoint and additive, not a subset of input | T | plan Architecture, verbatim |
| Reconcile-time only: priced on the actual-cost path, never in the reserve estimate | T | plan Architecture, verbatim |
| The usage count defaults to zero; existing callers and stored rows unchanged; reserve estimate does not move | T | plan §Global constraints and §Invariants |
| Change only `src/` and `migrations/`; tests are supplied | H | same contract S1's exams carry |

**No clause traces only to memory.** Clause 3 of the bar is met on this exam.

## One judgement call, recorded because it cuts the other way

The plan file the agent wrote **contains the golden's test bodies verbatim**,
including the name `test_cache_creation_defaults_to_zero_and_leaves_existing_cost_unchanged`
and the exact fixture rates and expected micro-USD totals.

None of that was carried into the instruction. The instruction states the
behaviour in prose and names only the two identifiers the tests bind to; it
contains no test name, no fixture value, and no expected total.

That is a deliberate line, and it is the line a real distiller has to draw too:
the session's own artefacts mix the ask with the oracle, and a distiller that
inlines the oracle into the instruction is not measuring whether work reproduces,
it is measuring whether an agent can transcribe. Drawing the line the other way —
omitting `defaults to zero`, which is a spec fact the source states plainly and
which existing callers depend on — would have made the exam unfairly hard for a
reason that has nothing to do with distillation. Spec facts in, oracle out.

The identifiers are the unavoidable exception. The golden's tests bind to
`cache_creation_micro_per_token` and `cache_creation_tokens` at module-fixture
level, so an instruction that withheld them would measure naming luck rather
than reproduction. They are carried because the transcript states them, not
because the tests need them — but the tests needing them is why leaving them out
was never an option, and that asymmetry is worth saying out loud.
