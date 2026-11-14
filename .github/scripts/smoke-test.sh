#!/usr/bin/env bash
# Copyright The fhirlint Authors
# SPDX-License-Identifier: Apache-2.0
# Smoke test of a release archive (#549): the unpacked binary, run the way
# users run it, before the release is published.
#
#   smoke-test.sh TAG BINARY [validate]
#
# Checks that `fhirlint version` reports TAG and that two commands that need no
# validator JAR work: `explain` with a built-in explanation and `config check`
# on the example config. With `validate` (Linux, JAR cache restored), also
# validates a fixture with a known error and expects exit 1 and that error.
# Run it from the repository root.
set -euo pipefail

want=$1
bin=$2
mode=${3:-}

version=$("$bin" version)
echo "$version"
first=$(head -n 1 <<<"$version")
if [[ ! "$first" =~ ^fhirlint:[[:space:]]+"$want"$ ]]; then
  echo "::error::expected fhirlint version $want, got: $first"
  exit 1
fi

if ! out=$("$bin" explain dom-6); then
  echo "$out"
  echo "::error::fhirlint explain dom-6 failed"
  exit 1
fi
if ! grep -q '^dom-6 ' <<<"$out"; then
  echo "$out"
  echo "::error::fhirlint explain dom-6 did not explain dom-6"
  exit 1
fi

"$bin" config check --config fhirlint.yml.example

if [ "$mode" = validate ]; then
  code=0
  report=$("$bin" validate --no-terminology-server --validator-version "${FHIRLINT_VALIDATOR_VERSION:?}" \
    --format json testdata/invalid/bad-gender.json) || code=$?
  if [ "$code" != 1 ]; then
    echo "$report"
    echo "::error::validating testdata/invalid/bad-gender.json exited $code, expected 1 (errors)"
    exit 1
  fi
  if ! grep -q '"severity": "error"' <<<"$report"; then
    echo "$report"
    echo "::error::validating testdata/invalid/bad-gender.json reported no error"
    exit 1
  fi
fi
echo "smoke test passed: fhirlint $want${mode:+ ($mode)}"
