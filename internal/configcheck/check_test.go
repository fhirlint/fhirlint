package configcheck_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/configcheck"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fhirlint.yml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCheck_ValidConfig(t *testing.T) {
	path := writeConfig(t, `
severity: warning
fail-on: error
fhir-version: 4.0.1
format:
  - terminal
cache: true
`)
	issues, err := configcheck.Check(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected no issues, got: %v", issues)
	}
}

func TestCheck_ReturnsNilForMissingFile(t *testing.T) {
	issues, err := configcheck.Check("/nonexistent/fhirlint.yml")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if issues != nil {
		t.Errorf("expected nil issues for missing file, got: %v", issues)
	}
}

func TestCheck_UnknownKey(t *testing.T) {
	path := writeConfig(t, `fhir-versoin: 4.0.1`)
	issues, err := configcheck.Check(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) == 0 {
		t.Fatal("expected issue for unknown key, got none")
	}
	if !strings.Contains(issues[0].Message, "fhir-versoin") {
		t.Errorf("expected key name in message, got: %q", issues[0].Message)
	}
}

func TestCheck_UnknownKey_Suggestion(t *testing.T) {
	path := writeConfig(t, `fhir-versoin: 4.0.1`)
	issues, _ := configcheck.Check(path)
	if !strings.Contains(issues[0].Message, "fhir-version") {
		t.Errorf("expected suggestion in message, got: %q", issues[0].Message)
	}
}

func TestCheck_InvalidEnum_Severity(t *testing.T) {
	path := writeConfig(t, `severity: verbose`)
	issues, _ := configcheck.Check(path)
	if len(issues) == 0 {
		t.Fatal("expected issue for invalid enum value")
	}
	if !strings.Contains(issues[0].Message, "verbose") {
		t.Errorf("expected invalid value in message, got: %q", issues[0].Message)
	}
	if !strings.Contains(issues[0].Message, "information") {
		t.Errorf("expected allowed values in message, got: %q", issues[0].Message)
	}
}

func TestCheck_InvalidEnum_FailOn(t *testing.T) {
	path := writeConfig(t, `fail-on: fatal`)
	issues, _ := configcheck.Check(path)
	if len(issues) == 0 {
		t.Fatal("expected issue for invalid fail-on value")
	}
}

func TestCheck_InvalidEnum_FHIRVersion(t *testing.T) {
	path := writeConfig(t, `fhir-version: 3.0.0`)
	issues, _ := configcheck.Check(path)
	if len(issues) == 0 {
		t.Fatal("expected issue for invalid fhir-version")
	}
}

func TestCheck_InvalidEnum_Format(t *testing.T) {
	path := writeConfig(t, "format:\n  - csv\n")
	issues, _ := configcheck.Check(path)
	if len(issues) == 0 {
		t.Fatal("expected issue for invalid format value")
	}
	if !strings.Contains(issues[0].Message, "csv") {
		t.Errorf("expected invalid value in message, got: %q", issues[0].Message)
	}
}

func TestCheck_TypeError_Bool(t *testing.T) {
	path := writeConfig(t, `cache: "yes"`)
	issues, _ := configcheck.Check(path)
	if len(issues) == 0 {
		t.Fatal("expected issue for invalid bool value")
	}
	if !strings.Contains(issues[0].Message, "boolean") {
		t.Errorf("expected 'boolean' in message, got: %q", issues[0].Message)
	}
}

func TestCheck_TypeError_Int(t *testing.T) {
	path := writeConfig(t, `max-warnings: "ten"`)
	issues, _ := configcheck.Check(path)
	if len(issues) == 0 {
		t.Fatal("expected issue for invalid int value")
	}
	if !strings.Contains(issues[0].Message, "integer") {
		t.Errorf("expected 'integer' in message, got: %q", issues[0].Message)
	}
}

func TestCheck_ValidSuppressRules(t *testing.T) {
	path := writeConfig(t, `
suppress:
  - messageId: dom-6
  - constraint: dom-6
  - expression: Patient.text
    severity: warning
  - pattern: ".*example.*"
`)
	issues, _ := configcheck.Check(path)
	if len(issues) != 0 {
		t.Errorf("expected no issues for valid suppress rules, got: %v", issues)
	}
}

func TestCheck_SuppressRule_UnknownKey(t *testing.T) {
	path := writeConfig(t, `
suppress:
  - msgId: dom-6
`)
	issues, _ := configcheck.Check(path)
	if len(issues) == 0 {
		t.Fatal("expected issue for unknown suppress key")
	}
}

func TestCheck_SuppressRule_MissingTypeKey(t *testing.T) {
	path := writeConfig(t, `
suppress:
  - severity: warning
`)
	issues, _ := configcheck.Check(path)
	if len(issues) == 0 {
		t.Fatal("expected issue when suppress map has no type key")
	}
	if !strings.Contains(issues[0].Message, "messageId") {
		t.Errorf("expected 'messageId' in message, got: %q", issues[0].Message)
	}
}

func TestCheck_ValidRules(t *testing.T) {
	path := writeConfig(t, `
rules:
  - id: patient-mrn
    resource: Patient
    assert: "identifier.exists()"
    message: "needs identifier"
    severity: error
  - id: has-name
    assert: "name.exists()"
`)
	issues, _ := configcheck.Check(path)
	if len(issues) != 0 {
		t.Errorf("expected no issues for valid rules, got: %v", issues)
	}
}

func TestCheck_Rule_UnknownKey(t *testing.T) {
	path := writeConfig(t, `
rules:
  - id: r1
    assert: "name.exists()"
    expr: "oops"
`)
	issues, _ := configcheck.Check(path)
	if len(issues) == 0 {
		t.Fatal("expected issue for unknown rule key")
	}
}

func TestCheck_Rule_MissingAssert(t *testing.T) {
	path := writeConfig(t, `
rules:
  - id: r1
    severity: warning
`)
	issues, _ := configcheck.Check(path)
	if len(issues) == 0 {
		t.Fatal("expected issue when rule has no assert")
	}
	if !strings.Contains(issues[0].Message, "assert") {
		t.Errorf("expected 'assert' in message, got: %q", issues[0].Message)
	}
}

func TestCheck_Rule_InvalidSeverity(t *testing.T) {
	path := writeConfig(t, `
rules:
  - id: r1
    assert: "name.exists()"
    severity: critical
`)
	issues, _ := configcheck.Check(path)
	if len(issues) == 0 {
		t.Fatal("expected issue for invalid rule severity")
	}
}

func TestCheck_ValidLint(t *testing.T) {
	path := writeConfig(t, `
lint:
  id-kebab-case: warning
  profile-name-pascalcase: error
  canonical-url-pattern:
    severity: error
    base: "https://example.org/fhir/"
`)
	issues, _ := configcheck.Check(path)
	if len(issues) != 0 {
		t.Errorf("expected no issues for valid lint config, got: %v", issues)
	}
}

func TestCheck_Lint_UnknownRule(t *testing.T) {
	path := writeConfig(t, `
lint:
  no-such-rule: warning
`)
	issues, _ := configcheck.Check(path)
	if len(issues) == 0 {
		t.Fatal("expected issue for unknown lint rule")
	}
	if !strings.Contains(issues[0].Message, "unknown lint rule") {
		t.Errorf("expected 'unknown lint rule' in message, got: %q", issues[0].Message)
	}
}

func TestCheck_Lint_InvalidSeverity(t *testing.T) {
	path := writeConfig(t, `
lint:
  id-kebab-case: critical
`)
	issues, _ := configcheck.Check(path)
	if len(issues) == 0 {
		t.Fatal("expected issue for invalid lint severity")
	}
}

func TestCheck_Lint_MissingRequiredParam(t *testing.T) {
	path := writeConfig(t, `
lint:
  canonical-url-pattern: error
`)
	issues, _ := configcheck.Check(path)
	if len(issues) == 0 {
		t.Fatal("expected issue when required param is missing")
	}
	if !strings.Contains(issues[0].Message, "base") {
		t.Errorf("expected 'base' in message, got: %q", issues[0].Message)
	}
}

func TestCheck_ValidOverride(t *testing.T) {
	path := writeConfig(t, `
overrides:
  - files: ["tests/**"]
    severity: error
    fail-on: never
`)
	issues, _ := configcheck.Check(path)
	if len(issues) != 0 {
		t.Errorf("expected no issues for valid override, got: %v", issues)
	}
}

func TestCheck_Override_UnknownKey(t *testing.T) {
	path := writeConfig(t, `
overrides:
  - files: ["tests/**"]
    fhir-version: 4.0.1
`)
	issues, _ := configcheck.Check(path)
	if len(issues) == 0 {
		t.Fatal("expected issue for unknown override key (fhir-version not overridable)")
	}
}

func TestCheck_Override_MissingFiles(t *testing.T) {
	path := writeConfig(t, `
overrides:
  - severity: error
`)
	issues, _ := configcheck.Check(path)
	if len(issues) == 0 {
		t.Fatal("expected issue for override missing 'files' key")
	}
	if !strings.Contains(issues[0].Message, "files") {
		t.Errorf("expected 'files' in message, got: %q", issues[0].Message)
	}
}

func TestCheck_ReportsLineNumbers(t *testing.T) {
	path := writeConfig(t, "severity: warning\nfhir-versoin: 4.0.1\n")
	issues, _ := configcheck.Check(path)
	if len(issues) == 0 {
		t.Fatal("expected at least one issue")
	}
	if issues[0].Line != 2 {
		t.Errorf("expected line 2, got %d", issues[0].Line)
	}
}

func TestCheck_EmptyFile(t *testing.T) {
	path := writeConfig(t, "")
	issues, err := configcheck.Check(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Errorf("expected no issues for empty file, got: %v", issues)
	}
}

func TestCheck_InvalidYAML(t *testing.T) {
	path := writeConfig(t, "key: [unclosed")
	issues, err := configcheck.Check(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) == 0 {
		t.Fatal("expected parse error issue for invalid YAML")
	}
	if !strings.Contains(issues[0].Message, "YAML") {
		t.Errorf("expected 'YAML' in parse error message, got: %q", issues[0].Message)
	}
}

func TestCheck_SuppressExpiresIsValidated(t *testing.T) {
	// config check must agree with what validate accepts (#254): before this,
	// a malformed date passed here and only failed at validation time.
	tests := []struct {
		name      string
		yaml      string
		wantIssue bool
	}{
		{"valid global", "suppress:\n  - constraint: dom-6\n    expires: 2026-12-31\n", false},
		{"malformed global", "suppress:\n  - constraint: dom-6\n    expires: 31.12.2026\n", true},
		{"malformed month", "suppress:\n  - constraint: dom-6\n    expires: 2026-13-01\n", true},
		{"not a date", "suppress:\n  - constraint: dom-6\n    expires: soon\n", true},
		{"no expires at all", "suppress:\n  - constraint: dom-6\n", false},
		{
			"malformed under overrides",
			"overrides:\n  - files: \"*.json\"\n    suppress:\n      - constraint: dom-6\n        expires: 31.12.2026\n",
			true,
		},
		{
			"valid under overrides",
			"overrides:\n  - files: \"*.json\"\n    suppress:\n      - constraint: dom-6\n        expires: 2026-12-31\n",
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.yaml)
			issues, err := configcheck.Check(path)
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			got := len(issues) > 0
			if got != tt.wantIssue {
				t.Errorf("issues = %v (%v), want issue: %v", got, issues, tt.wantIssue)
			}
		})
	}
}

// --- severity-override (#311) ---

func TestCheck_ValidSeverityOverrides(t *testing.T) {
	path := writeConfig(t, `
severity-override:
  - messageId: Rule_bdl_1
    severity: warning
    reason: "upstream profile defect"
    expires: 2026-12-31
  - constraint: dom-6
    severity: information
  - pattern: "Unknown extension.*"
    severity: error
`)
	issues, _ := configcheck.Check(path)
	if len(issues) != 0 {
		t.Errorf("expected no issues for valid severity-override rules, got: %v", issues)
	}
}

func TestCheck_SeverityOverride_Rejects(t *testing.T) {
	cases := []struct {
		name, yaml, want string
	}{
		{
			"missing severity",
			"severity-override:\n  - messageId: x\n",
			"must have a severity",
		},
		{
			"missing selector",
			"severity-override:\n  - severity: warning\n",
			"messageId",
		},
		{
			"unknown severity",
			"severity-override:\n  - messageId: x\n    severity: critical\n",
			"invalid value",
		},
		{
			"unknown key",
			"severity-override:\n  - messageId: x\n    severity: warning\n    why: nope\n",
			"unknown severity-override key",
		},
		{
			"string shorthand",
			"severity-override:\n  - \"messageId:x\"\n",
			"must be a map",
		},
		{
			"not a list",
			"severity-override:\n  messageId: x\n",
			"must be a list",
		},
		{
			"malformed expires",
			"severity-override:\n  - messageId: x\n    severity: warning\n    expires: 31.12.2026\n",
			"YYYY-MM-DD",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			issues, _ := configcheck.Check(writeConfig(t, tc.yaml))
			if len(issues) == 0 {
				t.Fatalf("expected an issue for %s", tc.name)
			}
			var found bool
			for _, iss := range issues {
				if strings.Contains(iss.Message, tc.want) {
					found = true
				}
			}
			if !found {
				t.Errorf("expected a message containing %q, got: %v", tc.want, issues)
			}
		})
	}
}
