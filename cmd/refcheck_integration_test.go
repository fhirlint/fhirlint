//go:build integration

package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

// TestIntegration_ReferenceCheckOnRealResults runs the real validator over two
// resources and confirms applyReferenceCheck resolves a cross-file reference and
// flags a dangling one, using the Filename the validator records.
func TestIntegration_ReferenceCheckOnRealResults(t *testing.T) {
	dir := t.TempDir()
	patientPath := filepath.Join(dir, "patient.json")
	encPath := filepath.Join(dir, "encounter.json")
	if err := os.WriteFile(patientPath, []byte(`{"resourceType":"Patient","id":"p1","name":[{"family":"X"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(encPath, []byte(`{"resourceType":"Encounter","id":"e1","status":"finished","class":{"code":"AMB"},"subject":{"reference":"Patient/p1"},"participant":[{"individual":{"reference":"Practitioner/ghost"}}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	opts := validator.Options{FHIRVersion: "4.0.1", NoTerminologyServer: true, BestPractice: "ignore"}
	pRes, err := validator.Run(patientPath, opts)
	if err != nil {
		t.Fatalf("validator.Run(patient): %v", err)
	}
	eRes, err := validator.Run(encPath, opts)
	if err != nil {
		t.Fatalf("validator.Run(encounter): %v", err)
	}

	// nil: no --index-only paths, so the index is built from the results
	// themselves. The signature gained this parameter in #288 and this call was
	// never updated — the file has not compiled since, because the integration
	// job only builds ./internal/validator/... .
	applyReferenceCheck([]*validator.Result{pRes, eRes}, nil)

	if !hasIssueID(eRes, "ref:unresolved") {
		t.Fatalf("expected unresolved reference on encounter, got %+v", eRes.Issues)
	}
	// Patient/p1 resolves across files: exactly one unresolved (Practitioner/ghost).
	count := 0
	for _, iss := range eRes.Issues {
		if iss.MessageID == "ref:unresolved" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly one unresolved reference, got %d (%+v)", count, eRes.Issues)
	}
}
