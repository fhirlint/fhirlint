//go:build integration

package validator

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func testdataPath(parts ...string) string {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "testdata")
	return filepath.Join(append([]string{root}, parts...)...)
}

func TestIntegration_ValidPatient(t *testing.T) {
	result, err := Run(testdataPath("patient.json"), Options{
		FHIRVersion:         "4.0.1",
		NoTerminologyServer: true,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if !result.Valid {
		t.Errorf("expected valid patient to pass, got issues: %v", result.Issues)
	}
}

func TestIntegration_InvalidGender(t *testing.T) {
	result, err := Run(testdataPath("invalid", "bad-gender.json"), Options{
		FHIRVersion:         "4.0.1",
		NoTerminologyServer: true,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result.Valid {
		t.Error("expected invalid patient (bad gender) to fail validation")
	}
	if !containsMessageFragment(result, "gender") {
		t.Errorf("expected an issue mentioning 'gender', got: %v", result.Issues)
	}
}

func TestIntegration_IncompleteMedicationRequest(t *testing.T) {
	result, err := Run(testdataPath("invalid", "incomplete-medication-request.json"), Options{
		FHIRVersion:         "4.0.1",
		NoTerminologyServer: true,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result.Valid {
		t.Error("expected incomplete MedicationRequest to fail validation")
	}
	// MedicationRequest requires intent — should appear in the issues
	if !containsMessageFragment(result, "intent") && !containsMessageFragment(result, "medication") {
		t.Errorf("expected an issue mentioning 'intent' or 'medication', got: %v", result.Issues)
	}
}

func TestIntegration_FHIRVersionR4B(t *testing.T) {
	result, err := Run(testdataPath("patient.json"), Options{
		FHIRVersion:         "4.3.0",
		NoTerminologyServer: true,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result == nil {
		t.Fatal("Run() returned nil result")
	}
}

func TestIntegration_CustomTerminologyServerInvalid(t *testing.T) {
	// Using a non-existent TX server should not panic — validator falls back gracefully.
	_, err := Run(testdataPath("patient.json"), Options{
		FHIRVersion:       "4.0.1",
		TerminologyServer: "http://127.0.0.1:19999",
	})
	// Acceptable outcomes: error from JAR crash, or a result with issues — but no panic.
	_ = err
}

func containsMessageFragment(result *Result, fragment string) bool {
	fragment = strings.ToLower(fragment)
	for _, issue := range result.Issues {
		if strings.Contains(strings.ToLower(issue.Message), fragment) ||
			strings.Contains(strings.ToLower(issue.Location), fragment) {
			return true
		}
	}
	return false
}
