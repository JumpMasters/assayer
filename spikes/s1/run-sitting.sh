#!/usr/bin/env bash
# One S1 sitting: isolate in a worktree, run the harness, evaluate, append one CSV row.
# Exits 0 even on a failed or errored sitting — that outcome is the measurement.
set -euo pipefail

EXAM_DIR=$(cd "$1" && pwd); N=$2; CSV=$3
SPIKE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=/dev/null
source "$EXAM_DIR/pin.env"

now_ms() { python3 -c 'import time; print(int(time.time()*1000))'; }

WT=$(mktemp -d)/wt
git -C "$REPO" worktree add --detach "$WT" "$BASE_SHA" >/dev/null 2>&1
cleanup() { git -C "$REPO" worktree remove --force "$WT" >/dev/null 2>&1 || true; }
trap cleanup EXIT
# `if` rather than `[ ... ] && ...` purely for legibility; bash exempts a failing
# test inside an AND-OR list from `set -e`, so both forms are safe here.
if [ -f "$EXAM_DIR/workspace.patch" ]; then
  git -C "$WT" apply "$EXAM_DIR/workspace.patch"
fi

RAW="$SPIKE/results/raw/${EXAM_ID}-${N}.json"
mkdir -p "$(dirname "$RAW")"

start=$(now_ms)
set +e
( cd "$WT" && env -u ANTHROPIC_API_KEY "${HARNESS:-claude}" -p \
    --output-format json \
    --model claude-opus-4-8 \
    --settings "$SPIKE/empty-settings.json" \
    --disable-slash-commands \
    --strict-mcp-config \
    --permission-mode acceptEdits \
    --allowedTools "Read" "Edit" "Write" "Glob" "Grep" "Bash(uv:*)" "Bash(python3:*)" \
    --max-budget-usd "$CAP_USD" \
    "$(cat "$EXAM_DIR/instruction.md")" ) > "$RAW" 2>"$RAW.err"
harness_rc=$?
set -e
wall_ms=$(( $(now_ms) - start ))

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
canon = next((v.get("canonicalModel", "unknown") for v in mu.values()), "unknown")
err = 1 if (rc != 0 or d.get("is_error") or d.get("subtype") != "success") else 0
print(d.get("total_cost_usd", 0), d.get("num_turns", 0),
      len(d.get("permission_denials") or []), canon, err,
      (d.get("stop_reason") or "none"))
PY
)

# A permission stall is ERROR, not FAIL — the sitting produced no verdict.
if [ "$denials" != "0" ]; then is_error=1; fi

run_set() {  # $1 = node-id file; 1 if every node passes (or the set is empty), else 0
  [ -s "$1" ] || { echo 1; return; }
  while read -r nid; do
    [ -z "$nid" ] && continue
    ( cd "$WT" && uv run pytest -q "$nid" ) >/dev/null 2>&1 || { echo 0; return; }
  done < "$1"
  echo 1
}

if [ "$is_error" = "1" ]; then
  f2p=0; p2p=0
else
  f2p=$(run_set "$EXAM_DIR/f2p.txt")
  p2p=$(run_set "$EXAM_DIR/p2p.txt")
fi

touched=$(git -C "$WT" status --porcelain | wc -l | tr -d ' ')

printf '%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s\n' \
  "$EXAM_ID" "$N" "$f2p" "$p2p" "$cost" "$turns" "$wall_ms" \
  "$model" "$denials" "$touched" "$is_error" "$stop" >> "$CSV"
