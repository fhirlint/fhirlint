// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The checks validate makes before it starts the validator, each with the
// message a user sees (#667). None of these may reach the JAR: a wrong flag
// is said once, up front, not after minutes of validation.
func TestFakeValidate_StartupChecks(t *testing.T) {
	prepareFake(t)
	dir := t.TempDir()
	writeFile(t, dir, "p.json", cleanPatient)
	writeFile(t, dir, "q.json", cleanPatient)
	log := filepath.Join(t.TempDir(), "calls.jsonl")
	t.Setenv(fakeJavaLogEnv, log)

	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--validation-timeout", "soon", "p.json"}, `invalid --validation-timeout value "soon"`},
		{[]string{"--validation-timeout", "-5s", "p.json"}, "--validation-timeout must be positive"},
		{[]string{"--max-messages", "-1", "p.json"}, "--max-messages must be zero or positive"},
		{[]string{"--slowest", "-1", "p.json"}, "--slowest must be zero or positive"},
		{[]string{"--offline", "--terminology-server", "https://tx.example.org/r4", "p.json"}, "--offline forbids network access"},
		{[]string{"--codesystem-size-limit", "-2", "p.json"}, "--codesystem-size-limit must be -1"},
		{[]string{"--url-timeout", "soon", "p.json"}, `invalid --url-timeout value "soon"`},
		{[]string{"--extract", "$.a", "--extract-each", "$.b", "p.json"}, "mutually exclusive"},
		{[]string{"--url", "https://example.org/p", "p.json", "q.json"}, "cannot combine file arguments with --url"},
		{[]string{"--extract-each", "$.items", "p.json", "q.json"}, "--extract-each requires a single input"},
		{[]string{"--watch=single", "p.json", "q.json"}, "--watch requires a single file or directory"},
		{[]string{"--server", "http://127.0.0.1:1", "--watch=single", "p.json"}, "--server is not compatible with --watch"},
		{[]string{"--tx-offline", "--no-terminology-server", "p.json"}, "--tx-offline and --no-terminology-server are mutually exclusive"},
		{[]string{"--tx-offline", "--server", "http://127.0.0.1:1", "p.json"}, "--tx-offline is not compatible with --server"},
		{[]string{"--tx-offline", "--terminology-server", "https://tx.example.org/r4", "p.json"}, "--tx-offline replaces the terminology server"},
		{[]string{"--tx-offline", "--tx-dir", "none-here", "p.json"}, "no terminology recording in none-here/"},
		{[]string{"--codesystem", "missing-cs.json", "p.json"}, "missing-cs.json"},
	} {
		r := runFakeKeepEnv(t, dir, append([]string{"validate"}, tc.args...)...)
		if r.err == nil || !strings.Contains(r.err.Error(), tc.want) {
			t.Errorf("%q: want an error with %q, got %v", tc.args, tc.want, r.err)
		}
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Errorf("no startup check may reach the validator, but it ran: %v", fakeJavaCalls(t, log))
	}
}

// Checks that depend on fhirlint.yml.
func TestFakeValidate_StartupChecksFromConfig(t *testing.T) {
	for _, tc := range []struct{ name, yaml, want string }{
		{"profile-map that doesn't compile", "profile-map:\n  Patient: {profile: https://example.org/p, when: 'name.('}\n", "name.("},
		{"an override's suppression that doesn't parse", "overrides:\n  - files: '*.json'\n    suppress: [dom-6]\n", `overrides: invalid suppress rule "dom-6"`},
		{"an override's suppression without a reason", "require-suppress-reason: true\noverrides:\n  - files: '*.json'\n    suppress: ['messageId:dom-6']\n", "overrides suppress"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "p.json", cleanPatient)
			writeFile(t, dir, "fhirlint.yml", tc.yaml)
			r := runFake(t, dir, "validate", "p.json")
			if r.err == nil || !strings.Contains(r.err.Error(), tc.want) {
				t.Errorf("want an error with %q, got %v", tc.want, r.err)
			}
		})
	}
}

// A .fhirlintignore that can't be read is a warning; the run goes ahead.
func TestFakeValidate_UnreadableIgnoreFileWarns(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "p.json", cleanPatient)
	if err := os.Mkdir(filepath.Join(dir, ".fhirlintignore"), 0o750); err != nil {
		t.Fatal(err)
	}
	r := runFake(t, dir, "validate", "p.json")
	if r.err != nil || !strings.Contains(r.stderr, "warning: reading .fhirlintignore") {
		t.Errorf("want a warning and a passing run: %v\n%s", r.err, r.stderr)
	}
}

// A user -fhir-settings can't be combined with a replay, which writes its own.
func TestFakeValidate_TxOfflineRefusesAUserFHIRSettings(t *testing.T) {
	srv, _ := fakeTxServer(t)
	dir := t.TempDir()
	writeFile(t, dir, "fhir/p.json", `{"resourceType":"Patient","id":"p1","a":"FAKE_TX:known"}`)
	if r := runFake(t, dir, "tx", "warm", "--terminology-server", srv.URL+"/r4", "fhir"); r.err != nil {
		t.Fatalf("tx warm: %v\n%s", r.err, r.stderr)
	}
	r := runFakeKeepEnv(t, dir, "validate", "--tx-offline", "--validator-arg", "-fhir-settings=mine.json", "fhir")
	if r.err == nil || !strings.Contains(r.err.Error(), "--tx-offline generates its own -fhir-settings") {
		t.Errorf("got %v", r.err)
	}
}

// Without Java there is no run, and the message says how to get it.
func TestFakeValidate_WithoutJava(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "p.json", cleanPatient)
	prepareFake(t)
	t.Setenv("PATH", t.TempDir())
	r := runFakeKeepEnv(t, dir, "validate", "p.json")
	if r.err == nil || !strings.Contains(r.err.Error(), "java not found") {
		t.Errorf("got %v", r.err)
	}
}
