// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// End-to-end tests of `fhirlint validate` against the fake validator in
// fakejar_test.go (#595). Each one checks behaviour a user sees: the exit
// code, the report, or a file fhirlint writes.

const (
	cleanPatient = `{"resourceType":"Patient","id":"p1"}`
	errPatient   = `{"resourceType":"Patient","id":"p2","note":"FAKE_ERROR"}`
	warnPatient  = `{"resourceType":"Patient","id":"p3","note":"FAKE_WARNING"}`
)

func mustFail(t *testing.T, r fakeRun) {
	t.Helper()
	if !errors.Is(r.err, errValidationFailed) {
		t.Fatalf("want a failed validation, got %v\nstdout:\n%s\nstderr:\n%s", r.err, r.stdout, r.stderr)
	}
}

func mustPass(t *testing.T, r fakeRun) {
	t.Helper()
	if r.err != nil {
		t.Fatalf("want success, got %v\nstdout:\n%s\nstderr:\n%s", r.err, r.stdout, r.stderr)
	}
}

func readJSONReport(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // test output
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("report is not JSON: %v\n%s", err, data)
	}
	return m
}

func TestFakeValidate_CleanResourcePasses(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "patient.json", cleanPatient)
	r := runFake(t, dir, "validate", "--no-color", "patient.json")
	mustPass(t, r)
	if !strings.Contains(r.stdout, "Valid: 1") {
		t.Errorf("summary should count one valid file:\n%s", r.stdout)
	}
}

func TestFakeValidate_ErrorFails(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "patient.json", errPatient)
	r := runFake(t, dir, "validate", "--no-color", "patient.json")
	mustFail(t, r)
	if !strings.Contains(r.stdout, "Fake error found") || !strings.Contains(r.stdout, "Patient.name") {
		t.Errorf("finding and location missing from the report:\n%s", r.stdout)
	}
}

func TestFakeValidate_FailOnThresholds(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "patient.json", warnPatient)
	mustPass(t, runFake(t, dir, "validate", "patient.json"))
	mustFail(t, runFake(t, dir, "validate", "--fail-on", "warning", "patient.json"))

	writeFile(t, dir, "bad.json", errPatient)
	mustPass(t, runFake(t, dir, "validate", "--fail-on", "never", "bad.json"))
	if r := runFake(t, dir, "validate", "--fail-on", "sometimes", "bad.json"); r.err == nil || errors.Is(r.err, errValidationFailed) {
		t.Errorf("an unknown --fail-on value must be a usage error, got %v", r.err)
	}
}

func TestFakeValidate_MaxWarnings(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.json", warnPatient)
	writeFile(t, dir, "b.json", strings.Replace(warnPatient, "p3", "p4", 1))
	mustPass(t, runFake(t, dir, "validate", "--max-warnings", "2", "a.json", "b.json"))
	r := runFake(t, dir, "validate", "--max-warnings", "1", "a.json", "b.json")
	if r.err == nil {
		t.Fatalf("two warnings over --max-warnings 1 must fail:\n%s", r.stdout)
	}
}

func TestFakeValidate_DirectoryAndExclude(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "fhir/ok.json", cleanPatient)
	writeFile(t, dir, "fhir/legacy/bad.json", errPatient)
	mustFail(t, runFake(t, dir, "validate", "fhir"))
	r := runFake(t, dir, "validate", "--exclude", "legacy/", "fhir")
	mustPass(t, r)
	if strings.Contains(r.stdout, "bad.json") {
		t.Errorf("excluded file still reported:\n%s", r.stdout)
	}

	writeFile(t, dir, ".fhirlintignore", "legacy/\n")
	mustPass(t, runFake(t, dir, "validate", "fhir"))
}

func TestFakeValidate_SeverityFilterHidesButStillFails(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "patient.json", strings.Replace(errPatient, "FAKE_ERROR", "FAKE_ERROR FAKE_WARNING", 1))
	r := runFake(t, dir, "validate", "--no-color", "--severity", "error", "patient.json")
	mustFail(t, r)
	if strings.Contains(r.stdout, "Fake warning found") {
		t.Errorf("--severity error must hide the warning:\n%s", r.stdout)
	}
	if !strings.Contains(r.stdout, "Fake error found") {
		t.Errorf("the error must stay:\n%s", r.stdout)
	}
}

func TestFakeValidate_SuppressFlag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "patient.json", errPatient)
	r := runFake(t, dir, "validate", "--no-color", "--suppress", "messageId:FAKE_ERROR_ID", "patient.json")
	mustPass(t, r)
	if strings.Contains(r.stdout, "Fake error found") {
		t.Errorf("suppressed finding still shown:\n%s", r.stdout)
	}
	r = runFake(t, dir, "validate", "--no-color", "--show-suppressed", "--suppress", "messageId:FAKE_ERROR_ID", "patient.json")
	mustPass(t, r)
	if !strings.Contains(r.stdout, "Fake error found") {
		t.Errorf("--show-suppressed must show the suppressed finding:\n%s", r.stdout)
	}
	if r := runFake(t, dir, "validate", "--require-suppress-reason", "--suppress", "messageId:FAKE_ERROR_ID", "patient.json"); r.err == nil {
		t.Error("--require-suppress-reason must reject a --suppress without a reason")
	}
}

func TestFakeValidate_ConfigSuppressAndSeverityOverride(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "patient.json", errPatient)
	writeFile(t, dir, "fhirlint.yml", `
severity-override:
  - messageId: FAKE_ERROR_ID
    severity: warning
    reason: profile defect
`)
	r := runFake(t, dir, "validate", "--no-color", "patient.json")
	mustPass(t, r)
	if !strings.Contains(r.stdout, "Fake error found") {
		t.Errorf("a re-levelled finding must still be shown:\n%s", r.stdout)
	}

	writeFile(t, dir, "fhirlint.yml", `
suppress:
  - messageId: FAKE_ERROR_ID
    reason: known
`)
	mustPass(t, runFake(t, dir, "validate", "patient.json"))

	// --no-repo-config ignores the checked directory's fhirlint.yml.
	mustFail(t, runFake(t, dir, "validate", "--no-repo-config", "patient.json"))
}

func TestFakeValidate_ExpiredSuppressionReportsAgain(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "patient.json", errPatient)
	writeFile(t, dir, "fhirlint.yml", `
suppress:
  - messageId: FAKE_ERROR_ID
    reason: temporary
    expires: 2020-01-01
`)
	r := runFake(t, dir, "validate", "patient.json")
	mustFail(t, r)
	if !strings.Contains(r.stderr+r.stdout, "2020-01-01") {
		t.Errorf("the lapsed rule should be named with its date:\nstdout:\n%s\nstderr:\n%s", r.stdout, r.stderr)
	}
}

func TestFakeValidate_OverridesNeverFail(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "fixtures/invalid/bad.json", errPatient)
	writeFile(t, dir, "fixtures/ok.json", cleanPatient)
	writeFile(t, dir, "fhirlint.yml", `
overrides:
  - files: ["fixtures/invalid/**"]
    fail-on: never
`)
	mustPass(t, runFake(t, dir, "validate", "fixtures"))
}

func TestFakeValidate_BaselineGenerateAndApply(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "patient.json", errPatient)
	r := runFake(t, dir, "validate", "--generate-baseline", "baseline.json", "patient.json")
	mustPass(t, r)
	if _, err := os.Stat(filepath.Join(dir, "baseline.json")); err != nil {
		t.Fatalf("baseline not written: %v", err)
	}
	mustPass(t, runFake(t, dir, "validate", "--baseline", "baseline.json", "patient.json"))

	// A new finding is not in the baseline and fails again.
	writeFile(t, dir, "patient.json", strings.Replace(errPatient, "FAKE_ERROR", "FAKE_ERROR FAKE_WARNING", 1))
	mustFail(t, runFake(t, dir, "validate", "--fail-on", "warning", "--baseline", "baseline.json", "patient.json"))
}

func TestFakeValidate_JSONReport(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "bad.json", errPatient)
	writeFile(t, dir, "ok.json", cleanPatient)
	r := runFake(t, dir, "validate", "--format", "json", "--output", "report.json", "bad.json", "ok.json")
	mustFail(t, r)
	rep := readJSONReport(t, filepath.Join(dir, "report.json"))
	files, _ := rep["files"].([]any)
	if len(files) != 2 {
		t.Fatalf("want two files in the report, got %d: %v", len(files), rep)
	}
	if !strings.Contains(mustMarshal(t, rep), "FAKE_ERROR_ID") {
		t.Errorf("the message id must be in the report:\n%s", mustMarshal(t, rep))
	}
}

func mustMarshal(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestFakeValidate_ReportFormats(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "bad.json", errPatient)
	for _, tc := range []struct {
		format, want string
		xmlDoc       bool
	}{
		{"sarif", `"FAKE_ERROR_ID"`, false},
		{"junit", "Fake error found", true},
		{"html", "Fake error found", false},
		{"markdown", "Fake error found", false},
		{"codeclimate", "FAKE_ERROR_ID", false},
		{"github", "::error", false},
	} {
		t.Run(tc.format, func(t *testing.T) {
			r := runFake(t, dir, "validate", "--format", tc.format, "bad.json")
			mustFail(t, r)
			if !strings.Contains(r.stdout, tc.want) {
				t.Fatalf("%s output lacks %q:\n%s", tc.format, tc.want, r.stdout)
			}
			if tc.xmlDoc {
				if err := xml.Unmarshal([]byte(r.stdout), new(struct{})); err != nil {
					t.Errorf("%s output is not well-formed XML: %v", tc.format, err)
				}
			}
		})
	}
}

func TestFakeValidate_Redact(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "bad.json", errPatient)
	r := runFake(t, dir, "validate", "--redact", "--format", "json", "--output", "report.json", "bad.json")
	mustFail(t, r)
	rep := mustMarshal(t, readJSONReport(t, filepath.Join(dir, "report.json")))
	if strings.Contains(rep, "Fake error found") {
		t.Errorf("--redact must remove the message text:\n%s", rep)
	}
	if !strings.Contains(rep, "FAKE_ERROR_ID") {
		t.Errorf("--redact keeps the message id:\n%s", rep)
	}
}

func TestFakeValidate_ResultCacheSkipsTheValidator(t *testing.T) {
	dir := t.TempDir()
	cacheDir := t.TempDir()
	writeFile(t, dir, "bad.json", errPatient)
	mustFail(t, runFake(t, dir, "validate", "--cache", "--cache-dir", cacheDir, "bad.json"))

	// Second run: the validator refuses to run, so only a cache hit can answer.
	t.Setenv(fakeJavaRefuseEnv, "1")
	r := runFakeKeepEnv(t, dir, "validate", "--no-color", "--cache", "--cache-dir", cacheDir, "bad.json")
	if !errors.Is(r.err, errValidationFailed) || !strings.Contains(r.stdout, "Fake error found") {
		t.Fatalf("a cached result must reproduce the finding without java: %v\n%s\n%s", r.err, r.stdout, r.stderr)
	}
}

func TestFakeValidate_JARCrashIsAFailureNotAPass(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "patient.json", `{"resourceType":"Patient","note":"FAKE_CRASH"}`)
	r := runFake(t, dir, "validate", "patient.json")
	if r.err == nil {
		t.Fatalf("a crashed validator must not pass:\n%s\n%s", r.stdout, r.stderr)
	}
}

func TestFakeValidate_ExtractAndIgnore(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "envelope.json", `{"data":`+errPatient+`}`)
	mustFail(t, runFake(t, dir, "validate", "--extract", "$.data", "envelope.json"))

	writeFile(t, dir, "patient.json", errPatient)
	mustPass(t, runFake(t, dir, "validate", "--ignore", "$.note", "patient.json"))
	data, _ := os.ReadFile(filepath.Join(dir, "patient.json")) //nolint:gosec // test file
	if string(data) != errPatient {
		t.Errorf("--ignore must not modify the input file, now %s", data)
	}
}

func TestFakeValidate_ExtractEach(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "list.json", `{"items":[`+cleanPatient+`,`+errPatient+`]}`)
	r := runFake(t, dir, "validate", "--no-color", "--extract-each", "$.items", "list.json")
	mustFail(t, r)
	if !strings.Contains(r.stdout, "Files: 2") {
		t.Errorf("each element is validated on its own:\n%s", r.stdout)
	}
}

func TestFakeValidate_NDJSON(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "export.ndjson", cleanPatient+"\n"+errPatient+"\n")
	r := runFake(t, dir, "validate", "--no-color", "export.ndjson")
	mustFail(t, r)
	if !strings.Contains(r.stdout, "Files: 2") {
		t.Errorf("each line is validated on its own:\n%s", r.stdout)
	}
}

func TestFakeValidate_BundleEntries(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "bundle.json", `{"resourceType":"Bundle","type":"collection","entry":[{"resource":`+errPatient+`}]}`)
	r := runFake(t, dir, "validate", "--no-color", "--bundle-entries", "bundle.json")
	mustFail(t, r)
	if !strings.Contains(r.stdout, "Files: 2") {
		t.Errorf("the Bundle and its entry are both validated:\n%s", r.stdout)
	}
}

func TestFakeValidate_SkipNonFHIR(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in/patient.json", cleanPatient)
	writeFile(t, dir, "in/package.json", `{"name":"not-fhir","version":"1.0.0"}`)
	r := runFake(t, dir, "validate", "--no-color", "--skip-non-fhir", "in")
	mustPass(t, r)
	if strings.Contains(r.stdout, "package.json") {
		t.Errorf("--skip-non-fhir must skip package.json:\n%s", r.stdout)
	}
}

func TestFakeValidate_LintAndCustomRulesJoinTheFindings(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "patient.json", `{"resourceType":"Patient","id":"Upper_Case"}`)
	writeFile(t, dir, "fhirlint.yml", `
lint:
  id-kebab-case: error
rules:
  - id: has-name
    assert: "name.exists()"
    severity: warning
`)
	r := runFake(t, dir, "validate", "--no-color", "--fail-on", "warning", "patient.json")
	mustFail(t, r)
	for _, want := range []string{"is not lowercase kebab-case", `rule "has-name" failed`} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("report lacks %s:\n%s", want, r.stdout)
		}
	}
}

func TestFakeValidate_CheckReferences(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "set/obs.json", `{"resourceType":"Observation","id":"o1","status":"final","code":{"text":"x"},"subject":{"reference":"Patient/missing"}}`)
	r := runFake(t, dir, "validate", "--no-color", "--check-references", "set")
	mustFail(t, r)
	if !strings.Contains(r.stdout, "Patient/missing") {
		t.Errorf("the unresolved reference must be reported:\n%s", r.stdout)
	}
}

func TestFakeValidate_GroupQuietSlowest(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.json", errPatient)
	writeFile(t, dir, "b.json", strings.Replace(errPatient, "p2", "p5", 1))
	writeFile(t, dir, "c.json", cleanPatient)
	r := runFake(t, dir, "validate", "--no-color", "--group", "--quiet", "--slowest", "2", "a.json", "b.json", "c.json")
	mustFail(t, r)
	if strings.Count(r.stdout, "Fake error found") != 1 || !strings.Contains(r.stdout, "2 files") {
		t.Errorf("--group must show the repeated finding once, with its count:\n%s", r.stdout)
	}
	// The fake validator logs no per-file times, and --slowest says so rather
	// than printing an empty list.
	if !strings.Contains(r.stdout, "No validation times") {
		t.Errorf("--slowest without timings must say so:\n%s", r.stdout)
	}
}

func TestFakeValidate_ShowSource(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "patient.json", errPatient)
	r := runFake(t, dir, "validate", "--no-color", "--show-source", "patient.json")
	mustFail(t, r)
	if !strings.Contains(r.stdout, `"note":"FAKE_ERROR"`) {
		t.Errorf("--show-source must quote line 1 of the file:\n%s", r.stdout)
	}
}

func TestFakeValidate_Stdin(t *testing.T) {
	dir := t.TempDir()
	in := writeFile(t, dir, "stdin.json", errPatient)
	f, err := os.Open(in) //nolint:gosec // test file
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	old := os.Stdin
	os.Stdin = f
	defer func() { os.Stdin = old }()
	mustFail(t, runFake(t, dir, "validate"))
}

func TestFakeValidate_LockWritesAndVerifies(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "patient.json", cleanPatient)
	mustPass(t, runFake(t, dir, "validate", "--lock", "patient.json"))
	if _, err := os.Stat(filepath.Join(dir, "fhirlint.lock")); err != nil {
		t.Fatalf("--lock must write fhirlint.lock: %v", err)
	}
	mustPass(t, runFake(t, dir, "validate", "patient.json"))
}

func TestFakeValidate_UsageErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.json", cleanPatient)
	writeFile(t, dir, "b.json", cleanPatient)
	for _, args := range [][]string{
		{"validate", "--extract", "$.a", "--extract-each", "$.b", "a.json"},
		{"validate", "--extract-each", "$.a", "a.json", "b.json"},
		{"validate", "--timeout", "soon", "a.json"},
		{"validate", "--max-messages", "-3", "a.json"},
		{"validate", "--slowest", "-1", "a.json"},
		{"validate", "missing.json"},
	} {
		if r := runFake(t, dir, args...); r.err == nil || errors.Is(r.err, errValidationFailed) {
			t.Errorf("%q must be a usage error, got %v", args, r.err)
		}
	}
}
