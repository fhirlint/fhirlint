//go:build integration

package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

// runRealValidation validates content with the real JAR and returns the Result,
// whose Filename points at a temp file the rule engine can re-read.
func runRealValidation(t *testing.T, content string) *validator.Result {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "resource.json")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write resource: %v", err)
	}
	res, err := validator.Run(p, validator.Options{
		FHIRVersion:         "4.0.1",
		NoTerminologyServer: true,
	})
	if err != nil {
		t.Fatalf("validator.Run: %v", err)
	}
	return res
}

// withRulesFile writes rules to a temp file, points --rules-file at it, and
// restores the global flag afterwards.
func withRulesFile(t *testing.T, rulesYAML string) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "rules.yml")
	if err := os.WriteFile(p, []byte(rulesYAML), 0o600); err != nil {
		t.Fatalf("write rules file: %v", err)
	}
	prev := flagRulesFile
	flagRulesFile = p
	t.Cleanup(func() { flagRulesFile = prev })
}

func hasMessageID(res *validator.Result, id string) bool {
	for _, iss := range res.Issues {
		if iss.MessageID == id {
			return true
		}
	}
	return false
}

const patientNoMRN = `{
  "resourceType": "Patient",
  "id": "p1",
  "name": [{"family": "Mustermann", "given": ["Max"]}]
}`

const patientWithMRN = `{
  "resourceType": "Patient",
  "id": "p1",
  "identifier": [{"system": "http://hospital.example/mrn", "value": "12345"}],
  "name": [{"family": "Mustermann", "given": ["Max"]}]
}`

// TestIntegration_RuleFailsOnRealResult exercises the full cmd wiring: a real
// JAR validation produces a Result, then applyCustomChecks re-reads the file and
// merges a failing rule finding alongside the JAR's own issues.
func TestIntegration_RuleFailsOnRealResult(t *testing.T) {
	res := runRealValidation(t, patientNoMRN)
	withRulesFile(t, `
rules:
  - id: patient-needs-mrn
    resource: Patient
    assert: "identifier.where(system='http://hospital.example/mrn').exists()"
    message: "Patient is missing an MRN identifier"
    severity: error
`)

	if err := applyCustomChecks(nil, []*validator.Result{res}); err != nil {
		t.Fatalf("applyCustomChecks: %v", err)
	}
	if !hasMessageID(res, "rule:patient-needs-mrn") {
		t.Fatalf("expected rule finding merged into result, got issues: %+v", res.Issues)
	}
	if res.Valid {
		t.Error("expected Valid=false after an error-severity rule finding")
	}
}

// TestIntegration_RulePassesOnRealResult confirms a satisfied rule adds no
// finding and does not affect a resource's validity.
func TestIntegration_RulePassesOnRealResult(t *testing.T) {
	res := runRealValidation(t, patientWithMRN)
	wasValid := res.Valid
	withRulesFile(t, `
rules:
  - id: patient-needs-mrn
    resource: Patient
    assert: "identifier.where(system='http://hospital.example/mrn').exists()"
    severity: error
`)

	if err := applyCustomChecks(nil, []*validator.Result{res}); err != nil {
		t.Fatalf("applyCustomChecks: %v", err)
	}
	if hasMessageID(res, "rule:patient-needs-mrn") {
		t.Fatalf("expected no rule finding for a compliant resource, got: %+v", res.Issues)
	}
	if res.Valid != wasValid {
		t.Errorf("rule that holds must not change validity (was %v, now %v)", wasValid, res.Valid)
	}
}

// TestIntegration_ResourceTypeFilter confirms a rule scoped to another
// resourceType does not fire on this resource.
func TestIntegration_ResourceTypeFilter(t *testing.T) {
	res := runRealValidation(t, patientNoMRN)
	withRulesFile(t, `
rules:
  - id: obs-status
    resource: Observation
    assert: "status.exists()"
    severity: error
`)

	if err := applyCustomChecks(nil, []*validator.Result{res}); err != nil {
		t.Fatalf("applyCustomChecks: %v", err)
	}
	if hasMessageID(res, "rule:obs-status") {
		t.Error("Observation-scoped rule must not fire on a Patient")
	}
}
