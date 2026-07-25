#!/usr/bin/env bash
# Ten sittings of each exam, sequentially. Resumable: skips rows already in the CSV.
set -euo pipefail
SPIKE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
CSV="$SPIKE/results/sittings.csv"; mkdir -p "$(dirname "$CSV")"
N=${N:-10}

[ -f "$CSV" ] || echo "exam,n,f2p_pass,p2p_pass,cost_usd,num_turns,wall_ms,canonical_model,permission_denials,touched_files,is_error,stop_reason" > "$CSV"

for exam in "$SPIKE"/exams/*/; do
  id=$(grep -h '^EXAM_ID=' "$exam/pin.env" | cut -d= -f2)
  for n in $(seq 1 "$N"); do
    if grep -q "^$id,$n," "$CSV"; then echo "skip $id/$n"; continue; fi
    echo "── $id sitting $n/$N"
    bash "$SPIKE/run-sitting.sh" "$exam" "$n" "$CSV"
    tail -1 "$CSV"
  done
done
