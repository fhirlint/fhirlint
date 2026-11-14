#!/usr/bin/env bash
# Copyright The fhirlint Authors
# SPDX-License-Identifier: Apache-2.0
# rule-diff.sh [-n N] [A_REF] [B_REF] — diff the findings of fhirlint's own
# rules (built-in lint rules and the reference check) between two builds over
# the real-world corpus (#577).
#
#   A_REF  the old build; default: the latest release tag
#   B_REF  the new build; default: the working tree, uncommitted changes included
#   -n N   show up to N added and N removed findings per rule (default 12)
#
# The corpus comes from scripts/corpus-fetch.sh ($FHIRLINT_CORPUS, else
# ~/.cache/fhirlint-corpus). Both builds are scanned with the same copy of the
# scan tool, internal/tools/rulescan from B, so only the rules differ. Each scan
# runs from an empty directory; the result cache is not involved, since the
# rules run in-process and never touch it.
#
# Afterwards B is checked against scripts/corpus/must-stay-reported.txt: a
# missing entry fails the script.
set -euo pipefail

n=12
if [ "${1:-}" = -n ]; then n=$2; shift 2; fi
repo=$(git rev-parse --show-toplevel)
a_ref=${1:-$(git -C "$repo" describe --tags --abbrev=0)}
b_ref=${2:-}
corpus=${FHIRLINT_CORPUS:-$HOME/.cache/fhirlint-corpus}/data
if [ ! -d "$corpus" ]; then
  echo "rule-diff: no corpus at $corpus — run scripts/corpus-fetch.sh first" >&2
  exit 2
fi

work=$(mktemp -d)
cleanup() {
  git -C "$repo" worktree remove --force "$work/a" 2>/dev/null || true
  if [ -n "$b_ref" ]; then git -C "$repo" worktree remove --force "$work/b" 2>/dev/null || true; fi
  rm -rf "$work"
}
trap cleanup EXIT

git -C "$repo" worktree add --quiet --detach "$work/a" "$a_ref"
if [ -n "$b_ref" ]; then
  git -C "$repo" worktree add --quiet --detach "$work/b" "$b_ref"
  b_src=$work/b
else
  b_src=$repo
fi

# The scan tool is the measuring device: B's copy goes into A's tree too.
rm -rf "$work/a/internal/tools/rulescan"
cp -R "$b_src/internal/tools/rulescan" "$work/a/internal/tools/rulescan"
rm -f "$work/a/internal/tools/rulescan/"*_test.go

(cd "$work/a" && go build -o "$work/rulescan-a" ./internal/tools/rulescan)
(cd "$b_src" && go build -o "$work/rulescan-b" ./internal/tools/rulescan)

mkdir "$work/empty"
echo "scan A ($a_ref)"
(cd "$work/empty" && "$work/rulescan-a" scan -o "$work/a.jsonl" "$corpus" >/dev/null)
echo "scan B (${b_ref:-working tree})"
(cd "$work/empty" && "$work/rulescan-b" scan -o "$work/b.jsonl" "$corpus" >/dev/null)

out=${RULE_DIFF_KEEP:-}
if [ -n "$out" ]; then
  mkdir -p "$out"
  cp "$work/a.jsonl" "$work/b.jsonl" "$out/"
fi

"$work/rulescan-b" diff -n "$n" -must "$b_src/scripts/corpus/must-stay-reported.txt" "$work/a.jsonl" "$work/b.jsonl"
