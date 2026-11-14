#!/usr/bin/env bash
# Weekly duration watch (#551), run by .github/workflows/duration-watch.yml.
#
#   duration-watch.sh REPO BUDGETS DAYS
#
# For each workflow in BUDGETS (file, minutes), reads its successful runs of
# the last DAYS days, one workflow at a time, and prints a Markdown table of
# median and 90th percentile wall time against the budget. Exits 1 when any
# workflow's p90 is over its budget, 0 otherwise. Needs gh and jq.
set -euo pipefail

repo=$1
budgets=$2
days=$3
since=$(date -u -d "$days days ago" +%Y-%m-%d)

echo "| Workflow | Runs | Median | p90 | Budget | |"
echo "|---|---|---|---|---|---|"
over=0
while read -r file minutes _; do
  case "$file" in ''|'#'*) continue ;; esac
  # One query per workflow: a single search over all of them would let the
  # busy ones crowd the quiet ones out of the result limit.
  durations=$(gh run list --repo "$repo" --workflow "$file" --status success \
    --created ">=$since" --limit 200 --json startedAt,updatedAt \
    --jq '.[] | ((.updatedAt | fromdate) - (.startedAt | fromdate))' | sort -n)
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
    over=1
  fi
  # shellcheck disable=SC2016 # the backticks are Markdown, not a command
  printf '| `%s` | %d | %dm%02ds | %dm%02ds | %sm | %s |\n' "$file" "$n" \
    $((median / 60)) $((median % 60)) $((p90 / 60)) $((p90 % 60)) "$minutes" "$status"
done < "$budgets"
exit "$over"
