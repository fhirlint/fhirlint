package reporter

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

// update regenerates the golden reports: go test ./internal/reporter -update
var update = flag.Bool("update", false, "rewrite the golden reports in testdata/golden")

// goldenResults exercises every field the machine-readable reports carry
// (#542): each severity, a finding with and without line/column, an inferred
// message ID, a suppressed issue, a Cause, a Hint, a message over the
// 2,000-character cap, a path with spaces and non-ASCII characters, and a file
// the validator crashed on. Keep it fixed: the golden files and the schema
// check in CI (lint.yml, "Report schemas") are built from it.
func goldenResults() []*validator.Result {
	long := "Long message start " + strings.Repeat("x", 2100) + " end"
	return []*validator.Result{
		{
			Filename: "examples/Patient-ok.json",
			Label:    "examples/Patient-ok.json",
			Valid:    true,
			Issues:   []validator.Issue{},
			Cached:   true,
		},
		{
			Filename:   "examples/Observation-bad.json",
			Label:      "examples/Observation-bad.json",
			Valid:      false,
			DurationMs: 1234,
			AppliedProfiles: []validator.AppliedProfile{
				{Profile: "http://example.org/StructureDefinition/obs", Reason: "profile-map: Observation when code.coding.exists()"},
			},
			Issues: []validator.Issue{
				{Severity: "fatal", Message: "Unable to parse JSON", Location: "Observation (line 1, col 1)", MessageID: "Validation_VAL_Unknown"},
				{Severity: "error", Message: "Observation.status: minimum required = 1, but only found 0", Location: "Observation (line 3, col 4)", MessageID: "Validation_VAL_Profile_Minimum"},
				{Severity: "warning", Message: "Best practice: dom-6", Location: "Observation", MessageID: "BP_DOM_6"},
				{Severity: "information", Message: "Code 'xyz' not found in value set", Location: "Observation.code.coding[0] (line 12, col 8)", MessageID: "Terminology_TX_NoValid_1_CC", MessageIDInferred: true},
				{Severity: "warning", Message: "Severity lowered by config", Location: "Observation.subject (line 20, col 6)", MessageID: "Reference_REF_CantResolve", OriginalSeverity: "error"},
				{Severity: "error", Message: long, Location: "Observation.note[0].text (line 30, col 10)", MessageID: "Type_Specific_Checks_DT_String_Length"},
			},
			Suppressed: []validator.Issue{
				{Severity: "warning", Message: "A suppressed finding", Location: "Observation.meta", MessageID: "MSG_DRAFT", SuppressReason: "draft profiles are fine here"},
			},
		},
		{
			Filename: "examples/Ärzte Liste/Practitioner 1.json",
			Label:    "examples/Ärzte Liste/Practitioner 1.json",
			Valid:    false,
			Issues: []validator.Issue{
				{Severity: "error", Message: "Profile http://example.org/StructureDefinition/p has no snapshot", Location: "Practitioner (line 2, col 2)", MessageID: "VALIDATION_VAL_PROFILE_NOSNAPSHOT", Cause: "Unable to generate snapshot: base not found"},
				{Severity: "error", Message: "Unable to resolve profile https://fhir.kbv.de/StructureDefinition/KBV_PR_Base_Practitioner", Location: "Practitioner.meta.profile[0] (line 5, col 9)", MessageID: "VALIDATION_VAL_PROFILE_UNKNOWN",
					Hint: &validator.PackageHint{Canonical: "https://fhir.kbv.de/StructureDefinition/KBV_PR_Base_Practitioner", Package: "kbv.basis#1.7.0", Packages: []string{"kbv.basis#1.7.0"}, Alias: "kbv-basis"}},
			},
		},
		{
			Filename: "examples/crash.xml",
			Label:    "examples/crash.xml",
			Valid:    false,
			Issues: []validator.Issue{
				{Severity: "fatal", Message: "the validator crashed on this file: java.lang.NullPointerException", MessageID: validator.MessageIDValidatorCrash},
			},
		},
	}
}

func goldenInfo() RunInfo {
	return RunInfo{
		Fhirlint:         "1.2.3",
		Validator:        "6.10.4",
		ValidatorBuild:   "FHIR Validation tool Version 6.10.4 (Git# 0123456789ab). Built 2026-10-01T00:00:00.000Z",
		FHIRVersion:      "4.0.1",
		ImplicitPackages: []validator.PackageRef{{ID: "hl7.terminology.r4", Version: "6.2.0"}},
	}
}

// TestGoldenReports fails when a reporter's output differs from its golden
// file. A deliberate change is committed with -update; the schema check in
// CI then validates the new file.
func TestGoldenReports(t *testing.T) {
	writers := []struct {
		file  string
		write func(dest string) error
	}{
		{"report.json", func(d string) error { return JSON(goldenResults(), "information", goldenInfo(), d) }},
		{"report.sarif", func(d string) error { return SARIF(goldenResults(), "information", goldenInfo(), d) }},
		{"report.codeclimate.json", func(d string) error { return CodeClimate(goldenResults(), "information", d) }},
		{"report.junit.xml", func(d string) error { return JUnit(goldenResults(), "information", goldenInfo(), d) }},
	}
	for _, w := range writers {
		t.Run(w.file, func(t *testing.T) {
			tmp := filepath.Join(t.TempDir(), w.file)
			if err := w.write(tmp); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(tmp) //nolint:gosec // G304: a file the test just wrote
			if err != nil {
				t.Fatal(err)
			}
			golden := filepath.Join("testdata", "golden", w.file)
			if *update {
				if err := os.MkdirAll(filepath.Dir(golden), 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, got, 0o600); err != nil { //nolint:gosec // G703: fixed file name from the test table under testdata
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden) //nolint:gosec // G304: fixed path under testdata
			if err != nil {
				t.Fatalf("%v (run go test ./internal/reporter -update)", err)
			}
			if !bytes.Equal(normalizeEOL(got), normalizeEOL(want)) {
				t.Errorf("%s differs from %s; if the change is intended, run go test ./internal/reporter -update and commit it", w.file, golden)
			}
		})
	}
}

// normalizeEOL lets a Windows checkout with autocrlf compare equal.
func normalizeEOL(b []byte) []byte { return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")) }
