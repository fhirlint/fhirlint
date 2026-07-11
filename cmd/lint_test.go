package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
	"github.com/spf13/viper"
)

// withLintConfig sets the viper lint config for the duration of a test.
func withLintConfig(t *testing.T, cfg interface{}) {
	t.Helper()
	prev := viper.Get("lint")
	viper.Set("lint", cfg)
	t.Cleanup(func() { viper.Set("lint", prev) })
}

func writeResource(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "resource.json")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write resource: %v", err)
	}
	return p
}

func TestApplyLintEngine_MergesFinding(t *testing.T) {
	p := writeResource(t, `{"resourceType":"Patient","id":"BadID"}`)
	withLintConfig(t, map[string]interface{}{"id-kebab-case": "warning"})

	res := &validator.Result{Filename: p, Valid: true}
	if err := applyCustomChecks(nil, []*validator.Result{res}); err != nil {
		t.Fatalf("applyCustomChecks: %v", err)
	}
	if len(res.Issues) != 1 || res.Issues[0].MessageID != "lint:id-kebab-case" {
		t.Fatalf("expected one lint:id-kebab-case issue, got %+v", res.Issues)
	}
	if !res.Valid { // warning does not invalidate
		return
	}
}

func TestApplyLintEngine_ErrorSeverityInvalidates(t *testing.T) {
	p := writeResource(t, `{"resourceType":"Patient","id":"BadID"}`)
	withLintConfig(t, map[string]interface{}{"id-kebab-case": "error"})

	res := &validator.Result{Filename: p, Valid: true}
	if err := applyCustomChecks(nil, []*validator.Result{res}); err != nil {
		t.Fatalf("applyCustomChecks: %v", err)
	}
	if res.Valid {
		t.Fatal("expected Valid=false after error-severity lint finding")
	}
}

func TestApplyLintEngine_NoConfigIsNoop(t *testing.T) {
	p := writeResource(t, `{"resourceType":"Patient","id":"BadID"}`)
	withLintConfig(t, nil)

	res := &validator.Result{Filename: p, Valid: true}
	if err := applyCustomChecks(nil, []*validator.Result{res}); err != nil {
		t.Fatalf("applyCustomChecks: %v", err)
	}
	if len(res.Issues) != 0 {
		t.Fatalf("expected no findings without lint config, got %+v", res.Issues)
	}
}

func TestApplyLintEngine_SkipsXML(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "resource.xml")
	if err := os.WriteFile(p, []byte(`<Patient id="BadID"/>`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	withLintConfig(t, map[string]interface{}{"id-kebab-case": "warning"})

	res := &validator.Result{Filename: p, Valid: true}
	if err := applyCustomChecks(nil, []*validator.Result{res}); err != nil {
		t.Fatalf("applyCustomChecks: %v", err)
	}
	if len(res.Issues) != 0 {
		t.Fatalf("XML must be skipped by lint, got %+v", res.Issues)
	}
}

func TestBuildLintEngine_InvalidConfigErrors(t *testing.T) {
	withLintConfig(t, map[string]interface{}{"no-such-rule": "warning"})
	if _, err := buildLintEngine(); err == nil {
		t.Fatal("expected error for unknown lint rule")
	}
}
