// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"strings"
	"testing"
)

// An environment variable named like a config key changes nothing (#639):
// only fhirlint.yml and the flags set validate's options.
func TestFakeEnvironmentDoesNotSetConfigKeys(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "w.json", warnPatient)
	writeFile(t, dir, "skip/bad.json", errPatient)
	writeFile(t, dir, "fhirlint.yml", "exclude: [skip/]\n")
	prepareFake(t)
	for k, v := range map[string]string{
		"URL":      "https://example.org/Patient/1", // would clash with the file argument
		"SEVERITY": "error",                         // would hide the warning
		"EXCLUDE":  "other/",                        // would replace the config's
		"SERVER":   "http://127.0.0.1:1",            // would send the run nowhere
		"FORMAT":   "json",
	} {
		t.Setenv(k, v)
	}
	r := runFakeKeepEnv(t, dir, "validate", "--no-color", ".")
	if r.err != nil {
		t.Fatalf("the environment must not change the run: %v\n%s", r.err, r.stderr)
	}
	if !strings.Contains(r.stdout, "Fake warning found") || strings.Contains(r.stdout, "skip/bad.json") || strings.Contains(r.stdout, `"files"`) {
		t.Errorf("want the terminal report with the warning and skip/ excluded:\n%s", r.stdout)
	}
}

// The variables fhirlint documents still work: FHIRLINT_JAR is bound
// explicitly, which the fake validator's runs depend on.
func TestFakeFHIRLINTJARStillReadFromTheEnvironment(t *testing.T) {
	jar := prepareFake(t)
	dir := t.TempDir()
	writeFile(t, dir, "ok.json", cleanPatient)
	t.Setenv(fakeJavaLogEnv, dir+"/calls.jsonl")
	mustPass(t, runFakeKeepEnv(t, dir, "validate", "ok.json"))
	for _, c := range fakeJavaCalls(t, dir+"/calls.jsonl") {
		if j := argAfter(c, "-jar"); len(j) > 0 && j[0] != jar {
			t.Errorf("the run must use FHIRLINT_JAR %s, got %s", jar, j[0])
		}
	}
}
