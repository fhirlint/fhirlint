package validator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fixtureOO loads a testdata/fixtures/*.json file as an operationOutcome.
func fixtureOO(t *testing.T, name string) operationOutcome {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "fixtures", name)
	data, err := os.ReadFile(root) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	var oo operationOutcome
	if err := json.Unmarshal(data, &oo); err != nil {
		t.Fatalf("parsing fixture %s: %v", name, err)
	}
	return oo
}

func TestToResult_NoIssues_IsValid(t *testing.T) {
	oo := fixtureOO(t, "oo-no-issues.json")
	result := toResult(oo, "patient.json")

	if !result.Valid {
		t.Error("expected Valid=true for OperationOutcome with no issues")
	}
	if len(result.Issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(result.Issues))
	}
}

func TestToResult_WarningOnly_IsValid(t *testing.T) {
	oo := fixtureOO(t, "oo-warning.json")
	result := toResult(oo, "patient.json")

	if !result.Valid {
		t.Error("expected Valid=true for warning-only OperationOutcome")
	}
	if len(result.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(result.Issues))
	}
	if result.Issues[0].Severity != "warning" {
		t.Errorf("expected severity=warning, got %q", result.Issues[0].Severity)
	}
}

func TestToResult_Error_IsInvalid(t *testing.T) {
	oo := fixtureOO(t, "oo-error.json")
	result := toResult(oo, "patient.json")

	if result.Valid {
		t.Error("expected Valid=false for OperationOutcome with error")
	}
}

func TestToResult_Fatal_IsInvalid(t *testing.T) {
	oo := fixtureOO(t, "oo-fatal.json")
	result := toResult(oo, "patient.json")

	if result.Valid {
		t.Error("expected Valid=false for OperationOutcome with fatal severity")
	}
}

func TestToResult_MixedIssues_CountsCorrectly(t *testing.T) {
	oo := fixtureOO(t, "oo-error.json")
	result := toResult(oo, "patient.json")

	if len(result.Issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(result.Issues))
	}

	errors, warnings := 0, 0
	for _, i := range result.Issues {
		switch i.Severity {
		case "error":
			errors++
		case "warning":
			warnings++
		}
	}
	if errors != 1 || warnings != 1 {
		t.Errorf("expected 1 error + 1 warning, got %d errors + %d warnings", errors, warnings)
	}
}

func TestToResult_LineColFromExtension(t *testing.T) {
	oo := fixtureOO(t, "oo-warning.json")
	result := toResult(oo, "patient.json")

	if len(result.Issues) == 0 {
		t.Fatal("expected at least 1 issue")
	}
	loc := result.Issues[0].Location
	if loc == "" {
		t.Error("expected non-empty location")
	}
	// Should contain line and col info
	for _, want := range []string{"line 3", "col 12"} {
		if !containsStr(loc, want) {
			t.Errorf("location %q should contain %q", loc, want)
		}
	}
}

func TestToResult_MessageIDFromExtension(t *testing.T) {
	oo := fixtureOO(t, "oo-warning.json")
	result := toResult(oo, "patient.json")

	if result.Issues[0].MessageID != "dom-6" {
		t.Errorf("expected messageId=dom-6, got %q", result.Issues[0].MessageID)
	}
}

func TestToResult_MultipleExpressionsJoined(t *testing.T) {
	oo := fixtureOO(t, "oo-multi-expression.json")
	result := toResult(oo, "bundle.json")

	loc := result.Issues[0].Location
	if !containsStr(loc, "Bundle.entry[2].resource") {
		t.Errorf("location %q should contain first expression", loc)
	}
	if !containsStr(loc, "Observation") {
		t.Errorf("location %q should contain second expression", loc)
	}
}

func TestToResult_FilenamePreserved(t *testing.T) {
	oo := fixtureOO(t, "oo-no-issues.json")
	result := toResult(oo, "my/custom/path.json")

	if result.Filename != "my/custom/path.json" {
		t.Errorf("expected filename %q, got %q", "my/custom/path.json", result.Filename)
	}
}

func TestToResult_MessageTextFromDetails(t *testing.T) {
	oo := fixtureOO(t, "oo-error.json")
	result := toResult(oo, "patient.json")

	want := "The value 'unknown' is not valid for element Patient.gender"
	if result.Issues[0].Message != want {
		t.Errorf("expected message %q, got %q", want, result.Issues[0].Message)
	}
}

func TestOOMError_DetectsOutOfMemory(t *testing.T) {
	stderr := "Exception in thread \"main\" java.lang.OutOfMemoryError: Java heap space"
	err := oomError(stderr)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "JAVA_OPTS") {
		t.Errorf("expected actionable hint with JAVA_OPTS, got: %v", err)
	}
}

func TestOOMError_NilOnNormalStderr(t *testing.T) {
	err := oomError("some other warning from the JVM")
	if err != nil {
		t.Errorf("expected nil, got: %v", err)
	}
}

func TestOOMError_NilOnEmpty(t *testing.T) {
	if err := oomError(""); err != nil {
		t.Errorf("expected nil for empty stderr, got: %v", err)
	}
}

// TestRun_Integration runs the actual validator — skipped in short mode.
func TestRun_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test (requires Java + JAR download)")
	}
	_, file, _, _ := runtime.Caller(0)
	patient := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "patient.json")

	result, err := Run(patient, Options{FHIRVersion: "4.0.1", NoTerminologyServer: true})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result == nil {
		t.Fatal("Run() returned nil result")
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
