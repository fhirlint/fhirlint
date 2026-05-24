package qualify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sampleReport(qualified bool) *Report {
	cases := []CaseResult{
		{Name: "valid/patient.json", Expectation: "valid", Pass: true, Detail: "accepted, no errors (expected)"},
		{Name: "invalid/bad.json", Expectation: "invalid", Pass: qualified, Detail: "error detected (expected)"},
	}
	passed, failed, q := Summarize(cases)
	return &Report{
		ToolVersion: "v9.9.9",
		JARVersion:  "6.9.7",
		JARSHA256:   "deadbeef",
		FHIRVersion: "4.0.1",
		Terminology: "offline",
		Timestamp:   "2026-05-24T00:00:00Z",
		Cases:       cases,
		Passed:      passed,
		Failed:      failed,
		Qualified:   q,
	}
}

func TestTerminal_ContainsKeyLines(t *testing.T) {
	out := Terminal(sampleReport(true))
	for _, want := range []string{
		"Operational Qualification",
		"Tool version:  v9.9.9",
		"JAR SHA256:    deadbeef",
		"Test cases: 2 passed · 0 failed",
		"PASS  valid/patient.json",
		"Result: QUALIFIED ✓",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("terminal output missing %q\n---\n%s", want, out)
		}
	}
}

func TestTerminal_NotQualified(t *testing.T) {
	out := Terminal(sampleReport(false))
	if !strings.Contains(out, "Result: NOT QUALIFIED ✗") {
		t.Errorf("expected NOT QUALIFIED banner, got:\n%s", out)
	}
	if !strings.Contains(out, "FAIL  invalid/bad.json") {
		t.Errorf("expected a FAIL row, got:\n%s", out)
	}
}

func TestJSON_RoundTrips(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "report.json")
	if err := JSON(sampleReport(true), dest); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest) //nolint:gosec // test reads its own temp file
	if err != nil {
		t.Fatal(err)
	}
	var r Report
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if r.ToolVersion != "v9.9.9" || !r.Qualified || r.Passed != 2 {
		t.Errorf("round-tripped report mismatch: %+v", r)
	}
}

func TestHTML_ContainsBannerAndCases(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "report.html")
	if err := HTML(sampleReport(true), dest); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest) //nolint:gosec // test reads its own temp file
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, want := range []string{
		"<!DOCTYPE html>",
		"QUALIFIED ✓",
		"deadbeef",
		"valid/patient.json",
		"Reviewed by:",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML report missing %q", want)
		}
	}
}
