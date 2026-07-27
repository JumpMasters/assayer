# Recorded results

Three real runs of the harness, kept verbatim. They are what this adapter's
tests read instead of structs written from a schema.

Recorded on 2026-07-26 against Claude Code **2.1.220** (`darwin/arm64`), on a
first-party API key, in an empty temporary directory. Total cost of recording
all three: about $0.020.

| File | Command | Exit | Subtype |
|---|---|---|---|
| `success.json` | `claude -p --bare --output-format json --setting-sources ''` | 0 | `success` |
| `turns.json` | the same, plus `--allowedTools Bash --max-turns 1 --max-budget-usd 0.50` | 1 | `error_max_turns` |
| `budget.json` | the same, plus `--allowedTools Bash --max-budget-usd 0.001` | 1 | `error_max_budget_usd` |

The exit statuses are here because they are not in the files: these are stdout,
and the status is the fact that matters most. Both cap hits exited **1**, which
is also what the harness returns when it has genuinely fallen over — so the exit
code cannot be what decides whether a sitting reached a verdict, and the adapter
reads the subtype instead.

Two other things these runs established, both pinned as tests:

- `turns.json` reports `num_turns: 2` against a cap of 1. A cap is a threshold
  noticed after it has been crossed.
- `budget.json` cost $0.00472475 against a cap of $0.001, and its `usage` block
  is zeroed while `total_cost_usd` is not. A budget-stopped sitting can report
  spending money and no tokens.

The files are unedited apart from being re-indented for review. Nothing is
trimmed to the fields the adapter reads: a fixture reduced to what the parser
wants cannot show the parser reading the wrong thing, and cannot show the
harness changing shape around it.

Re-recording is a deliberate act, not a refresh. These runs cost money, and a
new release changing what they contain is a finding rather than an
inconvenience — the version above is what makes that visible.
