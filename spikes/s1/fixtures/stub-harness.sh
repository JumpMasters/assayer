#!/usr/bin/env bash
# Stands in for `claude -p --output-format json` so the runner can be tested
# without spending a sitting. Writes the file the stub exam's f2p test checks.
set -euo pipefail
printf 'stub wrote this\n' > stub_output.txt
cat <<'JSON'
{"is_error":false,"num_turns":3,"stop_reason":"end_turn","total_cost_usd":0.42,
 "permission_denials":[],
 "modelUsage":{"claude-opus-4-8":{"canonicalModel":"claude-opus-4-8","provider":"firstParty"}},
 "result":"done","subtype":"success","type":"result"}
JSON
