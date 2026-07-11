package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fhirlint/fhirlint/internal/rules"
)

func writeRulesFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "rules.yml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write rules file: %v", err)
	}
	return p
}

func TestLoadRulesFromFile(t *testing.T) {
	p := writeRulesFile(t, `
rules:
  - id: patient-mrn
    resource: Patient
    assert: "identifier.exists()"
    message: "needs identifier"
    severity: error
  - id: has-name
    assert: "name.exists()"
`)
	got, err := loadRulesFromFile(p)
	if err != nil {
		t.Fatalf("loadRulesFromFile: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(got))
	}
	if got[0].ID != "patient-mrn" || got[0].Resource != "Patient" || got[0].Severity != "error" {
		t.Errorf("rule 0 mismatch: %+v", got[0])
	}
	if got[1].Severity != "error" { // default
		t.Errorf("expected default severity 'error', got %q", got[1].Severity)
	}
}

func TestLoadRulesFromFile_BareList(t *testing.T) {
	p := writeRulesFile(t, `
- id: r1
  assert: "name.exists()"
`)
	got, err := loadRulesFromFile(p)
	if err != nil {
		t.Fatalf("loadRulesFromFile: %v", err)
	}
	if len(got) != 1 || got[0].ID != "r1" {
		t.Fatalf("expected one rule r1, got %+v", got)
	}
}

func TestLoadRulesFromFile_InvalidExpr(t *testing.T) {
	p := writeRulesFile(t, `
rules:
  - id: bad
    assert: "identifier +"
`)
	// The file parses fine; the invalid expression is caught when the engine
	// compiles it, not at load — so loadRulesFromFile succeeds here.
	rs, err := loadRulesFromFile(p)
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	if _, err := rules.NewEngine(rs, rules.NewNativeEvaluator()); err == nil {
		t.Fatal("expected compile error for invalid expression")
	}
}

func TestLoadRulesFromFile_Empty(t *testing.T) {
	p := writeRulesFile(t, "rules: []\n")
	if _, err := loadRulesFromFile(p); err == nil {
		t.Fatal("expected error for a rules file with no rules")
	}
}

func TestIsXMLContent(t *testing.T) {
	cases := map[string]bool{
		`{"resourceType":"Patient"}`:      false,
		"  \n\t<Patient/>":                true,
		`<?xml version="1.0"?><Patient/>`: true,
		`   {"a":1}`:                      false,
		``:                                false,
	}
	for in, want := range cases {
		if got := isXMLContent([]byte(in)); got != want {
			t.Errorf("isXMLContent(%q) = %v, want %v", in, got, want)
		}
	}
}
