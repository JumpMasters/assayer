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
