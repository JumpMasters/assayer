#!/usr/bin/env bash
#
# Run the test suite with the race detector and enforce a minimum total
# statement coverage across the library packages (internal/...). The command
# entrypoint (cmd/...) is a thin shell over those packages and is excluded from
# the denominator so that trivial wiring cannot inflate the figure.
#
# Usage: scripts/coverage.sh [threshold-percent]   (default: 80)
set -euo pipefail

threshold="${1:-80}"
profile="${COVER_PROFILE:-coverage.out}"

go test -race -covermode=atomic -coverprofile="$profile" ./internal/... >/dev/null

# "total: (statements) 87.5%" -> 87.5
total="$(go tool cover -func="$profile" | awk '/^total:/ {print $NF}' | tr -d '%')"
echo "coverage: ${total}% (threshold ${threshold}%)"

awk -v have="$total" -v want="$threshold" 'BEGIN { exit (have + 0 >= want + 0) ? 0 : 1 }' || {
  echo "error: coverage ${total}% is below the required ${threshold}%" >&2
  exit 1
}
