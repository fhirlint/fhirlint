package diff

import (
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

func res(file string, issues ...validator.Issue) *validator.Result {
	return &validator.Result{Filename: file, Issues: issues}
}

func iss(sev, id, loc string) validator.Issue {
	return validator.Issue{Severity: sev, MessageID: id, Location: loc, Message: id + " msg"}
}

func TestCompute_Categorises(t *testing.T) {
	base := []*validator.Result{
		res("a.json", iss("warning", "dom-6", "A")),
		res("b.json", iss("error", "dom-4", "B")),
	}
	cur := []*validator.Result{
		res("b.json", iss("error", "dom-4", "B")),     // unchanged
		res("c.json", iss("error", "us-core-1", "C")), // new
	}
	d := Compute(base, cur, "information")

	if len(d.New) != 1 || d.New[0].MessageID != "us-core-1" {
		t.Errorf("expected 1 new (us-core-1), got %+v", d.New)
	}
	if len(d.Resolved) != 1 || d.Resolved[0].MessageID != "dom-6" {
		t.Errorf("expected 1 resolved (dom-6), got %+v", d.Resolved)
	}
	if len(d.Unchanged) != 1 || d.Unchanged[0].MessageID != "dom-4" {
		t.Errorf("expected 1 unchanged (dom-4), got %+v", d.Unchanged)
	}
}

func TestCompute_LocationNormalizedAcrossLineShift(t *testing.T) {
	base := []*validator.Result{res("a.json", iss("error", "dom-6", "Patient (line 5, col 2)"))}
	cur := []*validator.Result{res("a.json", iss("error", "dom-6", "Patient (line 9, col 2)"))}
	d := Compute(base, cur, "information")

	if len(d.New) != 0 || len(d.Resolved) != 0 {
		t.Errorf("line-only shift should be unchanged, got new=%d resolved=%d", len(d.New), len(d.Resolved))
	}
	if len(d.Unchanged) != 1 {
		t.Fatalf("expected 1 unchanged, got %d", len(d.Unchanged))
	}
	// The current run's location (line 9) is preserved for display.
	if d.Unchanged[0].Location != "Patient (line 9, col 2)" {
		t.Errorf("expected current location preserved, got %q", d.Unchanged[0].Location)
	}
}

func TestCompute_OccurrenceCounts(t *testing.T) {
	// Same key appears once in baseline, three times in current → 1 unchanged, 2 new.
	base := []*validator.Result{res("a.json", iss("warning", "dom-6", "A"))}
	cur := []*validator.Result{res("a.json",
		iss("warning", "dom-6", "A"),
		iss("warning", "dom-6", "A"),
		iss("warning", "dom-6", "A"),
	)}
	d := Compute(base, cur, "information")
	if len(d.Unchanged) != 1 {
		t.Errorf("expected 1 unchanged, got %d", len(d.Unchanged))
	}
	if len(d.New) != 2 {
		t.Errorf("expected 2 new (extra occurrences), got %d", len(d.New))
	}
}

func TestCompute_SeverityFilter(t *testing.T) {
	base := []*validator.Result{}
	cur := []*validator.Result{res("a.json",
		iss("error", "err", "A"),
		iss("warning", "warn", "B"),
		iss("information", "info", "C"),
	)}
	d := Compute(base, cur, "error")
	if len(d.New) != 1 || d.New[0].MessageID != "err" {
		t.Errorf("expected only the error to be considered new, got %+v", d.New)
	}
}

func TestCompute_EmptyInputsNonNilSlices(t *testing.T) {
	d := Compute(nil, nil, "information")
	if d.New == nil || d.Resolved == nil || d.Unchanged == nil {
		t.Error("slices should be non-nil so JSON marshals to [] not null")
	}
}
