// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package reporter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/diff"
)

func sampleDiff() *diff.Result {
	return &diff.Result{
		New: []diff.Issue{
			{File: "a.json", Severity: "error", MessageID: "E1", Location: "Patient.name", Message: "new error"},
			{File: "a.json", Severity: "warning", MessageID: "W1", Message: "new warning"},
			{File: "b.json", Severity: "fatal", MessageID: "F1", Message: "new fatal"},
			{File: "b.json", Severity: "information", Message: "new info"},
		},
		Resolved:  []diff.Issue{{File: "c.json", Severity: "error", MessageID: "R1"}},
		Unchanged: []diff.Issue{{File: "d.json", Severity: "warning", MessageID: "U1"}},
	}
}

func TestDiffTerminal_Sections(t *testing.T) {
	sides := DiffSides{Baseline: RunInfo{Validator: "6.10.4"}, Current: RunInfo{Validator: "7.0.5"}}
	out := captureStdout(t, func() { DiffTerminal(sampleDiff(), sides, false) })
	for _, want := range []string{
		"different validators — baseline 6.10.4, current 7.0.5",
		"New issues (4)", "✗ ERROR", "⚠ WARN", "✗ FATAL", "ℹ INFO",
		"a.json", "E1", "@ Patient.name", "—",
		"Resolved issues (1)", "R1",
		"Unchanged issues (1)", "(use --show-unchanged to list)",
		"4 new", "1 resolved", "1 unchanged",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "U1") {
		t.Errorf("unchanged issues are only counted without --show-unchanged:\n%s", out)
	}
	if out := captureStdout(t, func() { DiffTerminal(sampleDiff(), DiffSides{}, true) }); !strings.Contains(out, "U1") || strings.Contains(out, "different validators") {
		t.Errorf("--show-unchanged lists them, and unknown sides say nothing about validators:\n%s", out)
	}
	if out := captureStdout(t, func() { DiffTerminal(&diff.Result{}, DiffSides{}, false) }); strings.Count(out, "(none)") != 2 {
		t.Errorf("empty sections say (none):\n%s", out)
	}
}

func TestDiffJSON_Stdout(t *testing.T) {
	out := captureStdout(t, func() {
		if err := DiffJSON(sampleDiff(), DiffSides{}, ""); err != nil {
			t.Error(err)
		}
	})
	var got struct {
		Summary struct{ New, Resolved, Unchanged int }
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || got.Summary.New != 4 || got.Summary.Resolved != 1 || got.Summary.Unchanged != 1 {
		t.Errorf("summary: %+v (%v)\n%s", got, err, out)
	}
}

func TestDiffSARIF_OnlyNewIssues(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "d.sarif")
	if err := DiffSARIF(sampleDiff(), DiffSides{Current: RunInfo{Validator: "7.0.5"}}, dest); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest) //nolint:gosec // test temp dir
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{`"E1"`, `"W1"`, `"F1"`, "a.json", "b.json"} {
		if !strings.Contains(s, want) {
			t.Errorf("SARIF lacks %s", want)
		}
	}
	for _, not := range []string{`"R1"`, `"U1"`} {
		if strings.Contains(s, not) {
			t.Errorf("SARIF of a diff carries only new issues, found %s", not)
		}
	}
}
