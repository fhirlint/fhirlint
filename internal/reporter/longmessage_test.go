// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package reporter

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

const omittedMarker = "characters omitted; full text with --format json"

// The measured case from #519: a 300,000-character gender code quoted three
// times gave a 900 KB message, repeated in every human-facing report.
func longMessageResult() *validator.Result {
	value := strings.Repeat("x", 300_000)
	return &validator.Result{
		Filename: "big.json",
		Label:    "big.json",
		Issues: []validator.Issue{{
			Severity:  "error",
			Message:   "The value provided ('" + value + "') was not found in '" + value + "' and '" + value + "'",
			Location:  "Patient.gender (line 2, col 13)",
			MessageID: "Terminology_TX_NoValid_16",
		}},
	}
}

// Every human-facing sink stays far below the size of the message and says
// where the full text is; the 300 KB value appears in none of them.
func TestLongMessage_CappedInHumanFacingReports(t *testing.T) {
	r := longMessageResult()
	results := []*validator.Result{r}

	htmlPath := filepath.Join(t.TempDir(), "r.html")
	if err := HTML(results, "information", RunInfo{}, htmlPath); err != nil {
		t.Fatal(err)
	}
	htmlOut, _ := os.ReadFile(htmlPath) //nolint:gosec // test reads its own temp file

	var gh bytes.Buffer
	_ = writeGitHub(&gh, results, "information")
	junit, _ := xml.Marshal(buildJUnitReport(results, "information", RunInfo{}))
	sarif, _ := json.Marshal(buildSARIFReport(results, "information", RunInfo{}))
	cc, _ := json.Marshal(buildCodeClimateReport(results, "information"))

	sinks := map[string]string{
		"terminal": captureStdout(t, func() { Terminal(r, "information", false, false, false) }),
		"grouped":  captureStdout(t, func() { TerminalGrouped(results, "information", false, false) }),
		"github":   gh.String(),
		"markdown": buildMarkdownReport(results, "information", RunInfo{}),
		"junit":    string(junit),
		"sarif":    string(sarif),
		"html":     string(htmlOut),
		"cc":       string(cc),
	}
	for name, out := range sinks {
		if len(out) > 20_000 {
			t.Errorf("%s: %d bytes for one issue", name, len(out))
		}
		if !strings.Contains(out, omittedMarker) {
			t.Errorf("%s: no omission marker", name)
		}
		if !strings.Contains(out, "The value provided (") {
			t.Errorf("%s: lost the start of the message", name)
		}
	}
}

// JSON is the machine-readable output the marker points at: it keeps the text.
func TestLongMessage_JSONKeepsFullText(t *testing.T) {
	r := longMessageResult()
	out, _ := json.Marshal(buildJSONReport([]*validator.Result{r}, "information"))
	if !strings.Contains(string(out), r.Issues[0].Message) || strings.Contains(string(out), omittedMarker) {
		t.Error("JSON report does not carry the exact message")
	}
}

// The grouped terminal report used to print the message, the label and the
// message id without escaping (#496).
func TestTerminalGrouped_EscapesControlCharacters(t *testing.T) {
	r := hostileResult(t)
	out := captureStdout(t, func() { TerminalGrouped([]*validator.Result{r}, "information", true, true) })
	noRawControl(t, "grouped", out)
}
