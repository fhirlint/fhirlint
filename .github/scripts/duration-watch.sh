#!/usr/bin/env bash
# Copyright The fhirlint Authors
# SPDX-License-Identifier: Apache-2.0
# Weekly duration watch (#551), run by .github/workflows/duration-watch.yml.
#
#   duration-watch.sh REPO BUDGETS DAYS
#
# For each workflow in BUDGETS (file, minutes), reads its successful runs of
# the last DAYS days, one workflow at a time, and prints a Markdown table of
# their durations against the budget. Needs gh and jq.
#
# The budget is on the run's longest job, from its start to its end (#721).
# That is what timeout-minutes limits, and what grows when a suite grows, and
# no queue is in it. A burst of pull requests fills the runner pool and
# multiplies the wall time without any workflow getting slower: on 2026-11-16
# Sensitive paths waited 717 s and ran 5 s. Measuring from the first job's
# start to the last one's end is not enough either: the jobs of one run then
# queue one after the other, and a lint run whose longest job took 104 s
# spanned 496 s. The table still shows the wall time, so a queue that stays
# long is seen.
#
# The longest job needs a query per run, and a week has about a thousand runs,
# the GITHUB_TOKEN's hourly limit. But no job is longer than its run's wall
# time, so only a run whose wall time is over the budget can be over it: those
# runs are measured, every other run counts with its wall time. Over the
# budget the p90 is exact; within it, it is an upper bound, which is all the
# verdict needs.
#
# Exit codes:
#   0  every workflow within budget
#   3  at least one workflow's p90 longest job over its budget
#   anything else: the measurement failed, the table is incomplete (#736).
#      Under set -e that is the failing command's own code, and gh's is 1,
#      which is why "over budget" is not 1.
set -euo pipefail

over_budget=3

repo=$1
budgets=$2
days=$3
since=$(date -u -d "$days days ago" +%Y-%m-%d)

# retry runs a gh query and prints its output. The API answers a weekly burst
# of queries with the odd 502, so a failed query is retried; the last failure
# ends the script with gh's code.
retry() {
  local attempt out
  for attempt in 1 2 3; do
    if out=$("$@"); then
      printf '%s' "$out"
      return 0
    fi
    if [ "$attempt" -lt 3 ]; then
      echo "duration-watch: $* failed, retrying" >&2
      sleep "${DURATION_WATCH_RETRY_DELAY:-10}"
    fi
  done
  echo "duration-watch: $* failed, giving up" >&2
  return 1
}

# runs prints "id wall-seconds" for each successful run of a workflow.
runs() {
  retry gh run list --repo "$repo" --workflow "$1" --status success \
    --created ">=$since" --limit 200 --json databaseId,startedAt,updatedAt \
    --jq '.[] | "\(.databaseId) \((.updatedAt | fromdate) - (.startedAt | fromdate))"'
}

# longest_job prints the seconds the longest job of a run took, from its start
# on a runner to its end.
longest_job() {
  retry gh api "repos/$repo/actions/runs/$1/jobs?per_page=100" \
    --jq '[.jobs[] | select(.started_at != null and .completed_at != null)
      | (.completed_at | fromdate) - (.started_at | fromdate)] | max // 0'
}

# percentiles prints "count median p90" of the numbers on stdin.
percentiles() {
  sort -n | awk '{ a[NR] = $1 } END {
    p = int(NR * 0.9); if (p < NR * 0.9) p++
    printf "%d %d %d\n", NR, a[int((NR + 1) / 2)], a[p] }'
}

mmss() { printf '%dm%02ds' $(($1 / 60)) $(($1 % 60)); }

echo "| Workflow | Runs | Median wall | p90 wall | p90 longest job | Budget | |"
echo "|---|---|---|---|---|---|---|"
over=0
while read -r file minutes _; do
  case "$file" in ''|'#'*) continue ;; esac
  budget=$((minutes * 60))
  # One query per workflow: a single search over all of them would let the
  # busy ones crowd the quiet ones out of the result limit.
  list=$(runs "$file")
  if [ -z "$list" ]; then
    echo "| \`$file\` | 0 | | | | ${minutes}m | no runs |"
    continue
  fi
  read -r n median p90 < <(cut -d' ' -f2 <<<"$list" | percentiles)
  # The longest job only where the wall time leaves the budget in doubt.
  times=""
  while read -r id wall; do
    if [ "$wall" -gt "$budget" ]; then
      wall=$(longest_job "$id")
    fi
    times+="$wall"$'\n'
  done <<<"$list"
  read -r _ _ p90run < <(printf '%s' "$times" | percentiles)
  status="ok"
  run="≤ $(mmss "$p90run")" # within the budget, an upper bound
  if [ "$p90run" -gt "$budget" ]; then
    status="**over budget**"
    over=$over_budget
    run=$(mmss "$p90run")
  fi
  # shellcheck disable=SC2016 # the backticks are Markdown, not a command
  printf '| `%s` | %d | %s | %s | %s | %sm | %s |\n' "$file" "$n" \
    "$(mmss "$median")" "$(mmss "$p90")" "$run" "$minutes" "$status"
done < "$budgets"
exit "$over"
