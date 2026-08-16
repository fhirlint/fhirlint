package redact_test

import (
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/redact"
	"github.com/fhirlint/fhirlint/internal/validator"
)

// phi stands in for the kind of value the validator quotes into its message
// text. If it survives anywhere in a report, the redaction failed.
const phi = "1974-03-11"

func sampleResults() []*validator.Result {
	return []*validator.Result{
		{
			Filename:   "patients/p1.json",
			Label:      "patients/p1.json",
			SourcePath: "patients/p1.json",
			Issues: []validator.Issue{
				{
					Severity:         "error",
					Message:          "The value '" + phi + "' is not a valid date",
					Location:         "Patient.birthDate (line 5, col 18)",
					MessageID:        "Type_Specific_Checks_DT_Date_Valid",
					OriginalSeverity: "warning",
				},
			},
			Suppressed: []validator.Issue{
				{
					Severity:       "warning",
					Message:        "Patient.name '" + phi + "' has no text",
					Location:       "Patient.name[0]",
					MessageID:      "dom-6",
					SuppressReason: "accepted in 2024, tracked in FHIR-421",
				},
			},
		},
	}
}

func TestApplyRemovesMessageText(t *testing.T) {
	results := sampleResults()
	redact.Apply(results)

	issue := results[0].Issues[0]
	if strings.Contains(issue.Message, phi) {
		t.Errorf("message still carries the value: %q", issue.Message)
	}
	if issue.Message != redact.Placeholder {
		t.Errorf("message = %q, want %q", issue.Message, redact.Placeholder)
	}
	if !issue.Redacted {
		t.Error("Redacted must be set so a reader can tell the report was stripped")
	}
}

func TestApplyKeepsWhatDescribesTheFinding(t *testing.T) {
	results := sampleResults()
	redact.Apply(results)

	issue := results[0].Issues[0]
	// None of these come from the resource: they describe where and what the
	// finding is, which is what makes a redacted report still actionable.
	if issue.Severity != "error" {
		t.Errorf("severity = %q, want error", issue.Severity)
	}
	if issue.Location != "Patient.birthDate (line 5, col 18)" {
		t.Errorf("location was altered: %q", issue.Location)
	}
	if issue.MessageID != "Type_Specific_Checks_DT_Date_Valid" {
		t.Errorf("messageID was altered: %q", issue.MessageID)
	}
	if issue.OriginalSeverity != "warning" {
		t.Errorf("originalSeverity was altered: %q", issue.OriginalSeverity)
	}
	if results[0].Filename != "patients/p1.json" {
		t.Errorf("filename was altered: %q", results[0].Filename)
	}
}

func TestApplyRedactsSuppressedIssues(t *testing.T) {
	results := sampleResults()
	redact.Apply(results)

	// Suppressed issues are carried into JSON and HTML output for auditability,
	// so skipping them would leak through a second door.
	supp := results[0].Suppressed[0]
	if strings.Contains(supp.Message, phi) {
		t.Errorf("suppressed message still carries the value: %q", supp.Message)
	}
	if !supp.Redacted {
		t.Error("suppressed issue must be marked redacted")
	}
	// The reason is written by the user in their own config, not derived from
	// the resource, and it is the whole point of showing suppressed findings.
	if supp.SuppressReason != "accepted in 2024, tracked in FHIR-421" {
		t.Errorf("suppression reason was altered: %q", supp.SuppressReason)
	}
}

func TestApplyClearsSourcePath(t *testing.T) {
	results := sampleResults()
	redact.Apply(results)

	// Source snippets are resolved against SourcePath. Clearing it makes the
	// snippet impossible to render rather than merely switched off, so a
	// reporter honouring --show-source cannot reintroduce the leak.
	if results[0].SourcePath != "" {
		t.Errorf("SourcePath = %q, want empty", results[0].SourcePath)
	}
}

func TestApplyToleratesNilResults(t *testing.T) {
	results := []*validator.Result{nil, sampleResults()[0], nil}

	redact.Apply(results) // must not panic

	if !results[1].Issues[0].Redacted {
		t.Error("a nil entry must not stop later results from being redacted")
	}
}

func TestApplyOnEmptyInput(t *testing.T) {
	redact.Apply(nil)
	redact.Apply([]*validator.Result{})
}
