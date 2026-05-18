package reporter

import (
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

func TestFilterIssues_InformationShowsAll(t *testing.T) {
	issues := []validator.Issue{
		{Severity: "error"},
		{Severity: "warning"},
		{Severity: "information"},
	}
	got := filterIssues(issues, "information")
	if len(got) != 3 {
		t.Errorf("expected 3 issues with min=information, got %d", len(got))
	}
}

func TestFilterIssues_WarningHidesInfo(t *testing.T) {
	issues := []validator.Issue{
		{Severity: "error"},
		{Severity: "warning"},
		{Severity: "information"},
	}
	got := filterIssues(issues, "warning")
	if len(got) != 2 {
		t.Errorf("expected 2 issues (error+warning) with min=warning, got %d", len(got))
	}
	for _, i := range got {
		if i.Severity == "information" {
			t.Error("information issue should be filtered out")
		}
	}
}

func TestFilterIssues_ErrorOnly(t *testing.T) {
	issues := []validator.Issue{
		{Severity: "error"},
		{Severity: "warning"},
		{Severity: "information"},
	}
	got := filterIssues(issues, "error")
	if len(got) != 1 {
		t.Errorf("expected 1 issue with min=error, got %d", len(got))
	}
	if got[0].Severity != "error" {
		t.Errorf("expected error severity, got %q", got[0].Severity)
	}
}

func TestFilterIssues_EmptyInput(t *testing.T) {
	got := filterIssues(nil, "information")
	if len(got) != 0 {
		t.Errorf("expected 0 issues for nil input, got %d", len(got))
	}
}

func TestFilterIssues_AllFiltered(t *testing.T) {
	issues := []validator.Issue{
		{Severity: "information"},
		{Severity: "warning"},
	}
	got := filterIssues(issues, "error")
	if len(got) != 0 {
		t.Errorf("expected 0 issues when all are below threshold, got %d", len(got))
	}
}

func TestFilterIssues_PreservesOrder(t *testing.T) {
	issues := []validator.Issue{
		{Severity: "error", Message: "first"},
		{Severity: "error", Message: "second"},
		{Severity: "error", Message: "third"},
	}
	got := filterIssues(issues, "error")
	for i, want := range []string{"first", "second", "third"} {
		if got[i].Message != want {
			t.Errorf("position %d: expected %q, got %q", i, want, got[i].Message)
		}
	}
}

func TestTerminalSummary_ErrorCount(t *testing.T) {
	results := []*validator.Result{
		{Valid: false, Issues: []validator.Issue{
			{Severity: "error"},
			{Severity: "error"},
			{Severity: "warning"},
		}},
		{Valid: true, Issues: []validator.Issue{
			{Severity: "information"},
		}},
	}
	// TerminalSummary prints to stdout — we just verify it doesn't panic
	// and returns the correct total count
	total := TerminalSummary(results, "information")
	if total != 4 {
		t.Errorf("expected total=4, got %d", total)
	}
}

func TestFilterIssues_FatalAlwaysShown(t *testing.T) {
	issues := []validator.Issue{
		{Severity: "fatal"},
		{Severity: "error"},
		{Severity: "warning"},
		{Severity: "information"},
	}
	// fatal should survive even the strictest filter
	got := filterIssues(issues, "error")
	if len(got) != 2 {
		t.Errorf("expected fatal+error with min=error, got %d issues", len(got))
	}
	for _, i := range got {
		if i.Severity != "fatal" && i.Severity != "error" {
			t.Errorf("unexpected severity %q in filtered result", i.Severity)
		}
	}
}

func TestTerminalSummary_FatalCount(t *testing.T) {
	results := []*validator.Result{
		{Valid: false, Issues: []validator.Issue{
			{Severity: "fatal"},
			{Severity: "error"},
		}},
	}
	total := TerminalSummary(results, "information")
	if total != 2 {
		t.Errorf("expected total=2, got %d", total)
	}
}

func TestTerminalSummary_WithSeverityFilter(t *testing.T) {
	results := []*validator.Result{
		{Valid: false, Issues: []validator.Issue{
			{Severity: "error"},
			{Severity: "information"},
		}},
	}
	total := TerminalSummary(results, "error")
	if total != 1 {
		t.Errorf("expected total=1 after error-only filter, got %d", total)
	}
}
