package stats

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sample() *Report {
	r := Compute([]Resource{
		{Type: "Patient", Profiles: []string{"http://kbv/Patient"}},
		{Type: "Patient"},
		{Type: "Observation"},
	})
	r.Validation = &ValidationSummary{Files: 3, Valid: 2, Warnings: 4, Errors: 1}
	return r
}

func TestTerminal_Sections(t *testing.T) {
	out := Terminal(sample())
	for _, want := range []string{
		"Resource types",
		"Profiles declared",
		"http://kbv/Patient",
		"(none)",
		"Validation summary",
		"Files  3   Valid  2 (66%)   Warnings  4   Errors  1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("terminal output missing %q\n---\n%s", want, out)
		}
	}
}

func TestTerminal_NoValidationSection(t *testing.T) {
	r := Compute([]Resource{{Type: "Patient"}})
	out := Terminal(r)
	if strings.Contains(out, "Validation summary") {
		t.Errorf("validation section should be omitted when nil\n%s", out)
	}
}

func TestJSON_RoundTrips(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "stats.json")
	if err := JSON(sample(), dest); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest) //nolint:gosec // test reads its own temp file
	if err != nil {
		t.Fatal(err)
	}
	var r Report
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if r.TotalResources != 3 || r.Validation == nil || r.Validation.Valid != 2 {
		t.Errorf("round-tripped report mismatch: %+v", r)
	}
}
