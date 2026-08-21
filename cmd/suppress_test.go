package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

// loadTestConfig points viper at a fhirlint.yml written for the test.
func loadTestConfig(t *testing.T, content string) string {
	t.Helper()
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, content)
	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig: %v", err)
	}
	return dir
}

// exportAdvisor runs the export into a temp file and returns the entries.
func exportAdvisor(t *testing.T, dir string) []string {
	t.Helper()
	out := filepath.Join(dir, "advisor.json")

	flagSuppressExportOut = out
	t.Cleanup(func() { flagSuppressExportOut = "" })

	if err := runSuppressExport(suppressExportCmd, nil); err != nil {
		t.Fatalf("suppress export: %v", err)
	}

	data, err := os.ReadFile(out) //nolint:gosec // path built by the test
	if err != nil {
		t.Fatalf("reading advisor file: %v", err)
	}
	var file struct {
		Suppress []string `json:"suppress"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("parsing advisor file %s: %v", data, err)
	}
	return file.Suppress
}

func TestSuppressExport_WritesEntriesFromConfig(t *testing.T) {
	dir := loadTestConfig(t, `
suppress:
  - messageId:Type_Specific_Checks_DT_URL_Resolve
  - expression:Patient.name
`)

	got := exportAdvisor(t, dir)
	want := []string{"Type_Specific_Checks_DT_URL_Resolve", "*@Patient.name", "*@Patient.name.*"}
	if len(got) != len(want) {
		t.Fatalf("entries = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// The map form carries reason and expires, neither of which the advisor format
// has anywhere to put. The rule must still export — losing the annotation is
// expected, losing the rule would not be.
func TestSuppressExport_MapFormExportsWithoutAnnotations(t *testing.T) {
	dir := loadTestConfig(t, `
suppress:
  - messageId: SOME_ID
    reason: "known upstream defect"
    expires: 2099-01-01
`)

	got := exportAdvisor(t, dir)
	if len(got) != 1 || got[0] != "SOME_ID" {
		t.Errorf("entries = %q, want [SOME_ID]", got)
	}
}

// Rules the format cannot express must be absent from the file, not
// approximated into something that suppresses the wrong thing.
func TestSuppressExport_DropsUntranslatableRules(t *testing.T) {
	dir := loadTestConfig(t, `
suppress:
  - constraint:dom-6
  - pattern:.*example\.org.*
  - messageId:KEPT_ID
`)

	got := exportAdvisor(t, dir)
	if len(got) != 1 || got[0] != "KEPT_ID" {
		t.Errorf("entries = %q, want only [KEPT_ID]", got)
	}
}

// No config at all is a valid, empty advisor file rather than an error: a
// project can adopt the export before it has any suppressions.
func TestSuppressExport_NoRulesWritesEmptyFile(t *testing.T) {
	dir := loadTestConfig(t, "severity: warning\n")

	if got := exportAdvisor(t, dir); len(got) != 0 {
		t.Errorf("entries = %q, want none", got)
	}
}

func TestSuppressExport_StrictFailsWhenSomethingWasDropped(t *testing.T) {
	dir := loadTestConfig(t, "suppress:\n  - constraint:dom-6\n")

	flagSuppressExportOut = filepath.Join(dir, "advisor.json")
	flagSuppressExportStrict = true
	t.Cleanup(func() {
		flagSuppressExportOut = ""
		flagSuppressExportStrict = false
	})

	err := runSuppressExport(suppressExportCmd, nil)
	if err == nil {
		t.Fatal("err = nil, want a non-zero exit under --strict")
	}
	// The file is still written: --strict reports that the export is
	// incomplete, it does not refuse to produce it.
	if _, statErr := os.Stat(flagSuppressExportOut); statErr != nil {
		t.Errorf("advisor file was not written under --strict: %v", statErr)
	}
}

// Per-file overrides have no advisor equivalent — an advisor file applies to
// the whole run — so they must not leak into the export as global rules.
func TestSuppressExport_OverrideRulesAreNotExported(t *testing.T) {
	dir := loadTestConfig(t, `
suppress:
  - messageId:GLOBAL_ID
overrides:
  - files: ["examples/**"]
    suppress:
      - messageId:SCOPED_ID
`)

	got := exportAdvisor(t, dir)
	if len(got) != 1 || got[0] != "GLOBAL_ID" {
		t.Errorf("entries = %q, want only the global rule", got)
	}

	n, err := overrideSuppressCount()
	if err != nil {
		t.Fatalf("overrideSuppressCount: %v", err)
	}
	if n != 1 {
		t.Errorf("override rule count = %d, want 1 so the summary can report it", n)
	}
}

func TestSuppressExport_SeverityOverridesAreCountedNotExported(t *testing.T) {
	dir := loadTestConfig(t, `
severity-override:
  - messageId: SOME_ID
    severity: warning
`)

	if got := exportAdvisor(t, dir); len(got) != 0 {
		t.Errorf("entries = %q, want none — a severity override is not a suppression", got)
	}

	n, err := severityOverrideCount()
	if err != nil {
		t.Fatalf("severityOverrideCount: %v", err)
	}
	if n != 1 {
		t.Errorf("severity-override count = %d, want 1 so the summary can report it", n)
	}
}
