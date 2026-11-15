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

// The documented .fhirlint.yml is read by a run, not only by config check
// (#654); fhirlint.yml wins when both are there.
func TestFakeDotfileConfigIsRead(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "w.json", warnPatient)
	writeFile(t, dir, ".fhirlint.yml", "fail-on: warning\n")
	prepareFake(t)
	if r := runFakeKeepEnv(t, dir, "validate", "w.json"); !errors.Is(r.err, errValidationFailed) {
		t.Errorf("fail-on from .fhirlint.yml must apply, got %v", r.err)
	}
	if r := runFakeKeepEnv(t, dir, "config", "check"); r.err != nil || !strings.Contains(r.stdout+r.stderr, ".fhirlint.yml") {
		t.Errorf("config check must check the same file: %v\n%s%s", r.err, r.stdout, r.stderr)
	}

	writeFile(t, dir, "fhirlint.yml", "fail-on: never\n")
	if r := runFakeKeepEnv(t, dir, "validate", "w.json"); r.err != nil {
		t.Errorf("fhirlint.yml wins over .fhirlint.yml: %v", r.err)
	}
	if r := runFakeKeepEnv(t, dir, "config", "check"); r.err != nil || !strings.Contains(r.stdout+r.stderr, "/fhirlint.yml") {
		t.Errorf("config check checks the file the run reads: %v\n%s%s", r.err, r.stdout, r.stderr)
	}
}

// config check finds the config a run finds one or two levels up, too.
func TestFakeConfigCheckUsesTheRunsDiscovery(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "fhirlint.yml", "severty: error\n")
	sub := root + "/a/b"
	writeFile(t, sub, "p.json", cleanPatient)
	r := runFake(t, sub, "config", "check")
	if r.err == nil || !strings.Contains(r.stdout+r.stderr, `unknown key "severty"`) {
		t.Errorf("config check must report the parent's config: %v\n%s%s", r.err, r.stdout, r.stderr)
	}
}
