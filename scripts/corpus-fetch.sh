#!/usr/bin/env bash
# corpus-fetch.sh [DIR] — download the real-world FHIR corpus for the
# false-positive scan of the built-in rules (#577) and unpack it into DIR
# (default: $FHIRLINT_CORPUS, else ~/.cache/fhirlint-corpus).
#
# The sources and their sha256 are pinned in scripts/corpus/sources.tsv. A
# download whose hash differs is rejected, not used. The corpus stays out of
# the repository: it is ~1.5 GB unpacked and its licences are the sources' own.
#
# Layout: DIR/data/<name>/... holds the JSON files of one dataset, and
# DIR/data/<name>/.rulescan-base the canonical base for canonical-url-pattern.
# Each dataset is one reference-check set, as a directory run would be.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
dir=${1:-${FHIRLINT_CORPUS:-$HOME/.cache/fhirlint-corpus}}
mkdir -p "$dir/downloads" "$dir/data"

sha256() {
  if command -v sha256sum >/dev/null; then sha256sum "$1" | cut -d' ' -f1
  else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

while IFS=$'\t' read -r name url want base; do
  case $name in ''|'#'*) continue ;; esac
  archive="$dir/downloads/$name"
  if [ ! -f "$archive" ] || [ "$(sha256 "$archive")" != "$want" ]; then
    echo "fetch $name"
    curl -fsSL --retry 3 -o "$archive.part" "$url"
    got=$(sha256 "$archive.part")
    if [ "$got" != "$want" ]; then
      echo "corpus-fetch: $name: sha256 $got, expected $want — refusing it" >&2
      rm -f "$archive.part"
      exit 1
    fi
    mv "$archive.part" "$archive"
  fi
  out="$dir/data/$name"
  if [ -f "$out/.rulescan-sha256" ] && [ "$(cat "$out/.rulescan-sha256")" = "$want" ]; then
    continue
  fi
  echo "unpack $name"
  rm -rf "$out"
  mkdir -p "$out"
  if unzip -tq "$archive" >/dev/null 2>&1; then
    unzip -q -o "$archive" '*.json' -d "$out"
  else
    # FHIR packages: only the JSON resources; package.json and .index.json are
    # package metadata, not resources, and the scan skips them anyway.
    tar -xzf "$archive" -C "$out" --wildcards '*.json'
  fi
  if [ "$base" != "-" ]; then printf '%s\n' "$base" > "$out/.rulescan-base"; fi
  printf '%s\n' "$want" > "$out/.rulescan-sha256"
done < "$here/corpus/sources.tsv"

echo "corpus ready: $dir/data ($(find "$dir/data" -name '*.json' | wc -l | tr -d ' ') JSON files)"
