package reporter

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

func TestBuildJUnitReport_ValidNoIssues(t *testing.T) {
	r := makeResult(true)
	report := buildJUnitReport([]*validator.Result{r}, "information")

	if report.Tests != 1 {
		t.Errorf("expected tests=1, got %d", report.Tests)
	}
	if report.Failures != 0 {
		t.Errorf("expected failures=0, got %d", report.Failures)
	}
	if len(report.Suites[0].TestCases[0].Failures) != 0 {
		t.Errorf("expected no failures in testcase, got %d", len(report.Suites[0].TestCases[0].Failures))
	}
}

func TestBuildJUnitReport_SingleError(t *testing.T) {
	r := makeResult(false, issue("error", "bad value", "Patient.gender"))
	report := buildJUnitReport([]*validator.Result{r}, "information")

	if report.Failures != 1 {
		t.Errorf("expected failures=1, got %d", report.Failures)
	}
	tc := report.Suites[0].TestCases[0]
	if len(tc.Failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(tc.Failures))
	}
	if tc.Failures[0].Type != "error" {
		t.Errorf("expected failure type=error, got %q", tc.Failures[0].Type)
	}
	if tc.Failures[0].Message != "bad value" {
		t.Errorf("unexpected message: %q", tc.Failures[0].Message)
	}
}

func TestBuildJUnitReport_BodyIncludesLocation(t *testing.T) {
	r := makeResult(false, issue("error", "bad value", "Patient.gender"))
	report := buildJUnitReport([]*validator.Result{r}, "information")

	body := report.Suites[0].TestCases[0].Failures[0].Body
	if !strings.Contains(body, "Patient.gender") {
		t.Errorf("expected body to contain location, got: %q", body)
	}
}

func TestBuildJUnitReport_SeverityFilter(t *testing.T) {
	r := makeResult(false,
		issue("error", "err", ""),
		issue("warning", "warn", ""),
		issue("information", "info", ""),
	)
	report := buildJUnitReport([]*validator.Result{r}, "warning")

	if report.Failures != 2 {
		t.Errorf("expected 2 failures (error+warning), got %d", report.Failures)
	}
}

func TestBuildJUnitReport_MultipleFiles(t *testing.T) {
	r1 := makeResult(false, issue("error", "e1", ""))
	r2 := makeResult(true)
	report := buildJUnitReport([]*validator.Result{r1, r2}, "information")

	if report.Tests != 2 {
		t.Errorf("expected tests=2, got %d", report.Tests)
	}
	if report.Failures != 1 {
		t.Errorf("expected failures=1, got %d", report.Failures)
	}
	if len(report.Suites[0].TestCases) != 2 {
		t.Errorf("expected 2 testcases, got %d", len(report.Suites[0].TestCases))
	}
}

func TestBuildJUnitReport_TestCaseLabel(t *testing.T) {
	r := &validator.Result{Filename: "test.json", Label: "patient-001.json", Valid: true}
	report := buildJUnitReport([]*validator.Result{r}, "information")

	name := report.Suites[0].TestCases[0].Name
	if name != "patient-001.json" {
		t.Errorf("expected testcase name=patient-001.json, got %q", name)
	}
}

func TestJUnit_OutputIsValidXML(t *testing.T) {
	r := makeResult(false, issue("error", "bad value", "Patient.gender"))
	report := buildJUnitReport([]*validator.Result{r}, "information")

	out, err := xml.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("xml.MarshalIndent failed: %v", err)
	}
	var parsed junitTestSuites
	if err := xml.Unmarshal(out, &parsed); err != nil {
		t.Errorf("output is not valid XML: %v", err)
	}
}

func TestJUnit_XMLHeader(t *testing.T) {
	r := makeResult(true)
	report := buildJUnitReport([]*validator.Result{r}, "information")

	out, err := xml.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("xml.MarshalIndent failed: %v", err)
	}
	full := xml.Header + string(out)
	if !strings.HasPrefix(full, "<?xml") {
		t.Errorf("expected XML declaration at start, got: %q", full[:20])
	}
}
