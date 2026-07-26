#!/usr/bin/env bash
# Ten replays of the distilled exam, sequentially. Resumable: skips rows already in the CSV.
#
# Reuses S1's sitting runner unmodified, so the two spikes are measured by the same
# instrument under the same conditions and their numbers can be read side by side. The
# only S3-specific setting is COLLECT_ERROR_IS_FAIL, which the exam's own pin.env sets
# and ../README.md ("Amendment") justifies.
#
# One consequence of that reuse: the runner derives its own directory for raw harness
# output, so S3's per-replay JSON lands in `s1/results/raw/s3-N.json`, not under `s3/`.
# Both raw directories are gitignored and never leave the machine; only the CSV and the
# written-up results live here.
set -euo pipefail
SPIKE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
RUNNER="$SPIKE/../s1/run-sitting.sh"
EXAM="$SPIKE/exam"
CSV="$SPIKE/results/sittings.csv"; mkdir -p "$(dirname "$CSV")"
N=${N:-10}

[ -r "$RUNNER" ] || { echo "S1 sitting runner not found at $RUNNER" >&2; exit 1; }
[ -r "$EXAM/pin.env" ] || { echo "no exam at $EXAM" >&2; exit 1; }

[ -f "$CSV" ] || echo "exam,n,f2p_pass,p2p_pass,cost_usd,num_turns,wall_ms,canonical_model,permission_denials,touched_files,is_error,stop_reason" > "$CSV"

for n in $(seq 1 "$N"); do
  if grep -q "^s3,$n," "$CSV"; then echo "skip s3/$n"; continue; fi
  echo "── s3 replay $n/$N  ($(date +%H:%M:%S))"
  bash "$RUNNER" "$EXAM" "$n" "$CSV"
  tail -1 "$CSV"
done
