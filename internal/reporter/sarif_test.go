package reporter

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

func TestBuildSARIFReport_EmptyResults(t *testing.T) {
	report := buildSARIFReport(nil, "information", "1.0.0")
	if report.Version != sarifVersion {
		t.Errorf("expected version=%q, got %q", sarifVersion, report.Version)
	}
	if len(report.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(report.Runs))
	}
	if len(report.Runs[0].Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(report.Runs[0].Results))
	}
}

func TestBuildSARIFReport_ToolVersion(t *testing.T) {
	report := buildSARIFReport(nil, "information", "2.3.4")
	driver := report.Runs[0].Tool.Driver
	if driver.Version != "2.3.4" {
		t.Errorf("expected driver version=2.3.4, got %q", driver.Version)
	}
	if driver.Name != "fhirlint" {
		t.Errorf("expected driver name=fhirlint, got %q", driver.Name)
	}
}

func TestBuildSARIFReport_LevelMapping(t *testing.T) {
	r := makeResult(false,
		issue("error", "err msg", "Patient.gender"),
		issue("warning", "warn msg", "Patient"),
		issue("information", "info msg", ""),
	)
	report := buildSARIFReport([]*validator.Result{r}, "information", "0.0.1")

	levels := make(map[string]int)
	for _, res := range report.Runs[0].Results {
		levels[res.Level]++
	}
	if levels["error"] != 1 {
		t.Errorf("expected 1 error-level result, got %d", levels["error"])
	}
	if levels["warning"] != 1 {
		t.Errorf("expected 1 warning-level result, got %d", levels["warning"])
	}
	if levels["note"] != 1 {
		t.Errorf("expected 1 note-level result, got %d", levels["note"])
	}
}

func TestBuildSARIFReport_SeverityFilter(t *testing.T) {
	r := makeResult(false,
		issue("error", "e", ""),
		issue("warning", "w", ""),
		issue("information", "i", ""),
	)
	report := buildSARIFReport([]*validator.Result{r}, "warning", "0.0.1")

	if len(report.Runs[0].Results) != 2 {
		t.Errorf("expected 2 results after filter, got %d", len(report.Runs[0].Results))
	}
}

func TestBuildSARIFReport_RuleIDFromMessageID(t *testing.T) {
	iss := validator.Issue{Severity: "error", Message: "msg", MessageID: "dom-6"}
	r := &validator.Result{Filename: "f.json", Valid: false, Issues: []validator.Issue{iss}}
	report := buildSARIFReport([]*validator.Result{r}, "information", "0.0.1")

	res := report.Runs[0].Results[0]
	if res.RuleID != "dom-6" {
		t.Errorf("expected ruleId=dom-6, got %q", res.RuleID)
	}
	rules := report.Runs[0].Tool.Driver.Rules
	if len(rules) != 1 || rules[0].ID != "dom-6" {
		t.Errorf("expected 1 rule dom-6, got %v", rules)
	}
}

func TestBuildSARIFReport_FallbackRuleID(t *testing.T) {
	r := makeResult(false, issue("error", "msg", ""))
	report := buildSARIFReport([]*validator.Result{r}, "information", "0.0.1")

	res := report.Runs[0].Results[0]
	if res.RuleID != sarifDefaultRule {
		t.Errorf("expected ruleId=%q, got %q", sarifDefaultRule, res.RuleID)
	}
}

func TestBuildSARIFReport_DeduplicatesRules(t *testing.T) {
	iss1 := validator.Issue{Severity: "error", Message: "a", MessageID: "dom-6"}
	iss2 := validator.Issue{Severity: "warning", Message: "b", MessageID: "dom-6"}
	r := &validator.Result{Filename: "f.json", Valid: false, Issues: []validator.Issue{iss1, iss2}}
	report := buildSARIFReport([]*validator.Result{r}, "information", "0.0.1")

	if len(report.Runs[0].Tool.Driver.Rules) != 1 {
		t.Errorf("expected 1 unique rule, got %d", len(report.Runs[0].Tool.Driver.Rules))
	}
}

func TestBuildSARIFReport_LocationWithLineCol(t *testing.T) {
	iss := validator.Issue{
		Severity: "error",
		Message:  "bad",
		Location: "Patient.gender (line 5, col 12)",
	}
	r := &validator.Result{Filename: "patient.json", Valid: false, Issues: []validator.Issue{iss}}
	report := buildSARIFReport([]*validator.Result{r}, "information", "0.0.1")

	res := report.Runs[0].Results[0]
	if len(res.Locations) == 0 {
		t.Fatal("expected location in result")
	}
	region := res.Locations[0].PhysicalLocation.Region
	if region == nil {
		t.Fatal("expected non-nil region")
	}
	if region.StartLine != 5 {
		t.Errorf("expected StartLine=5, got %d", region.StartLine)
	}
	if region.StartColumn != 12 {
		t.Errorf("expected StartColumn=12, got %d", region.StartColumn)
	}
	logicals := res.Locations[0].LogicalLocations
	if len(logicals) == 0 || logicals[0].Name != "Patient.gender" {
		t.Errorf("expected logical location Patient.gender, got %v", logicals)
	}
}

func TestBuildSARIFReport_NoLocationWhenFilenameEmpty(t *testing.T) {
	r := &validator.Result{Filename: "", Valid: false, Issues: []validator.Issue{
		{Severity: "error", Message: "bad"},
	}}
	report := buildSARIFReport([]*validator.Result{r}, "information", "0.0.1")

	res := report.Runs[0].Results[0]
	if len(res.Locations) != 0 {
		t.Errorf("expected no locations when filename is empty, got %d", len(res.Locations))
	}
}

func TestParseLocationString_WithLineCol(t *testing.T) {
	expr, line, col := parseLocationString("Patient.gender (line 3, col 12)")
	if expr != "Patient.gender" {
		t.Errorf("expression: got %q", expr)
	}
	if line != 3 {
		t.Errorf("line: got %d", line)
	}
	if col != 12 {
		t.Errorf("col: got %d", col)
	}
}

func TestParseLocationString_NoLineCol(t *testing.T) {
	expr, line, col := parseLocationString("Patient.gender")
	if expr != "Patient.gender" {
		t.Errorf("expression: got %q", expr)
	}
	if line != 0 || col != 0 {
		t.Errorf("expected line=0 col=0, got %d %d", line, col)
	}
}

func TestParseLocationString_Empty(t *testing.T) {
	expr, line, col := parseLocationString("")
	if expr != "" || line != 0 || col != 0 {
		t.Errorf("unexpected: expr=%q line=%d col=%d", expr, line, col)
	}
}

func TestSarifLevel_Mapping(t *testing.T) {
	cases := map[string]string{
		"error":       "error",
		"fatal":       "error",
		"warning":     "warning",
		"information": "note",
		"":            "note",
	}
	for input, want := range cases {
		got := sarifLevel(input)
		if got != want {
			t.Errorf("sarifLevel(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestBuildSARIFReport_ValidJSON(t *testing.T) {
	r := makeResult(false, issue("error", "bad value", "Patient.gender"))
	report := buildSARIFReport([]*validator.Result{r}, "information", "1.0.0")

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent failed: %v", err)
	}
	if !strings.Contains(string(data), `"version": "2.1.0"`) {
		t.Errorf("expected SARIF version in output")
	}
	var roundtrip sarifReport
	if err := json.Unmarshal(data, &roundtrip); err != nil {
		t.Errorf("output is not valid JSON: %v", err)
	}
}

// A re-levelled finding must not be indistinguishable from one the validator
// reported at that level — SARIF has no field for it, so it travels in
// properties (#311).
func TestBuildSARIFReport_OriginalSeverityInProperties(t *testing.T) {
	downgraded := issue("warning", "bundle entry has no fullUrl", "Bundle.entry[0]")
	downgraded.OriginalSeverity = "error"
	r := makeResult(true, downgraded, issue("warning", "plain warning", "Patient"))

	report := buildSARIFReport([]*validator.Result{r}, "information", "1.0.0")
	results := report.Runs[0].Results
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Level != "warning" {
		t.Errorf("level = %q, want the effective severity (warning)", results[0].Level)
	}
	if results[0].Properties == nil || results[0].Properties.OriginalSeverity != "error" {
		t.Errorf("properties = %+v, want originalSeverity=error", results[0].Properties)
	}
	if results[1].Properties != nil {
		t.Errorf("untouched finding should carry no properties, got %+v", results[1].Properties)
	}
}

func TestSARIF_OriginalSeverityOmittedWhenUnchanged(t *testing.T) {
	report := buildSARIFReport(
		[]*validator.Result{makeResult(false, issue("error", "msg", "Patient"))},
		"information", "1.0.0")
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "originalSeverity") {
		t.Errorf("unchanged findings must not emit originalSeverity: %s", data)
	}
}
