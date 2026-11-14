// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Subcommands run in-process against the fake validator (#595).

func TestFakeStats(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "data/p1.json", cleanPatient)
	writeFile(t, dir, "data/p2.json", errPatient)
	writeFile(t, dir, "data/o1.json", `{"resourceType":"Observation","id":"o1"}`)

	r := runFake(t, dir, "stats", "--no-validate", "data")
	mustPass(t, r)
	if !strings.Contains(r.stdout, "Patient") || !strings.Contains(r.stdout, "Observation") {
		t.Errorf("stats must count each resource type:\n%s", r.stdout)
	}

	r = runFake(t, dir, "stats", "data")
	if !strings.Contains(r.stdout, "Valid  2") || !strings.Contains(r.stdout, "Errors  1") {
		t.Errorf("the validation summary must count the invalid file:\n%s\n%s", r.stdout, r.stderr)
	}
}

// cachePackage puts a package into the FHIR package cache under HOME, which
// runFake has pointed at a temporary directory.
func cachePackage(t *testing.T, name, version string, deps map[string]string) {
	t.Helper()
	home := os.Getenv("HOME")
	m := map[string]any{"name": name, "version": version, "fhirVersions": []string{"4.0.1"}}
	if deps != nil {
		m["dependencies"] = deps
	}
	b, _ := json.Marshal(m)
	writeFile(t, filepath.Join(home, ".fhir", "packages", name+"#"+version, "package"), "package.json", string(b))
	writeFile(t, filepath.Join(home, ".fhir", "packages", name+"#"+version, "package"), "StructureDefinition-x.json", `{"resourceType":"StructureDefinition","id":"x"}`)
}

func TestFakePackagesListAndTree(t *testing.T) {
	dir := t.TempDir()
	// The first run only sets up HOME; the package cache lives there.
	mustPass(t, runFake(t, dir, "packages", "--json"))
	cachePackage(t, "kbv.basis", "1.5.0", map[string]string{"hl7.fhir.r4.core": "4.0.1", "de.basisprofil.r4": "1.5.x"})
	cachePackage(t, "hl7.fhir.r4.core", "4.0.1", nil)

	r := runFakeKeepEnv(t, dir, "packages")
	mustPass(t, r)
	if !strings.Contains(r.stdout, "kbv.basis") || !strings.Contains(r.stdout, "1.5.0") {
		t.Errorf("the cached package must be listed:\n%s", r.stdout)
	}
	r = runFakeKeepEnv(t, dir, "packages", "--json")
	mustPass(t, r)
	var listed any
	if err := json.Unmarshal([]byte(r.stdout), &listed); err != nil || !strings.Contains(r.stdout, "hl7.fhir.r4.core") {
		t.Errorf("--json must print the packages as JSON: %v\n%s", err, r.stdout)
	}

	r = runFakeKeepEnv(t, dir, "packages", "tree", "kbv.basis#1.5.0")
	if !strings.Contains(r.stdout, "hl7.fhir.r4.core") || !strings.Contains(r.stdout, "de.basisprofil.r4") {
		t.Errorf("tree must show both dependencies:\n%s\n%s", r.stdout, r.stderr)
	}
	if !strings.Contains(r.stdout, "de.basisprofil.r4#1.5.x   MISSING") || !strings.Contains(r.stdout, "--offline would fail") {
		t.Errorf("a dependency missing from the cache must be marked, with what it means for --offline:\n%s", r.stdout)
	}
	if r := runFakeKeepEnv(t, dir, "packages", "tree", "nope"); r.err == nil {
		t.Error("a malformed package spec must be an error")
	}
}

// A validator that reports every resource as valid must not qualify: the
// built-in suite contains known-invalid resources it has to reject.
func TestFakeQualify_AnUncriticalValidatorFails(t *testing.T) {
	dir := t.TempDir()
	r := runFake(t, dir, "qualify", "--format", "json")
	if r.err == nil {
		t.Fatalf("qualification must fail when known-invalid cases pass:\n%s", r.stdout)
	}
	if !strings.Contains(r.stdout, `"valid"`) && !strings.Contains(r.stdout, "expected") {
		t.Errorf("the report must list the cases:\n%s", r.stdout)
	}
}

func TestFakeQualify_CustomSuiteCasesAreRecorded(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "suite/bad.json", errPatient)
	writeFile(t, dir, "suite/bad.expected.json", `{"valid": false}`)
	writeFile(t, dir, "suite/ok.json", cleanPatient)
	writeFile(t, dir, "suite/ok.expected.json", `{"valid": true}`)
	r := runFake(t, dir, "qualify", "--format", "json", "--test-suite", "suite")
	for _, name := range []string{"bad.json", "ok.json"} {
		if !strings.Contains(r.stdout, name) {
			t.Errorf("custom case %s missing from the report:\n%s", name, r.stdout)
		}
	}
	if r := runFake(t, dir, "qualify", "--test-suite", "no-such-dir"); r.err == nil {
		t.Error("a missing --test-suite directory must be an error")
	}
}

func TestFakeConfigCheckAndSchema(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "fhirlint.yml", "fail-on: warning\nsuppress:\n  - messageId: X\n    reason: known\n")
	mustPass(t, runFake(t, dir, "config", "check"))

	writeFile(t, dir, "fhirlint.yml", `
fail-on: sometimes
format: [terminal, pdf]
suppress:
  - colour: red
rules:
  - id: r1
overrides:
  - severity: error
unknown-key: 1
`)
	r := runFake(t, dir, "config", "check")
	if r.err == nil {
		t.Fatalf("a broken config must fail the check:\n%s", r.stdout)
	}
	out := r.stdout + r.stderr
	for _, want := range []string{`"sometimes" for "fail-on"`, `"pdf" for "format"`, `unknown suppress key "colour"`, "rule must have an assert expression", `missing required key "files"`, `unknown key "unknown-key"`} {
		if !strings.Contains(out, want) {
			t.Errorf("config check must name %q:\n%s", want, out)
		}
	}

	r = runFake(t, dir, "config", "schema")
	mustPass(t, r)
	var schema map[string]any
	if err := json.Unmarshal([]byte(r.stdout), &schema); err != nil || schema["properties"] == nil {
		t.Errorf("config schema must print a JSON Schema: %v\n%s", err, r.stdout)
	}
}

func TestFakeInit_WritesTheAnswers(t *testing.T) {
	dir := t.TempDir()
	answers := writeFile(t, t.TempDir(), "answers.txt", "https://tx.example.org/r4\nn\nkbv.basis#1.5.0, de.basisprofil.r4#1.5.0\nwarning\n")
	f, err := os.Open(answers) //nolint:gosec // test file
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	old := os.Stdin
	os.Stdin = f
	defer func() { os.Stdin = old }()

	mustPass(t, runFake(t, dir, "init"))
	data, err := os.ReadFile(filepath.Join(dir, "fhirlint.yml")) //nolint:gosec // test output
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"fail-on: warning", "terminology-server: https://tx.example.org/r4", "  - kbv.basis#1.5.0", "  - de.basisprofil.r4#1.5.0"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("fhirlint.yml lacks %q:\n%s", want, data)
		}
	}
	if r := runFake(t, dir, "init"); r.err == nil || !strings.Contains(r.err.Error(), "already exists") {
		t.Errorf("init must not overwrite an existing fhirlint.yml without --force, got %v", r.err)
	}
}
