package baseline_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fhirlint/fhirlint/internal/baseline"
	"github.com/fhirlint/fhirlint/internal/validator"
)

func makeResult(file string, issues ...validator.Issue) *validator.Result {
	return &validator.Result{Filename: file, Issues: issues, Valid: true}
}

func issue(severity, messageID, location string) validator.Issue {
	return validator.Issue{Severity: severity, MessageID: messageID, Location: location}
}

func TestGenerate_BasicEntries(t *testing.T) {
	results := []*validator.Result{
		makeResult("patient.json",
			issue("warning", "dom-6", "Patient"),
			issue("error", "req-1", "Patient.name (line 5, col 3)"),
		),
	}
	bf := baseline.Generate(results)
	if len(bf.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(bf.Entries))
	}
}

func TestGenerate_NormalizesLocation(t *testing.T) {
	results := []*validator.Result{
		makeResult("p.json", issue("warning", "dom-6", "Patient.name (line 5, col 3)")),
	}
	bf := baseline.Generate(results)
	if len(bf.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(bf.Entries))
	}
	if bf.Entries[0].Location != "Patient.name" {
		t.Errorf("expected normalized location, got %q", bf.Entries[0].Location)
	}
}

func TestGenerate_CountsDuplicates(t *testing.T) {
	results := []*validator.Result{
		makeResult("p.json",
			issue("warning", "dom-6", "Patient"),
			issue("warning", "dom-6", "Patient"),
			issue("warning", "dom-6", "Patient"),
		),
	}
	bf := baseline.Generate(results)
	if len(bf.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(bf.Entries))
	}
	if bf.Entries[0].Count != 3 {
		t.Errorf("expected count=3, got %d", bf.Entries[0].Count)
	}
}

func TestGenerate_EmptyResults(t *testing.T) {
	bf := baseline.Generate(nil)
	if len(bf.Entries) != 0 {
		t.Errorf("expected 0 entries for empty results, got %d", len(bf.Entries))
	}
}

func TestReadWrite_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fhirlint-baseline.json")

	original := &baseline.BaselineFile{
		Entries: []baseline.Entry{
			{File: "patient.json", MessageID: "dom-6", Location: "Patient", Count: 2},
		},
	}
	if err := baseline.Write(path, original); err != nil {
		t.Fatalf("Write: %v", err)
	}
	read, err := baseline.Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(read.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(read.Entries))
	}
	e := read.Entries[0]
	if e.File != "patient.json" || e.MessageID != "dom-6" || e.Location != "Patient" || e.Count != 2 {
		t.Errorf("round-trip mismatch: %+v", e)
	}
}

func TestRead_ReturnsNilForMissingFile(t *testing.T) {
	bf, err := baseline.Read("/nonexistent/fhirlint-baseline.json")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if bf != nil {
		t.Errorf("expected nil BaselineFile for missing file")
	}
}

func TestRead_ReturnsErrorForInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fhirlint-baseline.json")
	if err := os.WriteFile(path, []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := baseline.Read(path)
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestApply_SuppressesMatchingIssues(t *testing.T) {
	results := []*validator.Result{
		makeResult("patient.json",
			issue("warning", "dom-6", "Patient"),
			issue("error", "req-1", "Patient.name"),
		),
	}
	bf := &baseline.BaselineFile{
		Entries: []baseline.Entry{
			{File: "patient.json", MessageID: "dom-6", Location: "Patient", Count: 1},
		},
	}

	stale := baseline.Apply(results, bf)
	if stale != 0 {
		t.Errorf("expected 0 stale entries, got %d", stale)
	}
	if len(results[0].Issues) != 1 {
		t.Errorf("expected 1 active issue, got %d", len(results[0].Issues))
	}
	if results[0].Issues[0].MessageID != "req-1" {
		t.Errorf("wrong issue survived: %q", results[0].Issues[0].MessageID)
	}
	if len(results[0].Suppressed) != 1 || results[0].Suppressed[0].SuppressReason != "baseline" {
		t.Errorf("expected 1 baseline-suppressed issue, got suppressed=%v", results[0].Suppressed)
	}
}

func TestApply_ReturnsStaleCount(t *testing.T) {
	results := []*validator.Result{
		makeResult("patient.json", issue("warning", "req-1", "Patient.name")),
	}
	bf := &baseline.BaselineFile{
		Entries: []baseline.Entry{
			{File: "patient.json", MessageID: "dom-6", Location: "Patient", Count: 2},
		},
	}

	stale := baseline.Apply(results, bf)
	if stale != 2 {
		t.Errorf("expected stale=2, got %d", stale)
	}
}

func TestApply_NewIssuesNotSuppressed(t *testing.T) {
	results := []*validator.Result{
		makeResult("patient.json",
			issue("error", "new-issue", "Patient.id"),
		),
	}
	bf := &baseline.BaselineFile{
		Entries: []baseline.Entry{
			{File: "patient.json", MessageID: "dom-6", Location: "Patient", Count: 1},
		},
	}

	baseline.Apply(results, bf)
	if len(results[0].Issues) != 1 || results[0].Issues[0].MessageID != "new-issue" {
		t.Errorf("new issue should not be suppressed")
	}
}

func TestApply_CountRespected(t *testing.T) {
	results := []*validator.Result{
		makeResult("p.json",
			issue("warning", "dom-6", "Patient"),
			issue("warning", "dom-6", "Patient"),
			issue("warning", "dom-6", "Patient"),
		),
	}
	bf := &baseline.BaselineFile{
		Entries: []baseline.Entry{
			{File: "p.json", MessageID: "dom-6", Location: "Patient", Count: 2},
		},
	}

	stale := baseline.Apply(results, bf)
	if stale != 0 {
		t.Errorf("expected 0 stale, got %d", stale)
	}
	if len(results[0].Issues) != 1 {
		t.Errorf("expected 1 active issue (one above count), got %d", len(results[0].Issues))
	}
	if len(results[0].Suppressed) != 2 {
		t.Errorf("expected 2 suppressed issues, got %d", len(results[0].Suppressed))
	}
}

func TestApply_NormalizesLocationOnMatch(t *testing.T) {
	results := []*validator.Result{
		makeResult("p.json",
			issue("warning", "dom-6", "Patient.name (line 7, col 1)"),
		),
	}
	bf := &baseline.BaselineFile{
		Entries: []baseline.Entry{
			{File: "p.json", MessageID: "dom-6", Location: "Patient.name", Count: 1},
		},
	}

	stale := baseline.Apply(results, bf)
	if stale != 0 {
		t.Errorf("expected 0 stale, got %d", stale)
	}
	if len(results[0].Issues) != 0 {
		t.Errorf("issue with line/col suffix should match baseline entry without suffix")
	}
}

func TestApply_EmptyBaseline(t *testing.T) {
	results := []*validator.Result{
		makeResult("p.json", issue("error", "dom-6", "Patient")),
	}
	bf := &baseline.BaselineFile{}
	stale := baseline.Apply(results, bf)
	if stale != 0 {
		t.Errorf("expected 0 stale for empty baseline, got %d", stale)
	}
	if len(results[0].Issues) != 1 {
		t.Errorf("no issues should be suppressed with empty baseline")
	}
}
