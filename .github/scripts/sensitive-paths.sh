#!/usr/bin/env bash
# Helpers for .github/workflows/sensitive-paths.yml. The workflow runs this
# file from main (pull_request_target), never from the pull request. The list
# of sensitive paths lives here, in filter(), and nowhere else.
#
#   sensitive-paths.sh filter
#       Reads file names on stdin and prints the sensitive ones, sorted.
#   sensitive-paths.sh fingerprint < compare.json
#       Reads a compare API response and prints one line per sensitive file:
#       name, old name of a rename, and the SHA-256 of its patch. Prints
#       UNKNOWN when the response cannot be trusted to be complete: a
#       sensitive file without a patch, or the API's 300-file limit.
#   sensitive-paths.sh unchanged <repo> <base> <before> <after>
#       Exits 0 when the pull request's diff on the sensitive paths is the
#       same at <before> and <after> (#541), 1 otherwise or when unsure.
set -euo pipefail
export LC_ALL=C # byte order for sort, whatever the runner locale

# Groups:
#   plugin          hooks/, skills/, .claude-plugin/ — installed from main
#   pipelines       action.yml, templates/, .gitlab-ci.yml,
#                   .pre-commit-hooks.yaml — run in users' CI and hooks
#   build, release  .github/workflows/, .github/scripts/, .goreleaser.yml,
#                   Dockerfile
#   verification    the pinned HL7 signing key and the code checking it
#   agent config    CLAUDE.md and friends, .claude/, .mcp.json, ...
#   other checks    files that can quietly weaken the protections above
filter() {
  grep -E \
    -e '^(hooks|skills|\.claude-plugin|templates)/' \
    -e '^(action\.ya?ml|\.gitlab-ci\.yml|\.pre-commit-hooks\.yaml)$' \
    -e '^\.github/(workflows|actions|instructions|agents|skills|scripts)/' \
    -e '^(\.goreleaser\.ya?ml|Dockerfile)$' \
    -e '^internal/validator/(hl7_signing_key\.asc|signature\.go)$' \
    -e '(^|/)(CLAUDE|CLAUDE\.local|AGENTS|AGENT|GEMINI)\.md$' \
    -e '^(\.claude|\.agents|\.cursor|\.vscode|\.devcontainer)/' \
    -e '^(\.mcp\.json|\.envrc|\.cursorrules|\.claudeignore|\.github/copilot-instructions\.md)$' \
    -e '^(\.markdownlint-cli2\.jsonc|lychee\.toml)$' \
    -e '^(\.gitignore|\.golangci\.yml|\.github/dependabot\.yml|\.github/actionlint\.ya?ml|(\.github/|docs/)?CODEOWNERS)$' \
    | sort -u || true
}

fingerprint() {
  local json
  json=$(cat)
  if [ "$(jq '.files | length' <<<"$json")" -ge 300 ]; then
    echo UNKNOWN
    return
  fi
  # A file is sensitive when its name or the old name of a rename is.
  local names
  names=$(jq -r '.files[] | .filename, (.previous_filename // empty)' <<<"$json" | filter)
  [ -z "$names" ] && return
  jq -c '.files[]' <<<"$json" | while IFS= read -r f; do
    name=$(jq -r .filename <<<"$f")
    prev=$(jq -r '.previous_filename // ""' <<<"$f")
    if ! grep -qxF -e "$name" <<<"$names" && { [ -z "$prev" ] || ! grep -qxF -e "$prev" <<<"$names"; }; then
      continue
    fi
    if [ "$(jq 'has("patch")' <<<"$f")" != true ]; then
      echo UNKNOWN
      continue
    fi
    sum=$(jq -j .patch <<<"$f" | sha256sum | cut -d' ' -f1)
    printf '%s\t%s\t%s\n' "$name" "$prev" "$sum"
  done | sort -u
}

unchanged() {
  local repo=$1 base=$2 before=$3 after=$4 a b
  a=$(gh api "repos/$repo/compare/$base...$before" | fingerprint) || return 1
  b=$(gh api "repos/$repo/compare/$base...$after" | fingerprint) || return 1
  if printf '%s\n%s\n' "$a" "$b" | grep -qx UNKNOWN; then
    echo "a sensitive file has no patch or the diff is too large to compare"
    return 1
  fi
  if [ "$a" != "$b" ]; then
    echo "the pull request's changes to sensitive paths differ from the reviewed state"
    return 1
  fi
  echo "the pull request's changes to sensitive paths are the same as when they were reviewed"
}

cmd=${1:-}
shift || true
case "$cmd" in
  filter) filter ;;
  fingerprint) fingerprint ;;
  unchanged) unchanged "$@" ;;
  *) echo "usage: sensitive-paths.sh filter | fingerprint | unchanged <repo> <base> <before> <after>" >&2; exit 2 ;;
esac
