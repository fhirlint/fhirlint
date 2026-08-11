package reporter

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

func makeResult(valid bool, issues ...validator.Issue) *validator.Result {
	return &validator.Result{
		Filename: "test.json",
		Label:    "test.json",
		Valid:    valid,
		Issues:   issues,
	}
}

func issue(severity, message, location string) validator.Issue {
	return validator.Issue{Severity: severity, Message: message, Location: location}
}


func TestBuildJSONReport_ValidNoIssues(t *testing.T) {
	r := makeResult(true)
	report := buildJSONReport([]*validator.Result{r}, "information")

	if !report.Valid {
		t.Error("expected report.Valid=true")
	}
	if report.Summary.Total != 0 {
		t.Errorf("expected 0 total issues, got %d", report.Summary.Total)
	}
}

func TestBuildJSONReport_SingleError(t *testing.T) {
	r := makeResult(false, issue("error", "bad value", "Patient.gender"))
	report := buildJSONReport([]*validator.Result{r}, "information")

	if report.Valid {
		t.Error("expected report.Valid=false")
	}
	if report.Summary.Errors != 1 {
		t.Errorf("expected 1 error, got %d", report.Summary.Errors)
	}
	if report.Summary.Warnings != 0 {
		t.Errorf("expected 0 warnings, got %d", report.Summary.Warnings)
	}
}

func TestBuildJSONReport_SeverityFilter_HidesWarnings(t *testing.T) {
	r := makeResult(true,
		issue("warning", "missing narrative", "Patient"),
		issue("information", "best practice", "Patient"),
	)
	report := buildJSONReport([]*validator.Result{r}, "error")

	if report.Summary.Total != 0 {
		t.Errorf("expected 0 issues after error-only filter, got %d", report.Summary.Total)
	}
	if len(report.Files[0].Issues) != 0 {
		t.Errorf("expected 0 issues in file after filter, got %d", len(report.Files[0].Issues))
	}
}

func TestBuildJSONReport_SeverityFilter_ShowsWarningsAndAbove(t *testing.T) {
	r := makeResult(false,
		issue("error", "bad value", "Patient.gender"),
		issue("warning", "missing narrative", "Patient"),
		issue("information", "best practice", "Patient"),
	)
	report := buildJSONReport([]*validator.Result{r}, "warning")

	if report.Summary.Total != 2 {
		t.Errorf("expected 2 issues (error+warning), got %d", report.Summary.Total)
	}
	if report.Summary.Info != 0 {
		t.Errorf("expected info filtered out, got %d info", report.Summary.Info)
	}
}

func TestBuildJSONReport_MultipleFiles_Aggregation(t *testing.T) {
	r1 := makeResult(false, issue("error", "err1", "A"))
	r2 := makeResult(true, issue("warning", "warn1", "B"))
	report := buildJSONReport([]*validator.Result{r1, r2}, "information")

	if report.Valid {
		t.Error("expected report.Valid=false when any file has errors")
	}
	if len(report.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(report.Files))
	}
	if report.Summary.Errors != 1 || report.Summary.Warnings != 1 {
		t.Errorf("expected 1 error + 1 warning, got %d + %d",
			report.Summary.Errors, report.Summary.Warnings)
	}
}

func TestBuildJSONReport_AllValidFiles_ReportValid(t *testing.T) {
	r1 := makeResult(true)
	r2 := makeResult(true, issue("warning", "minor", "X"))
	report := buildJSONReport([]*validator.Result{r1, r2}, "information")

	if !report.Valid {
		t.Error("expected report.Valid=true when all files are valid")
	}
}

func TestBuildJSONReport_SummaryCountsAllSeverities(t *testing.T) {
	r := makeResult(false,
		issue("error", "e1", ""),
		issue("error", "e2", ""),
		issue("warning", "w1", ""),
		issue("information", "i1", ""),
		issue("information", "i2", ""),
		issue("information", "i3", ""),
	)
	report := buildJSONReport([]*validator.Result{r}, "information")

	if report.Summary.Total != 6 {
		t.Errorf("expected total=6, got %d", report.Summary.Total)
	}
	if report.Summary.Errors != 2 {
		t.Errorf("expected errors=2, got %d", report.Summary.Errors)
	}
	if report.Summary.Warnings != 1 {
		t.Errorf("expected warnings=1, got %d", report.Summary.Warnings)
	}
	if report.Summary.Info != 3 {
		t.Errorf("expected info=3, got %d", report.Summary.Info)
	}
}

func TestJSON_OutputIsValidJSON(t *testing.T) {
	r := makeResult(true, issue("warning", "msg", "loc"))
	report := buildJSONReport([]*validator.Result{r}, "information")
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent failed: %v", err)
	}
	var parsed JSONReport
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Errorf("output is not valid JSON: %v", err)
	}
}

// The JSON report carries both levels so a reader without the config can still
// tell what the validator actually said (#311).
func TestBuildJSONReport_CarriesOriginalSeverity(t *testing.T) {
	downgraded := issue("warning", "bundle entry has no fullUrl", "Bundle.entry[0]")
	downgraded.OriginalSeverity = "error"
	report := buildJSONReport([]*validator.Result{makeResult(true, downgraded)}, "information")

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"originalSeverity":"error"`) {
		t.Errorf("expected originalSeverity in output, got: %s", data)
	}
	// It counts as its effective severity, not its original one.
	if report.Summary.Warnings != 1 || report.Summary.Errors != 0 {
		t.Errorf("summary = %+v, want 1 warning and 0 errors", report.Summary)
	}
}
