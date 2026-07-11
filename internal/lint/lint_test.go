package lint

import (
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

func TestParseConfig(t *testing.T) {
	raw := map[string]interface{}{
		"id-kebab-case":           "warning",
		"profile-name-pascalcase": "error",
		"canonical-url-pattern": map[string]interface{}{
			"severity": "error",
			"base":     "https://example.org/fhir/",
		},
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if len(cfg) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(cfg))
	}
	if cfg["canonical-url-pattern"].Params["base"] != "https://example.org/fhir/" {
		t.Errorf("base param not parsed: %+v", cfg["canonical-url-pattern"])
	}
	if cfg["id-kebab-case"].Severity != "warning" {
		t.Errorf("severity not parsed: %+v", cfg["id-kebab-case"])
	}
}

func TestParseConfigErrors(t *testing.T) {
	cases := map[string]interface{}{
		"unknown rule":       map[string]interface{}{"no-such-rule": "warning"},
		"bad severity":       map[string]interface{}{"id-kebab-case": "critical"},
		"missing base param": map[string]interface{}{"canonical-url-pattern": "error"},
		"not a mapping":      []interface{}{"id-kebab-case"},
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseConfig(raw); err == nil {
				t.Fatalf("expected error for %s", name)
			}
		})
	}
}

func TestParseConfigDefaultSeverityFromMap(t *testing.T) {
	// A map form without a severity uses the rule's default.
	raw := map[string]interface{}{
		"canonical-url-pattern": map[string]interface{}{"base": "https://example.org/"},
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if got := cfg["canonical-url-pattern"].Severity; got != "error" {
		t.Errorf("expected default severity 'error', got %q", got)
	}
}

func newEngine(t *testing.T, raw map[string]interface{}) *Engine {
	t.Helper()
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	eng, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	return eng
}

func TestIDKebabCase(t *testing.T) {
	eng := newEngine(t, map[string]interface{}{"id-kebab-case": "warning"})

	bad := eng.Evaluate([]byte(`{"resourceType":"Patient","id":"Example_1"}`))
	if len(bad) != 1 || bad[0].MessageID != "lint:id-kebab-case" || bad[0].Severity != "warning" {
		t.Fatalf("expected one kebab-case finding, got %+v", bad)
	}

	good := eng.Evaluate([]byte(`{"resourceType":"Patient","id":"example-1"}`))
	if len(good) != 0 {
		t.Fatalf("expected no finding for valid id, got %+v", good)
	}

	// A resource without an id is not flagged.
	none := eng.Evaluate([]byte(`{"resourceType":"Patient"}`))
	if len(none) != 0 {
		t.Fatalf("expected no finding for missing id, got %+v", none)
	}
}

func TestCanonicalURLPattern(t *testing.T) {
	eng := newEngine(t, map[string]interface{}{
		"canonical-url-pattern": map[string]interface{}{"severity": "error", "base": "https://example.org/fhir/"},
	})

	bad := eng.Evaluate([]byte(`{"resourceType":"StructureDefinition","url":"http://other.com/sd/Foo"}`))
	if len(bad) != 1 || bad[0].Severity != "error" {
		t.Fatalf("expected one url finding, got %+v", bad)
	}

	good := eng.Evaluate([]byte(`{"resourceType":"StructureDefinition","url":"https://example.org/fhir/StructureDefinition/Foo"}`))
	if len(good) != 0 {
		t.Fatalf("expected no finding for compliant url, got %+v", good)
	}
}

func TestProfileNamePascalCase(t *testing.T) {
	eng := newEngine(t, map[string]interface{}{"profile-name-pascalcase": "warning"})

	bad := eng.Evaluate([]byte(`{"resourceType":"StructureDefinition","name":"my_profile"}`))
	if len(bad) != 1 {
		t.Fatalf("expected one name finding, got %+v", bad)
	}

	good := eng.Evaluate([]byte(`{"resourceType":"StructureDefinition","name":"MyProfile"}`))
	if len(good) != 0 {
		t.Fatalf("expected no finding for PascalCase name, got %+v", good)
	}

	// The rule only applies to StructureDefinition.
	other := eng.Evaluate([]byte(`{"resourceType":"Patient","name":[{"family":"x"}]}`))
	if len(other) != 0 {
		t.Fatalf("profile name rule must not fire on non-SD resources, got %+v", other)
	}
}

func TestEvaluateResultInvalidatesOnError(t *testing.T) {
	eng := newEngine(t, map[string]interface{}{"id-kebab-case": "error"})
	res := &validator.Result{Valid: true}
	eng.EvaluateResult(res, []byte(`{"resourceType":"Patient","id":"BAD_ID"}`))
	if res.Valid {
		t.Fatal("expected Valid=false after error-severity lint finding")
	}
	if len(res.Issues) != 1 {
		t.Fatalf("expected one merged issue, got %+v", res.Issues)
	}
}

func TestNewEngineEmpty(t *testing.T) {
	eng, err := NewEngine(nil)
	if err != nil {
		t.Fatalf("NewEngine(nil): %v", err)
	}
	if eng != nil {
		t.Fatal("expected nil engine for empty config")
	}
}
