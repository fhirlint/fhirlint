#!/usr/bin/env bash
# Copyright The fhirlint Authors
# SPDX-License-Identifier: Apache-2.0
# Weekly duration watch (#551), run by .github/workflows/duration-watch.yml.
#
#   duration-watch.sh REPO BUDGETS DAYS
#
# For each workflow in BUDGETS (file, minutes), reads its successful runs of
# the last DAYS days, one workflow at a time, and prints a Markdown table of
# median and 90th percentile wall time against the budget. Needs gh and jq.
#
# Exit codes:
#   0  every workflow within budget
#   3  at least one workflow's p90 over its budget
#   anything else: the measurement failed, the table is incomplete (#736).
#      Under set -e that is the failing command's own code, and gh's is 1,
#      which is why "over budget" is not 1.
set -euo pipefail

over_budget=3

repo=$1
budgets=$2
days=$3
since=$(date -u -d "$days days ago" +%Y-%m-%d)

# runs prints the duration in seconds of each successful run of a workflow,
# one per line. The API answers a weekly burst of queries with the odd 502, so
# a failed query is retried; the last failure ends the script with gh's code.
runs() {
  local attempt out
  for attempt in 1 2 3; do
    if out=$(gh run list --repo "$repo" --workflow "$1" --status success \
      --created ">=$since" --limit 200 --json startedAt,updatedAt \
      --jq '.[] | ((.updatedAt | fromdate) - (.startedAt | fromdate))'); then
      printf '%s' "$out"
      return 0
    fi
    if [ "$attempt" -lt 3 ]; then
      echo "duration-watch: gh run list failed for $1, retrying" >&2
      sleep "${DURATION_WATCH_RETRY_DELAY:-10}"
    fi
  done
  echo "duration-watch: gh run list failed for $1, giving up" >&2
  return 1
}

echo "| Workflow | Runs | Median | p90 | Budget | |"
echo "|---|---|---|---|---|---|"
over=0
while read -r file minutes _; do
  case "$file" in ''|'#'*) continue ;; esac
  # One query per workflow: a single search over all of them would let the
  # busy ones crowd the quiet ones out of the result limit.
  durations=$(runs "$file" | sort -n)
  if [ -z "$durations" ]; then
    echo "| \`$file\` | 0 | | | ${minutes}m | no runs |"
    continue
  fi
  read -r n median p90 < <(awk '{ a[NR] = $1 } END {
    p = int(NR * 0.9); if (p < NR * 0.9) p++
    printf "%d %d %d\n", NR, a[int((NR + 1) / 2)], a[p] }' <<<"$durations")
  status="ok"
  if [ "$p90" -gt $((minutes * 60)) ]; then
    status="**over budget**"
    over=$over_budget
  fi
  # shellcheck disable=SC2016 # the backticks are Markdown, not a command
  printf '| `%s` | %d | %dm%02ds | %dm%02ds | %sm | %s |\n' "$file" "$n" \
    $((median / 60)) $((median % 60)) $((p90 / 60)) $((p90 % 60)) "$minutes" "$status"
done < "$budgets"
exit "$over"
