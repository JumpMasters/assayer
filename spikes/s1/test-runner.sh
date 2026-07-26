#!/usr/bin/env bash
# Proves the runner's isolation, parsing, and CSV logic without spending a sitting.
set -euo pipefail
SPIKE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT

# A throwaway repo to act as the pinned workspace.
git init -q "$TMP/repo" && cd "$TMP/repo"
git -c user.email=t@t -c user.name=t commit -q --allow-empty -m base
SHA=$(git rev-parse HEAD)

EXAM="$TMP/exam"; cp -R "$SPIKE/fixtures/exam-stub" "$EXAM"
{ echo "REPO=$TMP/repo"; echo "BASE_SHA=$SHA"; } >> "$EXAM/pin.env"

CSV="$TMP/out.csv"
HARNESS="$SPIKE/fixtures/stub-harness.sh" bash "$SPIKE/run-sitting.sh" "$EXAM" 1 "$CSV"

row=$(cat "$CSV")
echo "row: $row"
IFS=, read -r exam n f2p p2p cost turns wall model denials touched err stop <<<"$row"
[ "$exam" = "stub" ]                || { echo "FAIL: exam id: $exam"; exit 1; }
[ "$cost" = "0.42" ]                || { echo "FAIL: cost not parsed: $cost"; exit 1; }
[ "$model" = "claude-opus-4-8" ]    || { echo "FAIL: canonical model: $model"; exit 1; }
[ "$err" = "0" ]                    || { echo "FAIL: clean run marked error"; exit 1; }
[ "$touched" = "1" ]                || { echo "FAIL: touched files: $touched"; exit 1; }
[ "$(git -C "$TMP/repo" worktree list | wc -l)" -eq 1 ] \
  || { echo "FAIL: worktree leaked"; exit 1; }
echo "PASS"

cat > "$TMP/bad-harness.sh" <<'EOF'
#!/usr/bin/env bash
echo "boom" >&2; exit 1
EOF
chmod +x "$TMP/bad-harness.sh"
CSV2="$TMP/out2.csv"
HARNESS="$TMP/bad-harness.sh" bash "$SPIKE/run-sitting.sh" "$EXAM" 2 "$CSV2" \
  || { echo "FAIL: runner aborted on a failed sitting"; exit 1; }
IFS=, read -r _ _ f2p _ _ _ _ _ _ _ err _ <<<"$(cat "$CSV2")"
[ "$err" = "1" ] && [ "$f2p" = "0" ] || { echo "FAIL: error path"; exit 1; }
echo "PASS (error path)"

# real-result.json was captured from an actual `claude -p --output-format json` run;
# only session_id and uuid were zeroed before committing.
cat > "$TMP/real-harness.sh" <<EOF
#!/usr/bin/env bash
cat "$SPIKE/fixtures/real-result.json"
EOF
chmod +x "$TMP/real-harness.sh"
CSV3="$TMP/out3.csv"
HARNESS="$TMP/real-harness.sh" bash "$SPIKE/run-sitting.sh" "$EXAM" 3 "$CSV3"
row3=$(cat "$CSV3")
echo "row: $row3"
IFS=, read -r _ _ _ _ cost3 turns3 _ model3 denials3 _ err3 _ <<<"$row3"
[ "$cost3" = "0.0510675" ]          || { echo "FAIL: real cost not parsed: $cost3"; exit 1; }
[ "$turns3" = "1" ]                 || { echo "FAIL: real num_turns: $turns3"; exit 1; }
[ "$model3" = "claude-opus-4-8" ]   || { echo "FAIL: real canonical model: $model3"; exit 1; }
[ "$denials3" = "0" ]               || { echo "FAIL: real permission_denials: $denials3"; exit 1; }
[ "$err3" = "0" ]                   || { echo "FAIL: real run marked error"; exit 1; }
echo "PASS (real payload)"

# The two columns S1 exists to measure. A stub pytest stands in for `uv run pytest`
# via the PYTEST seam, so the exit-code verdict is proven offline.
cat > "$TMP/pytest-stub.sh" <<'EOF'
#!/usr/bin/env bash
exit "${PYTEST_RC:-0}"
EOF
chmod +x "$TMP/pytest-stub.sh"
EXAMP="$TMP/exam-pytest"; cp -R "$EXAM" "$EXAMP"
printf 'tests/test_stub.py::test_one\n' > "$EXAMP/f2p.txt"

# $1 = stub pytest exit code, $2 = expected f2p_pass, $3 = expected is_error,
# $4 = COLLECT_ERROR_IS_FAIL (default 0)
check_rc() {
  local policy="${4:-0}" csv="$TMP/pytest-$1-${4:-0}.csv"
  PYTEST_RC="$1" PYTEST="$TMP/pytest-stub.sh" HARNESS="$SPIKE/fixtures/stub-harness.sh" \
    COLLECT_ERROR_IS_FAIL="$policy" \
    bash "$SPIKE/run-sitting.sh" "$EXAMP" "9$1$policy" "$csv"
  echo "row (pytest rc=$1, collect_error_is_fail=$policy): $(cat "$csv")"
  IFS=, read -r _ _ f2p _ _ _ _ _ _ _ err _ <<<"$(cat "$csv")"
  [ "$f2p" = "$2" ] && [ "$err" = "$3" ] \
    || { echo "FAIL: pytest rc=$1 policy=$policy gave f2p=$f2p err=$err, want f2p=$2 err=$3"; exit 1; }
}
check_rc 0 1 0   # clean pass
check_rc 1 0 0   # genuine failed assertion
check_rc 4 0 1   # measured: pytest exits 4 for a node id or file that no longer exists
check_rc 2 0 1   # measured: pytest exits 2 when a directory hits a collection error
check_rc 3 0 1   # internal error stays ERROR under either policy
echo "PASS (pytest exit codes)"

# With the exam supplying its own tests, a collection error is the replay's source, not
# our plumbing — so 2 and 4 become FAIL. 3 and every other code must stay ERROR, or the
# narrowing would have swallowed the invariant it was carved out of.
check_rc 4 0 0 1
check_rc 2 0 0 1
check_rc 3 0 1 1
check_rc 1 0 0 1
check_rc 0 1 0 1
echo "PASS (COLLECT_ERROR_IS_FAIL narrows only the collection codes)"

# A declared node file that is missing is a script fault, not a passing set.
EXAMM="$TMP/exam-missing"; cp -R "$EXAM" "$EXAMM"; rm "$EXAMM/f2p.txt"
CSV4="$TMP/out4.csv"
HARNESS="$SPIKE/fixtures/stub-harness.sh" bash "$SPIKE/run-sitting.sh" "$EXAMM" 4 "$CSV4"
echo "row: $(cat "$CSV4")"
IFS=, read -r _ _ f2p _ _ _ _ _ _ _ err _ <<<"$(cat "$CSV4")"
[ "$err" = "1" ] && [ "$f2p" = "0" ] \
  || { echo "FAIL: missing f2p.txt scored f2p=$f2p err=$err"; exit 1; }
echo "PASS (missing node file)"

# Every node id is evaluated, including a last line with no trailing newline.
cat > "$TMP/pytest-nid.sh" <<'EOF'
#!/usr/bin/env bash
# Fails only the node id in $FAIL_NID, which run_set passes as the last argument.
[ "${*: -1}" = "${FAIL_NID:-}" ] && exit 1
exit 0
EOF
chmod +x "$TMP/pytest-nid.sh"
EXAMN="$TMP/exam-nonewline"; cp -R "$EXAM" "$EXAMN"
printf 'tests/t.py::a\ntests/t.py::b' > "$EXAMN/f2p.txt"   # deliberately no final newline
CSV5="$TMP/out5.csv"
FAIL_NID='tests/t.py::b' PYTEST="$TMP/pytest-nid.sh" \
  HARNESS="$SPIKE/fixtures/stub-harness.sh" \
  bash "$SPIKE/run-sitting.sh" "$EXAMN" 5 "$CSV5"
echo "row: $(cat "$CSV5")"
IFS=, read -r _ _ f2p _ _ _ _ _ _ _ err _ <<<"$(cat "$CSV5")"
[ "$f2p" = "0" ] && [ "$err" = "0" ] \
  || { echo "FAIL: last node id skipped — f2p=$f2p err=$err"; exit 1; }
echo "PASS (unterminated node file)"

# The anti-gaming property: a harness that plants its own passing test under tests/ must
# not get to keep it. tests.patch is applied after the harness runs, once the agent's
# tests/ edits are discarded, so F2P reflects the exam's real test — not the agent's.
EXAMG="$TMP/exam-gaming"; cp -R "$EXAM" "$EXAMG"
printf 'tests/test_gate.py::test_gate\n' > "$EXAMG/f2p.txt"

# tests.patch: the exam's own test, built via a throwaway repo so `git apply` sees an
# ordinary new-file diff, independent of the stub repo run-sitting.sh operates on.
PATCHSRC="$TMP/patchsrc"; git init -q "$PATCHSRC"
mkdir -p "$PATCHSRC/tests"
printf 'SENTINEL_EXAM\n' > "$PATCHSRC/tests/test_gate.py"
git -C "$PATCHSRC" add tests/test_gate.py
git -C "$PATCHSRC" diff --cached > "$EXAMG/tests.patch"

cat > "$TMP/bogus-harness.sh" <<'EOF'
#!/usr/bin/env bash
# Plants a test that would exit 0 (pass) if it survived to evaluation — the gaming
# attempt this case exists to defeat.
set -euo pipefail
mkdir -p tests
printf 'SENTINEL_BOGUS\n' > tests/test_gate.py
cat <<'JSON'
{"is_error":false,"num_turns":1,"stop_reason":"end_turn","total_cost_usd":0.01,
 "permission_denials":[],
 "modelUsage":{"claude-opus-4-8":{"canonicalModel":"claude-opus-4-8","provider":"firstParty"}},
 "result":"done","subtype":"success","type":"result"}
JSON
EOF
chmod +x "$TMP/bogus-harness.sh"

cat > "$TMP/pytest-gate.sh" <<'EOF'
#!/usr/bin/env bash
# Node id is the last argument, same convention as pytest-nid.sh above. Unlike the other
# stubs, this one actually inspects file content, so the test can tell which version of
# tests/test_gate.py survived: the harness's bogus one, or the exam's own tests.patch.
nid="${*: -1}"; file="${nid%%::*}"
[ -f "$file" ] || exit 4
grep -q SENTINEL_BOGUS "$file" && exit 0   # bogus test present: the gaming outcome
grep -q SENTINEL_EXAM "$file" && exit 1    # exam's real test: correctly still fails
exit 4
EOF
chmod +x "$TMP/pytest-gate.sh"

CSVG="$TMP/out-gaming.csv"
HARNESS="$TMP/bogus-harness.sh" PYTEST="$TMP/pytest-gate.sh" \
  bash "$SPIKE/run-sitting.sh" "$EXAMG" 6 "$CSVG"
echo "row: $(cat "$CSVG")"
IFS=, read -r _ _ f2p _ _ _ _ _ _ _ err _ <<<"$(cat "$CSVG")"
[ "$f2p" = "0" ] && [ "$err" = "0" ] \
  || { echo "FAIL: bogus harness test gamed F2P — f2p=$f2p err=$err"; exit 1; }
echo "PASS (tests.patch beats a gamed test)"

# --max-budget-usd bounds spend, not wall clock: a harness that hangs without spending
# never trips it. TIMEOUT_S is the backstop; a short one here keeps the suite fast. The
# stub harness spawns its own subprocess (mirrors the real CLI) so the timeout path is
# proven to kill the whole process group, not just the harness's own PID.
EXAMTO="$TMP/exam-timeout"; cp -R "$EXAM" "$EXAMTO"
echo "TIMEOUT_S=2" >> "$EXAMTO/pin.env"

CHILD_PID_FILE="$TMP/slow-child.pid"
cat > "$TMP/slow-harness.sh" <<'EOF'
#!/usr/bin/env bash
sleep 20 &
echo $! > "$CHILD_PID_FILE"
wait
EOF
chmod +x "$TMP/slow-harness.sh"

CSVTO="$TMP/out-timeout.csv"
CHILD_PID_FILE="$CHILD_PID_FILE" HARNESS="$TMP/slow-harness.sh" \
  bash "$SPIKE/run-sitting.sh" "$EXAMTO" 7 "$CSVTO"
echo "row: $(cat "$CSVTO")"
IFS=, read -r _ _ f2p _ _ _ _ _ _ _ err stop <<<"$(cat "$CSVTO")"
[ "$f2p" = "0" ] && [ "$err" = "1" ] && [ "$stop" = "timeout" ] \
  || { echo "FAIL: timeout not distinguishable — f2p=$f2p err=$err stop=$stop"; exit 1; }

# Not an assumption: confirm the harness's own subprocess is actually gone, not just
# the harness's own PID.
child_pid=$(cat "$CHILD_PID_FILE" 2>/dev/null || echo "")
[ -n "$child_pid" ] || { echo "FAIL: stub never recorded its child pid"; exit 1; }
if kill -0 "$child_pid" 2>/dev/null; then
  echo "FAIL: timeout left a subprocess (pid $child_pid) running"; exit 1
fi
echo "PASS (timeout kills harness and its subprocess, is_error not a failed assertion)"
