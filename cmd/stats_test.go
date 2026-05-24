package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func resetStatsFlags() {
	flagStatsFormat = []string{"terminal"}
	flagStatsOutput = ""
	flagStatsNoValidate = true // keep tests offline (no JAR/Java)
	flagStatsFHIRVersion = defaultFHIRVersion
	flagStatsExclude = nil
}

func TestRunStats_StructuralTerminal(t *testing.T) {
	resetStatsFlags()
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.json"), `{"resourceType":"Patient","meta":{"profile":["http://p"]}}`)
	mustWrite(t, filepath.Join(dir, "b.json"), `{"resourceType":"Patient"}`)
	mustWrite(t, filepath.Join(dir, "c.json"), `{"resourceType":"Observation"}`)

	var err error
	out := captureStdout(t, func() { err = runStats(nil, []string{dir}) })
	if err != nil {
		t.Fatalf("runStats returned error: %v", err)
	}
	if !strings.Contains(out, "Patient") || !strings.Contains(out, "Observation") {
		t.Errorf("expected resource types in output:\n%s", out)
	}
	if !strings.Contains(out, "http://p") || !strings.Contains(out, "(none)") {
		t.Errorf("expected profiles section in output:\n%s", out)
	}
	if strings.Contains(out, "Validation summary") {
		t.Errorf("--no-validate should omit the validation summary:\n%s", out)
	}
}

func TestRunStats_RespectsExclude(t *testing.T) {
	resetStatsFlags()
	flagStatsFormat = []string{"json"}
	flagStatsExclude = []string{"skip/"}
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "keep.json"), `{"resourceType":"Patient"}`)
	_ = os.MkdirAll(filepath.Join(dir, "skip"), 0750)
	mustWrite(t, filepath.Join(dir, "skip", "ignored.json"), `{"resourceType":"Observation"}`)

	var err error
	out := captureStdout(t, func() { err = runStats(nil, []string{dir}) })
	if err != nil {
		t.Fatalf("runStats returned error: %v", err)
	}
	if strings.Contains(out, "Observation") {
		t.Errorf("excluded dir should not be counted:\n%s", out)
	}
	if !strings.Contains(out, "Patient") {
		t.Errorf("kept file should be counted:\n%s", out)
	}
}

func TestRunStats_NoFilesErrors(t *testing.T) {
	resetStatsFlags()
	err := runStats(nil, []string{t.TempDir()})
	var ee *exitErr
	if err == nil {
		t.Fatal("expected an error for an empty directory")
	}
	if !errors.As(err, &ee) || ee.code != 2 {
		t.Errorf("expected exitErr code 2, got %v", err)
	}
}

func TestStatsOutputFile(t *testing.T) {
	resetStatsFlags()
	flagStatsOutput = ""
	if got := statsOutputFile("json"); got != "" {
		t.Errorf("empty output should stay empty, got %q", got)
	}
	flagStatsOutput = "out.json"
	flagStatsFormat = []string{"json"}
	if got := statsOutputFile("json"); got != "out.json" {
		t.Errorf("single format keeps name, got %q", got)
	}
	// Multiple formats: append the extension only when it isn't already present.
	flagStatsOutput = "report"
	flagStatsFormat = []string{"terminal", "json"}
	if got := statsOutputFile("json"); got != "report.json" {
		t.Errorf("multiple formats append ext, got %q", got)
	}
	flagStatsOutput = "report.json"
	if got := statsOutputFile("json"); got != "report.json" {
		t.Errorf("should not double-append when ext already present, got %q", got)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}
