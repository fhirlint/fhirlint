//go:build integration

package validator

import (
	"strings"
	"testing"
)

func TestIntegration_Compare_LocalProfiles(t *testing.T) {
	dest := t.TempDir()
	r, err := RunCompare(
		"http://example.org/fhir/StructureDefinition/demo-patient",
		"http://example.org/fhir/StructureDefinition/demo-patient-v2",
		CompareOptions{
			FHIRVersion: "4.0.1",
			IGs:         []string{testdataPath("profiles", "demo-patient-a.json"), testdataPath("profiles", "demo-patient-b.json")},
			DestDir:     dest,
		},
	)
	if err != nil {
		t.Fatalf("RunCompare() error: %v", err)
	}
	if !r.Differs() {
		t.Fatal("expected differences between the two demo profiles")
	}
	if r.HTMLFile == "" {
		t.Error("expected a generated comparison HTML file")
	}

	var sawCardinality bool
	for _, m := range r.Messages {
		if strings.Contains(m.Message, "cardinalities differ") {
			sawCardinality = true
		}
		if m.Severity == "" || m.Path == "" {
			t.Errorf("message missing severity/path: %+v", m)
		}
	}
	if !sawCardinality {
		t.Errorf("expected a cardinality difference, got: %+v", r.Messages)
	}
}

func TestIntegration_Compare_Equivalent(t *testing.T) {
	dest := t.TempDir()
	canonical := "http://example.org/fhir/StructureDefinition/demo-patient"
	r, err := RunCompare(canonical, canonical, CompareOptions{
		FHIRVersion: "4.0.1",
		IGs:         []string{testdataPath("profiles", "demo-patient-a.json")},
		DestDir:     dest,
	})
	if err != nil {
		t.Fatalf("RunCompare() error: %v", err)
	}
	if r.Differs() {
		t.Errorf("expected no differences comparing a profile to itself, got: %+v", r.Messages)
	}
}
