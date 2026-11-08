package reporter

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

func causeResult() *validator.Result {
	return &validator.Result{
		Filename: "p.json",
		Label:    "p.json",
		Issues: []validator.Issue{{
			Severity:  "error",
			Message:   "StructureDefinition http://x/SD/p|1.0 has no snapshot - validation is against the snapshot, so it must be provided",
			Location:  "Patient (line 1, col 2)",
			MessageID: "Validation_VAL_Profile_NoSnapshot",
			Cause:     "Attempt to a slice an element that does not repeat: Patient.maritalStatus \x1b[2J",
		}},
	}
}

// #521: the cause the validator logged on its console reaches every report.
func TestCause_InEveryReport(t *testing.T) {
	r := causeResult()
	results := []*validator.Result{r}
	var gh bytes.Buffer
	_ = writeGitHub(&gh, results, "information")
	sarif, _ := json.Marshal(buildSARIFReport(results, "information", RunInfo{}))
	cc, _ := json.Marshal(buildCodeClimateReport(results, "information"))
	js, _ := json.Marshal(buildJSONReport(results, "information"))

	for name, out := range map[string]string{
		"terminal": captureStdout(t, func() { Terminal(r, "information", false, false, false) }),
		"grouped":  captureStdout(t, func() { TerminalGrouped(results, "information", false, false) }),
		"github":   gh.String(),
		"markdown": buildMarkdownReport(results, "information", RunInfo{}),
		"sarif":    string(sarif),
		"cc":       string(cc),
		"json":     string(js),
	} {
		if !strings.Contains(out, "does not repeat: Patient.maritalStatus") {
			t.Errorf("%s: cause missing:\n%s", name, out)
		}
	}
	if !strings.Contains(string(js), `"cause":`) {
		t.Error("JSON report has no cause field")
	}
}

func TestCause_TerminalEscapesIt(t *testing.T) {
	out := captureStdout(t, func() { Terminal(causeResult(), "information", false, false, false) })
	if !strings.Contains(out, "↳ cause: Attempt") || strings.Contains(out, "\x1b[2J") {
		t.Errorf("terminal output:\n%q", out)
	}
}

func TestMessageWithCause(t *testing.T) {
	if got := MessageWithCause(validator.Issue{Message: "m"}); got != "m" {
		t.Errorf("no cause: %q", got)
	}
	if got := MessageWithCause(validator.Issue{Message: "m", Cause: "c"}); got != "m\nCause: c" {
		t.Errorf("with cause: %q", got)
	}
}
