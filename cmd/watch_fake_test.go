// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"path/filepath"
	"strings"
	"testing"
)

// The watch mode (#665) is the validator's own -watch-mode: fhirlint starts
// it and waits until the JVM ends. The fake validator validates once and
// exits, so a test gets control back after one round.

func TestFakeWatch_RunsTheValidatorsWatchMode(t *testing.T) {
	prepareFake(t)
	dir := t.TempDir()
	writeFile(t, dir, "fhir/ok.json", cleanPatient)
	writeFile(t, dir, "fhir/bad.json", errPatient)
	writeFile(t, dir, "fhir/notes.txt", "not FHIR")
	log := filepath.Join(t.TempDir(), "calls.jsonl")
	t.Setenv(fakeJavaLogEnv, log)

	r := runFakeKeepEnv(t, dir, "validate", "--watch=all", "--watch-interval", "500", "--watch-settle-time", "750", "fhir")
	if r.err != nil {
		t.Fatalf("watch: %v\n%s", r.err, r.stderr)
	}
	if !strings.Contains(r.stderr, "Watching 2 file(s) for changes (mode: all)") {
		t.Errorf("fhirlint must say what it watches:\n%s", r.stderr)
	}
	if !strings.Contains(r.stdout, "mode all, scan delay 500, settle time 750") || !strings.Contains(r.stdout, "bad.json: 1 error(s)") {
		t.Errorf("the validator's watch output goes to stdout:\n%s", r.stdout)
	}
	calls := fakeJavaCalls(t, log)
	if got := argAfter(calls[len(calls)-1], "-watch-mode"); len(got) == 0 || got[0] != "all" {
		t.Errorf("-watch-mode all: %v", calls[len(calls)-1])
	}

	// --watch alone is single mode, with the validator's own delays.
	r = runFakeKeepEnv(t, dir, "validate", "--watch", "fhir/ok.json")
	if r.err != nil || !strings.Contains(r.stdout, "mode single, scan delay default, settle time default") {
		t.Errorf("--watch alone: %v\n%s", r.err, r.stdout)
	}
}

// The three keys work from fhirlint.yml too.
func TestFakeWatch_FromConfig(t *testing.T) {
	prepareFake(t)
	dir := t.TempDir()
	writeFile(t, dir, "p.json", cleanPatient)
	writeFile(t, dir, "fhirlint.yml", "watch: all\nwatch-interval: 300\nwatch-settle-time: 400\n")
	r := runFakeKeepEnv(t, dir, "validate", "p.json")
	if r.err != nil || !strings.Contains(r.stdout, "mode all, scan delay 300, settle time 400") {
		t.Errorf("watch keys from the config: %v\n%s", r.err, r.stdout)
	}
}

// The validator's watch ending with an error is the run's error.
func TestFakeWatch_ValidatorExitIsAnError(t *testing.T) {
	prepareFake(t)
	dir := t.TempDir()
	writeFile(t, dir, "p.json", cleanPatient)
	t.Setenv("FAKEJAVA_WATCH_EXIT", "1")
	if r := runFakeKeepEnv(t, dir, "validate", "--watch", "p.json"); r.err == nil {
		t.Error("a watch that dies must not end as a success")
	}
}

func TestFakeWatch_UsageErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "p.json", cleanPatient)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--watch", "--url", "https://example.org/p"}, "--watch is not compatible with --url"},
		{[]string{"--watch", "--format", "json", "--output", "r.json", "p.json"}, "--watch is not compatible with --format json --output"},
		{[]string{"--watch", "--since", "HEAD", "p.json"}, "--since is not compatible with --watch"},
		{[]string{"--watch", "missing.json"}, "missing.json"},
	} {
		r := runFake(t, dir, append([]string{"validate"}, tc.args...)...)
		if r.err == nil || !strings.Contains(r.err.Error(), tc.want) {
			t.Errorf("%q: want %q, got %v", tc.args, tc.want, r.err)
		}
	}
}
