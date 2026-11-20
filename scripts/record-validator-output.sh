#!/usr/bin/env bash
# Copyright The fhirlint Authors
# SPDX-License-Identifier: Apache-2.0
#
# Records the HL7 validator's real output for the parser's unit tests (#715):
#
#   scripts/record-validator-output.sh 7.0.6
#
# It runs the JAR of that release over every case in
# internal/validator/recorded_output_cases_test.go, writes what the JAR left
# behind into internal/validator/testdata/validator-output/<version>/, and then
# rewrites each case's expected.json from fhirlint's parser. Review the diff:
# a changed expected.json is a change in what fhirlint reports for that
# validator.
#
# Needs Java 17+ and the network (the JAR, the packages and, for one case, the
# live terminology servers). A manual step, not a CI job. The JAR goes into a
# cache of its own per version, so the one in ~/.fhirlint is left alone.
set -euo pipefail

version=${1:-}
if [[ ! $version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "usage: $0 <validator version, e.g. 7.0.6>" >&2
  exit 2
fi

cd "$(git rev-parse --show-toplevel)"

export FHIRLINT_CACHE_DIR="${FHIRLINT_RECORD_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/fhirlint-record/$version}"
export FHIRLINT_VALIDATOR_VERSION="$version"

go test -tags record -run '^TestRecordValidatorOutput$' -count=1 -timeout 60m -v ./internal/validator/
go test -run '^TestRecordedValidatorOutput$' -count=1 ./internal/validator/ -update-recorded
go test -run '^TestRecordedValidatorOutput$' -count=1 ./internal/validator/

git status --short internal/validator/testdata/validator-output/
