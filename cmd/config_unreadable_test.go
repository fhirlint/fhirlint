// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"errors"
	"strings"
	"testing"
)

// A config that exists but can't be read stops the command, instead of the
// run going on with the defaults (#646).
func TestFakeUnreadableConfigStopsTheRun(t *testing.T) {
	for _, tc := range []struct{ name, yaml string }{
		{"YAML error", "fail-on: warning\nsuppress: [unclosed\n"},
		{"a list at the root", "- fail-on: warning\n"},
		{"a single value", "fail-on warning\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "w.json", warnPatient)
			writeFile(t, dir, "fhirlint.yml", tc.yaml)
			prepareFake(t)
			for _, args := range [][]string{
				{"validate", "w.json"},
				{"validate", "--config", "fhirlint.yml", "w.json"},
				{"stats", "w.json"},
			} {
				r := runFakeKeepEnv(t, dir, args...)
				var ee *exitErr
				if !errors.As(r.err, &ee) || ee.code != 2 || !strings.Contains(r.err.Error(), "fhirlint.yml cannot be read") || !strings.Contains(r.err.Error(), "fhirlint config check") {
					t.Errorf("%q: want exit 2 naming the file and config check, got %v", args, r.err)
				}
			}

			// config check reports it; version still works.
			r := runFakeKeepEnv(t, dir, "config", "check")
			if r.err == nil || !strings.Contains(r.stdout+r.stderr, "line ") {
				t.Errorf("config check must report the problem: %v\n%s%s", r.err, r.stdout, r.stderr)
			}
			if r := runFakeKeepEnv(t, dir, "version"); r.err != nil {
				t.Errorf("version must not need the config: %v", r.err)
			}
		})
	}
}

// No config at all is fine, and so is an empty one.
func TestFakeMissingOrEmptyConfigIsFine(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "ok.json", cleanPatient)
	mustPass(t, runFake(t, dir, "validate", "ok.json"))
	writeFile(t, dir, "fhirlint.yml", "")
	mustPass(t, runFakeKeepEnv(t, dir, "validate", "ok.json"))
}

// A file named fhirlint is the binary, not a config (#653): a directory
// holding it runs as if it weren't there.
func TestFakeBinaryNamedFhirlintIsNotAConfig(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "w.json", warnPatient)
	writeFile(t, dir, "fhirlint", "\x7fELF\x02\x01\x01\x00 not yaml")
	r := runFake(t, dir, "validate", "--fail-on", "warning", "w.json")
	if !errors.Is(r.err, errValidationFailed) {
		t.Errorf("the run must go ahead and fail on the warning, got %v", r.err)
	}
	if r := runFakeKeepEnv(t, dir, "config", "check"); r.err == nil || !strings.Contains(r.stderr+r.stdout, "No fhirlint.yml found") {
		t.Errorf("config check must not take the binary for a config: %v\n%s%s", r.err, r.stdout, r.stderr)
	}
}
