package suppress

import (
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

func TestParseCLI_Valid(t *testing.T) {
	cases := []struct {
		input      string
		wantType   string
		wantVal    string
		wantReason string
	}{
		{"messageId:dom-6", "messageId", "dom-6", ""},
		{"constraint:dom-6", "constraint", "dom-6", ""},
		{"expression:Patient.name", "expression", "Patient.name", ""},
		{"messageId:Measure_M_POPULATIONIDENTIFIER", "messageId", "Measure_M_POPULATIONIDENTIFIER", ""},
		{"constraint:dom-6|Narrative not required in API context", "constraint", "dom-6", "Narrative not required in API context"},
		{"messageId:UNKNOWN_CODESYSTEM| trimmed reason ", "messageId", "UNKNOWN_CODESYSTEM", "trimmed reason"},
	}
	for _, tc := range cases {
		r, err := ParseCLI(tc.input)
		if err != nil {
			t.Errorf("ParseCLI(%q) error: %v", tc.input, err)
			continue
		}
		if r.Type != tc.wantType {
			t.Errorf("ParseCLI(%q).Type = %q, want %q", tc.input, r.Type, tc.wantType)
		}
		if r.Value != tc.wantVal {
			t.Errorf("ParseCLI(%q).Value = %q, want %q", tc.input, r.Value, tc.wantVal)
		}
		if r.Reason != tc.wantReason {
			t.Errorf("ParseCLI(%q).Reason = %q, want %q", tc.input, r.Reason, tc.wantReason)
		}
	}
}

func TestParseCLI_Invalid(t *testing.T) {
	cases := []string{
		"dom-6",          // missing type prefix
		"messageId:",     // empty value
		"unknown:dom-6",  // unknown type
	}
	for _, s := range cases {
		if _, err := ParseCLI(s); err == nil {
			t.Errorf("ParseCLI(%q) expected error, got nil", s)
		}
	}
}

func TestParseMap_Valid(t *testing.T) {
	r, err := ParseMap(map[string]interface{}{"messageId": "dom-6"})
	if err != nil {
		t.Fatalf("ParseMap error: %v", err)
	}
	if r.Type != "messageId" || r.Value != "dom-6" {
		t.Errorf("unexpected rule: %+v", r)
	}
}

func TestParseMap_WithSeverity(t *testing.T) {
	r, err := ParseMap(map[string]interface{}{"expression": "Patient.name", "severity": "warning"})
	if err != nil {
		t.Fatalf("ParseMap error: %v", err)
	}
	if r.Severity != "warning" {
		t.Errorf("Severity = %q, want %q", r.Severity, "warning")
	}
}

func TestParseMap_WithReason(t *testing.T) {
	r, err := ParseMap(map[string]interface{}{"constraint": "dom-6", "reason": "Narrative not required in API context"})
	if err != nil {
		t.Fatalf("ParseMap error: %v", err)
	}
	if r.Reason != "Narrative not required in API context" {
		t.Errorf("Reason = %q, want %q", r.Reason, "Narrative not required in API context")
	}
}

func TestParseMap_MissingType(t *testing.T) {
	if _, err := ParseMap(map[string]interface{}{"severity": "error"}); err == nil {
		t.Error("expected error for map with no type key")
	}
}

func TestMatches_MessageID(t *testing.T) {
	r := Rule{Type: "messageId", Value: "dom-6"}
	issue := validator.Issue{Severity: "warning", MessageID: "dom-6", Location: "Patient"}
	if !r.Matches(issue) {
		t.Error("expected match for messageId:dom-6")
	}
	issue.MessageID = "dom-7"
	if r.Matches(issue) {
		t.Error("unexpected match for different messageId")
	}
}

func TestMatches_Constraint(t *testing.T) {
	r := Rule{Type: "constraint", Value: "dom-6"}
	// exact short ID
	if !r.Matches(validator.Issue{Severity: "warning", MessageID: "dom-6"}) {
		t.Error("expected match for short messageId dom-6")
	}
	// full URI emitted by HL7 FHIR Validator >= 6.x
	if !r.Matches(validator.Issue{Severity: "warning", MessageID: "http://hl7.org/fhir/StructureDefinition/DomainResource#dom-6"}) {
		t.Error("expected match for full URI messageId ending in #dom-6")
	}
	// different constraint should not match
	if r.Matches(validator.Issue{Severity: "warning", MessageID: "http://hl7.org/fhir/StructureDefinition/DomainResource#dom-7"}) {
		t.Error("unexpected match for different constraint dom-7")
	}
}

func TestMatches_Expression(t *testing.T) {
	r := Rule{Type: "expression", Value: "MedicationRequest.intent"}
	cases := []struct {
		loc   string
		match bool
	}{
		{"MedicationRequest.intent", true},
		{"MedicationRequest.intent (line 3, col 5)", true},
		{"MedicationRequest.intent.value", true},
		{"MedicationRequest.status", false},
		{"Patient.name", false},
	}
	for _, tc := range cases {
		issue := validator.Issue{Location: tc.loc}
		if r.Matches(issue) != tc.match {
			t.Errorf("expression match(%q) = %v, want %v", tc.loc, !tc.match, tc.match)
		}
	}
}

func TestMatches_SeverityFilter(t *testing.T) {
	r := Rule{Type: "messageId", Value: "dom-6", Severity: "error"}
	if r.Matches(validator.Issue{Severity: "warning", MessageID: "dom-6"}) {
		t.Error("should not match warning when severity filter is error")
	}
	if !r.Matches(validator.Issue{Severity: "error", MessageID: "dom-6"}) {
		t.Error("should match error when severity filter is error")
	}
}

func TestApply_SplitsIssues(t *testing.T) {
	results := []*validator.Result{
		{
			Valid: false,
			Issues: []validator.Issue{
				{Severity: "error", MessageID: "dom-6", Message: "err"},
				{Severity: "warning", MessageID: "other", Message: "warn"},
			},
		},
	}
	rules := []Rule{{Type: "messageId", Value: "dom-6"}}
	counts := Apply(results, rules)

	if len(results[0].Issues) != 1 {
		t.Errorf("active issues = %d, want 1", len(results[0].Issues))
	}
	if len(results[0].Suppressed) != 1 {
		t.Errorf("suppressed issues = %d, want 1", len(results[0].Suppressed))
	}
	if counts[0] != 1 {
		t.Errorf("match count = %d, want 1", counts[0])
	}
}

func TestApply_RecomputesValid(t *testing.T) {
	results := []*validator.Result{
		{
			Valid: false,
			Issues: []validator.Issue{
				{Severity: "error", MessageID: "dom-6"},
			},
		},
	}
	rules := []Rule{{Type: "messageId", Value: "dom-6"}}
	Apply(results, rules)

	if !results[0].Valid {
		t.Error("result should be valid after suppressing the only error")
	}
}

func TestApply_UnmatchedRuleHasZeroCount(t *testing.T) {
	results := []*validator.Result{
		{Issues: []validator.Issue{{Severity: "warning", MessageID: "other"}}},
	}
	rules := []Rule{{Type: "messageId", Value: "dom-6"}}
	counts := Apply(results, rules)
	if counts[0] != 0 {
		t.Errorf("unmatched rule count = %d, want 0", counts[0])
	}
}

func TestParseCLI_Pattern(t *testing.T) {
	r, err := ParseCLI("pattern:.*example\\.org.*")
	if err != nil {
		t.Fatalf("ParseCLI pattern error: %v", err)
	}
	if r.Type != "pattern" {
		t.Errorf("Type = %q, want pattern", r.Type)
	}
	if r.Regexp == nil {
		t.Error("Regexp should be compiled and non-nil")
	}
}

func TestParseCLI_InvalidPattern(t *testing.T) {
	if _, err := ParseCLI("pattern:[invalid"); err == nil {
		t.Error("expected error for invalid regex pattern")
	}
}

func TestParseMap_Pattern(t *testing.T) {
	r, err := ParseMap(map[string]interface{}{"pattern": ".*example\\.org.*"})
	if err != nil {
		t.Fatalf("ParseMap pattern error: %v", err)
	}
	if r.Type != "pattern" || r.Regexp == nil {
		t.Errorf("unexpected rule: %+v", r)
	}
}

func TestParseMap_InvalidPattern(t *testing.T) {
	if _, err := ParseMap(map[string]interface{}{"pattern": "[invalid"}); err == nil {
		t.Error("expected error for invalid regex in map")
	}
}

func TestMatches_Pattern(t *testing.T) {
	r, _ := ParseCLI("pattern:.*example\\.org.*")
	cases := []struct {
		message string
		match   bool
	}{
		{"Unknown code from example.org/codes", true},
		{"URL points to example.org in narrative", true},
		{"Unrelated validation message", false},
		{"", false},
	}
	for _, tc := range cases {
		issue := validator.Issue{Severity: "warning", Message: tc.message}
		if r.Matches(issue) != tc.match {
			t.Errorf("pattern.Matches(%q) = %v, want %v", tc.message, !tc.match, tc.match)
		}
	}
}

func TestMatches_PatternWithSeverityFilter(t *testing.T) {
	r, _ := ParseCLI("pattern:.*example\\.org.*")
	r.Severity = "warning"
	if r.Matches(validator.Issue{Severity: "error", Message: "code from example.org"}) {
		t.Error("should not match when severity filter does not match")
	}
	if !r.Matches(validator.Issue{Severity: "warning", Message: "code from example.org"}) {
		t.Error("should match when severity filter matches")
	}
}
