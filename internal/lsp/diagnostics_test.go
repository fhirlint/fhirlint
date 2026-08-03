package lsp

import (
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

func TestRangeFor_UsesReportedPosition(t *testing.T) {
	lines := []string{"{", `  "gender": "nope"`, "}"}

	got := rangeFor(lines, 2, 3)

	if got.Start.Line != 1 || got.Start.Character != 2 {
		t.Errorf("start = %+v, want line 1 char 2", got.Start)
	}
	if got.End.Character != len(lines[1]) {
		t.Errorf("end should reach the end of the line, got %d", got.End.Character)
	}
}

func TestRangeFor_ColumnPastEndOfLineMarksWholeLine(t *testing.T) {
	// The validator reports positions against the resource as it parsed it,
	// which need not line up with a reformatted buffer. An empty range at the
	// line end would be invisible and un-hoverable.
	lines := []string{"{", `  "id": "x"`, "}"}

	got := rangeFor(lines, 1, 33)

	if got.Start.Character != 0 {
		t.Errorf("start should fall back to the line start, got %d", got.Start.Character)
	}
	if got.End.Character != 1 {
		t.Errorf("end should be the line length, got %d", got.End.Character)
	}
	if !covers(got, position{Line: 0, Character: 0}) {
		t.Error("the resulting range must be hoverable at the line start")
	}
}

func TestRangeFor_MissingPositionAnchorsToStart(t *testing.T) {
	lines := []string{"{}", ""}

	got := rangeFor(lines, 0, 0)

	if got.Start.Line != 0 || got.Start.Character != 0 {
		t.Errorf("an issue without a position should anchor to the document start, got %+v", got.Start)
	}
}

func TestRangeFor_LineBeyondDocument(t *testing.T) {
	lines := []string{"{}"}

	got := rangeFor(lines, 99, 1)

	if got.Start.Line != 0 {
		t.Errorf("a line past the end should anchor to the start, got %+v", got.Start)
	}
}

func TestToDiagnostics_MapsSeverities(t *testing.T) {
	res := &validator.Result{Issues: []validator.Issue{
		{Severity: "fatal", Location: "A (line 1, col 1)"},
		{Severity: "error", Location: "A (line 1, col 1)"},
		{Severity: "warning", Location: "A (line 1, col 1)"},
		{Severity: "information", Location: "A (line 1, col 1)"},
	}}

	got := toDiagnostics(res, "{}\n")

	want := []int{severityError, severityError, severityWarning, severityInformation}
	if len(got) != len(want) {
		t.Fatalf("expected %d diagnostics, got %d", len(want), len(got))
	}
	for i, w := range want {
		if got[i].Severity != w {
			t.Errorf("diagnostic %d severity = %d, want %d", i, got[i].Severity, w)
		}
	}
}

func TestToDiagnostics_CarriesExpressionInData(t *testing.T) {
	res := &validator.Result{Issues: []validator.Issue{
		{Severity: "error", Location: "Patient.gender (line 1, col 1)", MessageID: "x"},
	}}

	got := toDiagnostics(res, "{}\n")

	if len(got) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(got))
	}
	data, ok := got[0].Data.(diagnosticData)
	if !ok {
		t.Fatalf("data has unexpected type %T", got[0].Data)
	}
	if data.Expression != "Patient.gender" {
		t.Errorf("expression = %q, want Patient.gender", data.Expression)
	}
}

func TestToDiagnostics_NilResult(t *testing.T) {
	if got := toDiagnostics(nil, ""); got != nil {
		t.Errorf("a nil result should produce no diagnostics, got %v", got)
	}
}

func TestCovers(t *testing.T) {
	r := textRange{Start: position{Line: 1, Character: 2}, End: position{Line: 1, Character: 10}}

	cases := []struct {
		pos  position
		want bool
	}{
		{position{Line: 1, Character: 2}, true},
		{position{Line: 1, Character: 10}, true},
		{position{Line: 1, Character: 1}, false},
		{position{Line: 1, Character: 11}, false},
		{position{Line: 0, Character: 5}, false},
		{position{Line: 2, Character: 5}, false},
	}
	for _, c := range cases {
		if got := covers(r, c.pos); got != c.want {
			t.Errorf("covers(%+v) = %v, want %v", c.pos, got, c.want)
		}
	}
}
