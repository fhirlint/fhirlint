package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

func writeProfile(t *testing.T, url string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "profile.json")
	body := `{"resourceType":"StructureDefinition","url":"` + url + `","type":"Patient"}`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResolveCompareSide_LocalFileReadsCanonical(t *testing.T) {
	path := writeProfile(t, "http://example.org/sd/demo")
	ig, canonical, err := resolveCompareSide(path, "", "left")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ig != path {
		t.Errorf("ig = %q, want the file path", ig)
	}
	if canonical != "http://example.org/sd/demo" {
		t.Errorf("canonical = %q, want it read from the file url", canonical)
	}
}

func TestResolveCompareSide_LocalFileProfileOverride(t *testing.T) {
	path := writeProfile(t, "http://example.org/sd/demo")
	_, canonical, err := resolveCompareSide(path, "http://override/sd", "left")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if canonical != "http://override/sd" {
		t.Errorf("canonical = %q, want the explicit override", canonical)
	}
}

func TestResolveCompareSide_PackageRequiresProfile(t *testing.T) {
	_, _, err := resolveCompareSide("kbv.basis#1.5.0", "", "left")
	if err == nil || !strings.Contains(err.Error(), "left-profile") {
		t.Errorf("err = %v, want a hint to pass --left-profile", err)
	}
}

func TestResolveCompareSide_PackageAliasResolves(t *testing.T) {
	ig, canonical, err := resolveCompareSide("kbv-basis", "KBV_PR_Base_Patient", "right")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ig != "kbv.basis#1.9.0" {
		t.Errorf("ig = %q, want the alias resolved to a package spec", ig)
	}
	if canonical != "KBV_PR_Base_Patient" {
		t.Errorf("canonical = %q, want the profile flag passed through", canonical)
	}
}

// An alias standing for several packages cannot name one profile to compare,
// so it is rejected rather than silently reduced to its first package (#334).
func TestResolveCompareSide_MultiPackageAliasRejected(t *testing.T) {
	_, _, err := resolveCompareSide("mii", "MII_PR_Person_Patient", "left")
	if err == nil {
		t.Fatal("err = nil, want a rejection for a multi-package alias")
	}
	for _, want := range []string{"several packages", "kerndatensatz.base"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %q, want it to mention %q", err, want)
		}
	}
}

func TestRunCompare_BadFormat(t *testing.T) {
	flagCompareFormat = "pdf"
	defer func() { flagCompareFormat = "terminal" }()
	err := runCompare(nil, nil)
	if err == nil || !strings.Contains(err.Error(), "unknown format") {
		t.Errorf("err = %v, want unknown-format error", err)
	}
}

func TestRunCompare_MissingSides(t *testing.T) {
	flagCompareFormat = "terminal"
	flagCompareLeft, flagCompareRight = "", ""
	err := runCompare(nil, nil)
	if err == nil || !strings.Contains(err.Error(), "both --left and --right") {
		t.Errorf("err = %v, want missing-side error", err)
	}
}

func TestPrintCompareTerminal_NoDifferences(t *testing.T) {
	out := captureStdout(t, func() {
		printCompareTerminal(&validator.CompareResult{Left: "a", Right: "b", Messages: []validator.CompareMessage{}})
	})
	if !strings.Contains(out, "no differences") {
		t.Errorf("expected equivalence line, got:\n%s", out)
	}
}

func TestPrintCompareTerminal_Differences(t *testing.T) {
	out := captureStdout(t, func() {
		printCompareTerminal(&validator.CompareResult{
			Left: "a", Right: "b",
			Messages: []validator.CompareMessage{
				{Severity: "error", Path: "Patient.birthDate", Message: "min 0→1"},
				{Severity: "information", Path: "Patient.name", Message: "cardinality"},
			},
		})
	})
	if !strings.Contains(out, "2 difference(s)") {
		t.Errorf("expected difference count, got:\n%s", out)
	}
	if !strings.Contains(out, "✗") || !strings.Contains(out, "Patient.birthDate") {
		t.Errorf("expected severity marker and path, got:\n%s", out)
	}
}

func TestPrintCompareJSON_ToFile(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "out.json")
	flagCompareOutput = dest
	defer func() { flagCompareOutput = "" }()
	_ = captureStdout(t, func() {
		_ = printCompareJSON(&validator.CompareResult{Left: "a", Right: "b", Messages: []validator.CompareMessage{}})
	})
	data, err := os.ReadFile(dest) //nolint:gosec // test reads its own temp file
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}
	if !strings.Contains(string(data), `"left": "a"`) {
		t.Errorf("file missing expected JSON, got:\n%s", data)
	}
}

func TestCompareDestDir_TempCleanup(t *testing.T) {
	flagCompareFormat = "terminal"
	dir, cleanup, err := compareDestDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("temp dir not created: %v", err)
	}
	cleanup()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("temp dir should be removed by cleanup")
	}
}

func TestCompareDestDir_HTMLDefault(t *testing.T) {
	flagCompareFormat = "html"
	flagCompareOutput = ""
	defer func() { flagCompareFormat = "terminal" }()
	dir, _, err := compareDestDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir != "fhirlint-compare" {
		t.Errorf("dir = %q, want default ./fhirlint-compare", dir)
	}
}
