//go:build integration

package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

func runRealValidationLint(t *testing.T, content string) *validator.Result {
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

func hasLintMessageID(res *validator.Result, id string) bool {
	for _, iss := range res.Issues {
		if iss.MessageID == id {
			return true
		}
	}
	return false
}

// TestIntegration_LintFiresOnRealResult confirms the full cmd wiring: a real JAR
// validation of a resource with a JAR-valid but non-kebab id passes validation,
// then applyCustomChecks merges the convention finding.
func TestIntegration_LintFiresOnRealResult(t *testing.T) {
	res := runRealValidationLint(t, `{"resourceType":"Patient","id":"ExamplePatient1","name":[{"family":"X"}]}`)
	// The id is a valid FHIR id, so the validator itself must not have errored on it.
	if hasLintMessageID(res, "lint:id-kebab-case") {
		t.Fatal("precondition: lint finding present before applyCustomChecks")
	}
	withLintConfig(t, map[string]interface{}{"id-kebab-case": "warning"})

	if err := applyCustomChecks(nil, []*validator.Result{res}); err != nil {
		t.Fatalf("applyCustomChecks: %v", err)
	}
	if !hasLintMessageID(res, "lint:id-kebab-case") {
		t.Fatalf("expected lint finding merged into result, got: %+v", res.Issues)
	}
}

// TestIntegration_LintCompliantResource confirms a compliant resource yields no
// lint finding.
func TestIntegration_LintCompliantResource(t *testing.T) {
	res := runRealValidationLint(t, `{"resourceType":"Patient","id":"example-patient-1","name":[{"family":"X"}]}`)
	withLintConfig(t, map[string]interface{}{"id-kebab-case": "error"})

	if err := applyCustomChecks(nil, []*validator.Result{res}); err != nil {
		t.Fatalf("applyCustomChecks: %v", err)
	}
	if hasLintMessageID(res, "lint:id-kebab-case") {
		t.Errorf("expected no lint finding for a kebab-case id, got: %+v", res.Issues)
	}
}
