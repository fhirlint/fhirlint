package reporter

import (
	"encoding/json"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

func issueWithID(severity, message, location, messageID string) validator.Issue {
	return validator.Issue{Severity: severity, Message: message, Location: location, MessageID: messageID}
}

func TestBuildCodeClimateReport_EmptyResults(t *testing.T) {
	report := buildCodeClimateReport(nil, "information")
	if report == nil {
		t.Fatal("expected non-nil slice so it marshals to [] not null")
	}
	if len(report) != 0 {
		t.Errorf("expected 0 entries, got %d", len(report))
	}
	data, _ := json.Marshal(report)
	if string(data) != "[]" {
		t.Errorf("expected empty array to marshal to [], got %s", data)
	}
}

func TestBuildCodeClimateReport_SeverityMapping(t *testing.T) {
	r := makeResult(false,
		issue("fatal", "fatal msg", "Patient"),
		issue("error", "err msg", "Patient.gender"),
		issue("warning", "warn msg", "Patient"),
		issue("information", "info msg", ""),
	)
	report := buildCodeClimateReport([]*validator.Result{r}, "information")

	want := map[string]string{
		"fatal msg": "critical",
		"err msg":   "major",
		"warn msg":  "minor",
		"info msg":  "info",
	}
	for _, e := range report {
		if exp, ok := want[e.Description]; ok && e.Severity != exp {
			t.Errorf("%q: expected severity %q, got %q", e.Description, exp, e.Severity)
		}
	}
}

func TestBuildCodeClimateReport_DescriptionAndCheckName(t *testing.T) {
	r := makeResult(false,
		issueWithID("error", "A resource should have narrative", "Patient", "dom-6"),
		issue("error", "no id here", "Patient"),
	)
	report := buildCodeClimateReport([]*validator.Result{r}, "information")

	if report[0].Description != "dom-6: A resource should have narrative" {
		t.Errorf("expected message-id-prefixed description, got %q", report[0].Description)
	}
	if report[0].CheckName != "dom-6" {
		t.Errorf("expected check_name=dom-6, got %q", report[0].CheckName)
	}
	if report[1].Description != "no id here" {
		t.Errorf("expected bare message when no id, got %q", report[1].Description)
	}
	if report[1].CheckName != ccDefaultCheck {
		t.Errorf("expected default check_name, got %q", report[1].CheckName)
	}
}

func TestBuildCodeClimateReport_LocationLine(t *testing.T) {
	r := makeResult(false,
		issue("error", "with line", "Patient.gender (line 12, col 5)"),
		issue("error", "no line", "Patient.gender"),
	)
	report := buildCodeClimateReport([]*validator.Result{r}, "information")

	if report[0].Location.Lines.Begin != 12 {
		t.Errorf("expected begin line 12, got %d", report[0].Location.Lines.Begin)
	}
	if report[1].Location.Lines.Begin != 1 {
		t.Errorf("expected fallback begin line 1, got %d", report[1].Location.Lines.Begin)
	}
	if report[0].Location.Path != "test.json" {
		t.Errorf("expected path test.json, got %q", report[0].Location.Path)
	}
}

func TestBuildCodeClimateReport_FingerprintStableAndUnique(t *testing.T) {
	r1 := makeResult(false, issueWithID("error", "msg", "Patient.gender", "dom-6"))
	r2 := makeResult(false, issueWithID("error", "msg", "Patient.gender", "dom-6"))
	a := buildCodeClimateReport([]*validator.Result{r1}, "information")
	b := buildCodeClimateReport([]*validator.Result{r2}, "information")

	if a[0].Fingerprint != b[0].Fingerprint {
		t.Error("fingerprint should be stable across identical runs")
	}
	if a[0].Fingerprint == "" {
		t.Error("fingerprint should not be empty")
	}

	// A different issue must produce a different fingerprint.
	other := buildCodeClimateReport([]*validator.Result{
		makeResult(false, issueWithID("error", "different", "Patient.name", "dom-6")),
	}, "information")
	if a[0].Fingerprint == other[0].Fingerprint {
		t.Error("distinct issues should have distinct fingerprints")
	}
}

func TestBuildCodeClimateReport_SeverityFilter(t *testing.T) {
	r := makeResult(false,
		issue("error", "an error", "Patient.gender"),
		issue("warning", "a warning", "Patient"),
		issue("information", "an info", ""),
	)
	report := buildCodeClimateReport([]*validator.Result{r}, "error")

	if len(report) != 1 {
		t.Fatalf("expected only the error to survive --severity error, got %d entries", len(report))
	}
	if report[0].Description != "an error" {
		t.Errorf("expected the error entry, got %q", report[0].Description)
	}
}
