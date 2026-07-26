#!/usr/bin/env bash
# The control arm: the same exam, replayed against a repository whose history has been
# stripped back to the base commit.
#
# Why it exists. The main arm pins a worktree of the real tollgate repository, and the
# golden solution (d84d3ad) is a descendant of the base commit on main. A git worktree
# shares the object database, so from inside a replay `git show d84d3ad` returns the
# answer in full, and Bash is unrestricted. A clean sweep in the main arm is therefore
# not by itself evidence that the instruction carried the work — the replay may have
# read the commit the exam was distilled from.
#
# This arm removes that possibility and changes nothing else: same instruction, same
# tests, same assertions, same conditions, same runner. The tree at the base commit is
# exported and re-committed as the single commit of a fresh repository, so there is no
# main, no branch, no remote, and no descendant to read.
#
# This is not only a spike artefact. A user's exam pins a commit in a repository whose
# history contains the session it was blessed from, so the same reachability exists in
# the product. That is recorded in results/summary.md as a design consequence.
set -euo pipefail
SPIKE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
RUNNER="$SPIKE/../s1/run-sitting.sh"
EXAM="$SPIKE/exam"
CSV="$SPIKE/results/control.csv"; mkdir -p "$(dirname "$CSV")"
N=${N:-10}

CLEAN=${CLEAN:-$(mktemp -d)/tollgate-base}
SRC=$(. "$EXAM/pin.env" >/dev/null 2>&1 && echo "$REPO")
SHA=$(. "$EXAM/pin.env" >/dev/null 2>&1 && echo "$BASE_SHA")

if [ ! -d "$CLEAN/.git" ]; then
  mkdir -p "$CLEAN"
  # `git archive` emits exactly the tracked tree at that commit and nothing else — no
  # objects, no refs, no reflog. Same starting content a worktree would have.
  git -C "$SRC" archive "$SHA" | tar -x -C "$CLEAN"
  git -C "$CLEAN" init -q
  git -C "$CLEAN" add -A
  git -C "$CLEAN" -c user.email=spike@local -c user.name=spike \
    commit -q -m "tollgate tree at ${SHA:0:7}, history removed"
fi
CLEAN_SHA=$(git -C "$CLEAN" rev-parse HEAD)

# Fail loudly rather than silently measuring the wrong thing: if the solution is still
# reachable, this arm proves nothing.
if git -C "$CLEAN" cat-file -e "$SHA^{commit}" 2>/dev/null; then
  echo "control repo still reaches the original history — aborting" >&2; exit 1
fi
echo "control repo: $CLEAN @ ${CLEAN_SHA:0:7} ($(git -C "$CLEAN" rev-list --count HEAD) commit)"

[ -f "$CSV" ] || echo "exam,n,f2p_pass,p2p_pass,cost_usd,num_turns,wall_ms,canonical_model,permission_denials,touched_files,is_error,stop_reason" > "$CSV"

for n in $(seq 1 "$N"); do
  if grep -q "^s3ctl,$n," "$CSV"; then echo "skip s3ctl/$n"; continue; fi
  echo "── s3ctl replay $n/$N  ($(date +%H:%M:%S))"
  S3_EXAM_ID=s3ctl TOLLGATE_REPO="$CLEAN" S3_BASE_SHA="$CLEAN_SHA" \
    bash "$RUNNER" "$EXAM" "$n" "$CSV"
  tail -1 "$CSV"
done
