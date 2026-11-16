// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package reporter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

// The corners of the formatters (#691).

func corner691Results() []*validator.Result {
	return []*validator.Result{{
		Filename: "p.json", Label: "p.json",
		Issues: []validator.Issue{{Severity: "warning", OriginalSeverity: "error", Message: "re-levelled", Location: "Patient.name", MessageID: "X"}},
	}}
}

// Every file formatter prints to stdout without a destination and reports a
// destination it can't write instead of dropping the report.
func TestFormatters_StdoutAndUnwritableDest(t *testing.T) {
	results := corner691Results()
	bad := filepath.Join(t.TempDir(), "missing", "report")
	for name, run := range map[string]func(dest string) error{
		"json":        func(d string) error { return JSON(results, "information", RunInfo{}, d) },
		"junit":       func(d string) error { return JUnit(results, "information", RunInfo{}, d) },
		"sarif":       func(d string) error { return SARIF(results, "information", RunInfo{}, d) },
		"codeclimate": func(d string) error { return CodeClimate(results, "information", d) },
		"markdown":    func(d string) error { return Markdown(results, "information", RunInfo{}, d) },
		"html":        func(d string) error { return HTML(results, "information", RunInfo{}, d) },
	} {
		var err error
		out := captureStdout(t, func() { err = run("") })
		if err != nil || !strings.Contains(out, "re-levelled") {
			t.Errorf("%s to stdout: %v\n%.200s", name, err, out)
		}
		if err := run(bad); err == nil {
			t.Errorf("%s: an unwritable destination must be an error", name)
		}
	}
}

// Markdown names the level the validator reported when an override changed it.
func TestMarkdown_ToAFileNamesTheOriginalSeverity(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "r.md")
	if err := Markdown(corner691Results(), "information", RunInfo{}, dest); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest) //nolint:gosec // a path in the test temp dir
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "WARNING (reported as ERROR)") {
		t.Errorf("got:\n%s", data)
	}
}

// The terminal marks a cached result and a re-levelled finding, and the
// summary counts checks that didn't run however they are filtered.
func TestTerminal_CachedReLevelledAndSkippedChecks(t *testing.T) {
	DisableColors()
	r := corner691Results()[0]
	r.Cached = true
	out := captureStdout(t, func() { Terminal(r, "information", false, false, false) })
	for _, want := range []string{"↩ cached", "↕ reported as error"} {
		if !strings.Contains(out, want) {
			t.Errorf("terminal output lacks %q:\n%s", want, out)
		}
	}
	skipped := []*validator.Result{{Filename: "v.json", Issues: []validator.Issue{
		{Severity: "information", MessageID: validator.SkippedCheckMessageIDs[0]},
		{Severity: "information", MessageID: validator.SkippedCheckMessageIDs[3]},
	}}}
	out = captureStdout(t, func() { TerminalSummary(skipped, "error") })
	for _, want := range []string{"Checks skipped: 1", "Codes unchecked: 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary lacks %q:\n%s", want, out)
		}
	}
}

// A nil result in the list (a file that failed before a result existed) is
// skipped, not a crash.
func TestSlowest_SkipsNilResults(t *testing.T) {
	var b strings.Builder
	Slowest(&b, []*validator.Result{nil, {Label: "a.json", DurationMs: 1200}}, 3)
	if !strings.Contains(b.String(), "a.json") {
		t.Errorf("got %q", b.String())
	}
}
