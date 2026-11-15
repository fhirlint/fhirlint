// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fhirlint fhirpath against the fake validator's fhirpath mode (#630).

const fpPatient = `{"resourceType":"Patient","id":"p1","name":[{"family":"Muster","given":["Anna","Lena"]}]}`

func TestFakeFHIRPath_Results(t *testing.T) {
	prepareFake(t)
	dir := t.TempDir()
	writeFile(t, dir, "p.json", fpPatient)
	log := filepath.Join(t.TempDir(), "calls.jsonl")
	t.Setenv(fakeJavaLogEnv, log)

	for _, tc := range []struct {
		expr, want string
	}{
		{"Patient.name.given", "[0] Anna\n[1] Lena\n"},
		{"Patient.name.family", "Muster\n"},
		{"Patient.birthDate", "(empty)\n"}, // no match is a result, not an error
	} {
		r := runFakeKeepEnv(t, dir, "fhirpath", tc.expr, "p.json")
		if r.err != nil || r.stdout != tc.want {
			t.Errorf("%s: %v\n%q", tc.expr, r.err, r.stdout)
		}
	}

	// The engine runs without terminology, for the version asked for.
	calls := fakeJavaCalls(t, log)
	last := calls[len(calls)-1]
	if v := argAfter(last, "-version"); len(v) == 0 || v[0] != "4.0.1" {
		t.Errorf("default FHIR version: %v", last)
	}
	if tx := argAfter(last, "-tx"); len(tx) == 0 || tx[0] != "n/a" {
		t.Errorf("fhirpath must run without a terminology server: %v", last)
	}
	r := runFakeKeepEnv(t, dir, "fhirpath", "--fhir-version", "5.0.0", "Patient.id", "p.json")
	calls = fakeJavaCalls(t, log)
	if v := argAfter(calls[len(calls)-1], "-version"); r.err != nil || len(v) == 0 || v[0] != "5.0.0" {
		t.Errorf("--fhir-version 5.0.0: %v %v", r.err, calls[len(calls)-1])
	}
}

func TestFakeFHIRPath_JSONAndStdin(t *testing.T) {
	prepareFake(t)
	dir := t.TempDir()
	in := writeFile(t, dir, "p.json", fpPatient)
	f, err := os.Open(in) //nolint:gosec // test file
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	old := os.Stdin
	os.Stdin = f
	defer func() { os.Stdin = old }()

	r := runFakeKeepEnv(t, dir, "fhirpath", "--format", "json", "Patient.name.given")
	if r.err != nil {
		t.Fatalf("%v\n%s", r.err, r.stderr)
	}
	var got struct {
		Expression string   `json:"expression"`
		Result     []string `json:"result"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &got); err != nil || got.Expression != "Patient.name.given" || strings.Join(got.Result, "|") != "Anna|Lena" {
		t.Errorf("JSON from stdin: %v %+v\n%s", err, got, r.stdout)
	}
}

// Values come from the resource, so control characters in them are made
// harmless before they reach the terminal (#496).
func TestFakeFHIRPath_ItemsAreTerminalSafe(t *testing.T) {
	prepareFake(t)
	dir := t.TempDir()
	// An OSC sequence (set the window title): the parser strips only colour
	// codes, so this one reaches the printing and only termsafe stops it.
	writeFile(t, dir, "p.json", `{"resourceType":"Patient","id":"p1","name":[{"family":"\u001b]0;pwned\u0007Muster"}]}`)
	r := runFakeKeepEnv(t, dir, "fhirpath", "Patient.name.family", "p.json")
	if r.err != nil || strings.ContainsAny(r.stdout, "\x1b\x07") || !strings.Contains(r.stdout, "Muster") {
		t.Errorf("an escape sequence from the resource must not reach the terminal: %v %q", r.err, r.stdout)
	}
}

func TestFakeFHIRPath_Errors(t *testing.T) {
	prepareFake(t)
	dir := t.TempDir()
	writeFile(t, dir, "p.json", fpPatient)
	writeFile(t, dir, "broken.json", `{"resourceType":"Patient",`)
	writeFile(t, dir, "crash.json", `{"resourceType":"Patient","note":"FAKE_CRASH"}`)
	writeFile(t, dir, "sub/p.json", fpPatient)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"Patient.name.(", "p.json"}, "Expression terminated unexpectedly"},
		{[]string{"Patient.name", "broken.json"}, "Error parsing JSON source"},
		{[]string{"Patient.name", "crash.json"}, "Error: fake crash before evaluation"},
		{[]string{"Patient.name", "sub"}, "is a directory"},
		{[]string{"Patient.name", "missing.json"}, "missing.json"},
		{[]string{"  ", "p.json"}, "empty FHIRPath expression"},
		{[]string{"--format", "xml", "Patient.name", "p.json"}, `unknown format "xml"`},
		{[]string{"--fhir-version", "9.9.9", "Patient.name", "p.json"}, "9.9.9"},
	} {
		r := runFakeKeepEnv(t, dir, append([]string{"fhirpath"}, tc.args...)...)
		var ee *exitErr
		if !errors.As(r.err, &ee) || ee.code != 2 || !strings.Contains(r.err.Error(), tc.want) {
			t.Errorf("%q: want exit 2 with %q, got %v", tc.args, tc.want, r.err)
		}
	}
}
