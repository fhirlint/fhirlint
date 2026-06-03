//go:build integration

package validator

import (
	"strings"
	"testing"
)

func TestIntegration_FHIRPath_Navigation(t *testing.T) {
	r, err := RunFHIRPath("Patient.gender", testdataPath("patient.json"), FHIRPathOptions{FHIRVersion: "4.0.1"})
	if err != nil {
		t.Fatalf("RunFHIRPath() error: %v", err)
	}
	if r.Empty() {
		t.Fatal("expected a gender value, got empty result")
	}
}

func TestIntegration_FHIRPath_Boolean(t *testing.T) {
	r, err := RunFHIRPath("Patient.exists()", testdataPath("patient.json"), FHIRPathOptions{FHIRVersion: "4.0.1"})
	if err != nil {
		t.Fatalf("RunFHIRPath() error: %v", err)
	}
	if len(r.Items) != 1 || r.Items[0] != "true" {
		t.Errorf("got %q, want [true]", r.Items)
	}
}

func TestIntegration_FHIRPath_EmptyResult(t *testing.T) {
	r, err := RunFHIRPath("Patient.name.where(use='nonexistent')", testdataPath("patient.json"), FHIRPathOptions{FHIRVersion: "4.0.1"})
	if err != nil {
		t.Fatalf("RunFHIRPath() error: %v", err)
	}
	if !r.Empty() {
		t.Errorf("expected empty result, got %q", r.Items)
	}
}

func TestIntegration_FHIRPath_MalformedExpression(t *testing.T) {
	_, err := RunFHIRPath("Patient.name..given(", testdataPath("patient.json"), FHIRPathOptions{FHIRVersion: "4.0.1"})
	if err == nil {
		t.Fatal("expected an error for a malformed expression")
	}
	if strings.Contains(err.Error(), "\n\tat ") {
		t.Errorf("error should be a concise message, not a stack trace: %v", err)
	}
}
