#!/usr/bin/env bash
# One S1 sitting: isolate in a worktree, run the harness, evaluate, append one CSV row.
# Exits 0 even on a failed or errored sitting — that outcome is the measurement.
#
# Exam directory contract: `workspace.patch`, if present, is applied before the harness
# runs. `tests.patch`, if present, holds the exam's own verification tests and is applied
# after the harness runs, once any edits the agent made under tests/ are discarded — the
# SWE-bench shape. A sitting starts from BASE_SHA, where those tests do not exist yet, so
# an agent can neither guess a node id's name to pass it nor pass it by writing or editing
# a test itself.
set -euo pipefail

EXAM_DIR=$(cd "$1" && pwd); N=$2; CSV=$3
SPIKE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=/dev/null
source "$EXAM_DIR/pin.env"

now_ms() { python3 -c 'import time; print(int(time.time()*1000))'; }

WT_PARENT=$(mktemp -d)
WT="$WT_PARENT/wt"
cleanup() {
  git -C "$REPO" worktree remove --force "$WT" >/dev/null 2>&1 || true
  rm -rf "$WT_PARENT"
}
# Armed before the worktree exists, so a failing `worktree add` still removes the parent.
trap cleanup EXIT
git -C "$REPO" worktree add --detach "$WT" "$BASE_SHA" >/dev/null 2>&1
# `if` rather than `[ ... ] && ...` purely for legibility; bash exempts a failing
# test inside an AND-OR list from `set -e`, so both forms are safe here.
if [ -f "$EXAM_DIR/workspace.patch" ]; then
  git -C "$WT" apply "$EXAM_DIR/workspace.patch"
fi

RAW="$SPIKE/results/raw/${EXAM_ID}-${N}.json"
mkdir -p "$(dirname "$RAW")"

# `--settings` only adds settings on top of the user's `~/.claude/` config; it does not
# replace it. `--setting-sources project` is what actually excludes user-level hooks,
# plugins, and memory injection — without it, a memory plugin's growing store would vary
# between sittings and be recorded as model variance.
start=$(now_ms)
set +e
( cd "$WT" && env -u ANTHROPIC_API_KEY "${HARNESS:-claude}" -p \
    --output-format json \
    --model claude-opus-4-8 \
    --settings "$SPIKE/empty-settings.json" \
    --setting-sources project \
    --disable-slash-commands \
    --strict-mcp-config \
    --permission-mode acceptEdits \
    --allowedTools "Read" "Edit" "Write" "Glob" "Grep" "Bash(uv:*)" "Bash(python3:*)" \
    --max-budget-usd "$CAP_USD" \
    "$(cat "$EXAM_DIR/instruction.md")" ) > "$RAW" 2>"$RAW.err"
harness_rc=$?
set -e
wall_ms=$(( $(now_ms) - start ))

# Captured now, before tests/ is reset to the exam's own state below, so this is the
# agent's footprint — not inflated by the exam's own test patch landing afterward.
# -uall: without it an entirely untracked directory collapses to one entry.
touched=$(git -C "$WT" status --porcelain -uall | wc -l | tr -d ' ')

# Parse the result JSON. A harness that crashed leaves unparseable output: that is ERROR.
read -r cost turns denials model is_error stop < <(
  python3 - "$RAW" "$harness_rc" <<'PY'
import json, sys
try:
    d = json.load(open(sys.argv[1]))
except Exception:
    print("0 0 0 unknown 1 unparseable"); sys.exit()
rc = int(sys.argv[2])
mu = d.get("modelUsage") or {}
# Every model the sitting billed, not just the first: a session can carry a second
# entry for internal or subagent calls, and naming only one would misattribute the row.
canon = "|".join(mu) or "unknown"
err = 1 if (rc != 0 or d.get("is_error") or d.get("subtype") != "success") else 0
print(d.get("total_cost_usd", 0), d.get("num_turns", 0),
      len(d.get("permission_denials") or []), canon, err,
      (d.get("stop_reason") or "none"))
PY
)

# A permission stall is ERROR, not FAIL — the sitting produced no verdict.
if [ "$denials" != "0" ]; then is_error=1; fi

# Same seam as HARNESS, and for the same reason: the test substitutes a stub pytest.
read -ra PYTEST_CMD <<<"${PYTEST:-uv run pytest}"

# $1 = node-id file. Sets $set_pass (1 if every node passed, 0 otherwise) and
# $set_error. pytest exits 0 for pass and 1 for a failed assertion; every other code
# means the test never ran a verdict — a missing node id, a collection error, a broken
# environment. Those are ERROR, never FAIL, so an unknown code can never manufacture a
# regression. An unreadable node file is our own fault and is ERROR too; a readable but
# empty one is legitimately vacuous and passes.
run_set() {
  set_pass=1; set_error=0
  if [ ! -r "$1" ]; then set_pass=0; set_error=1; return; fi
  # `|| [ -n "$nid" ]` so a final line with no trailing newline is still evaluated.
  while read -r nid || [ -n "$nid" ]; do
    [ -z "$nid" ] && continue
    rc=0
    # </dev/null so pytest cannot swallow the remaining node ids; `cd` failure exits
    # non-{0,1} so it too reads as ERROR rather than as a failed assertion.
    ( cd "$WT" || exit 99; "${PYTEST_CMD[@]}" -q "$nid" ) >/dev/null 2>&1 </dev/null \
      || rc=$?
    case "$rc" in
      0) ;;
      1) set_pass=0 ;;                      # keep going: a later node may be an ERROR
      *) set_pass=0; set_error=1; return ;;
    esac
  done < "$1"
}

# Discards the agent's edits under tests/, then applies the exam's own tests — see the
# header comment for why. No tests.patch means an older-shape exam: leave tests/ alone
# so the existing stub fixtures keep working unchanged. A restore or apply failure is
# ERROR, never a failed assertion: the sitting produced no valid verdict either way.
apply_exam_tests() {
  [ -f "$EXAM_DIR/tests.patch" ] || return 0
  # cat-file -e tells us whether tests/ was ever tracked at BASE_SHA; a bare `checkout`
  # errors on a path that wasn't, which is the ordinary case for a freshly authored exam.
  if git -C "$WT" cat-file -e "$BASE_SHA:tests" 2>/dev/null; then
    git -C "$WT" checkout "$BASE_SHA" -- tests || return 1
  fi
  git -C "$WT" clean -fdq -- tests || return 1
  git -C "$WT" apply "$EXAM_DIR/tests.patch" || return 1
}

if [ "$is_error" != "1" ] && ! apply_exam_tests; then is_error=1; fi

if [ "$is_error" = "1" ]; then
  f2p=0; p2p=0
else
  run_set "$EXAM_DIR/f2p.txt"; f2p=$set_pass; f2p_error=$set_error
  run_set "$EXAM_DIR/p2p.txt"; p2p=$set_pass
  if [ "$f2p_error" = "1" ] || [ "$set_error" = "1" ]; then is_error=1; fi
fi

printf '%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s\n' \
  "$EXAM_ID" "$N" "$f2p" "$p2p" "$cost" "$turns" "$wall_ms" \
  "$model" "$denials" "$touched" "$is_error" "$stop" >> "$CSV"
