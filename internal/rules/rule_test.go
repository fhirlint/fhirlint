package rules

import "testing"

func TestParseMap(t *testing.T) {
	tests := []struct {
		name    string
		in      map[string]interface{}
		wantErr bool
		want    Rule
	}{
		{
			name: "full rule",
			in: map[string]interface{}{
				"id":       "patient-mrn",
				"resource": "Patient",
				"assert":   "identifier.exists()",
				"message":  "Patient needs an identifier",
				"severity": "warning",
			},
			want: Rule{ID: "patient-mrn", Resource: "Patient", Assert: "identifier.exists()", Message: "Patient needs an identifier", Severity: "warning"},
		},
		{
			name: "severity defaults to error",
			in:   map[string]interface{}{"id": "r1", "assert": "true"},
			want: Rule{ID: "r1", Assert: "true", Severity: "error"},
		},
		{
			name: "lowercase keys (viper)",
			in:   map[string]interface{}{"id": "r2", "assert": "name.exists()", "severity": "information"},
			want: Rule{ID: "r2", Assert: "name.exists()", Severity: "information"},
		},
		{name: "missing id", in: map[string]interface{}{"assert": "true"}, wantErr: true},
		{name: "missing assert", in: map[string]interface{}{"id": "r3"}, wantErr: true},
		{name: "bad id char", in: map[string]interface{}{"id": "bad id", "assert": "true"}, wantErr: true},
		{name: "invalid severity", in: map[string]interface{}{"id": "r4", "assert": "true", "severity": "critical"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMap(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got rule %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.ID != tt.want.ID || got.Resource != tt.want.Resource || got.Assert != tt.want.Assert ||
				got.Message != tt.want.Message || got.Severity != tt.want.Severity {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestMessageID(t *testing.T) {
	r := Rule{ID: "no-example-refs"}
	if got := r.MessageID(); got != "rule:no-example-refs" {
		t.Fatalf("MessageID() = %q, want rule:no-example-refs", got)
	}
}

func TestFailureMessageDefault(t *testing.T) {
	r := Rule{ID: "r1", Assert: "identifier.exists()"}
	want := `rule "r1" failed: identifier.exists()`
	if got := r.failureMessage(); got != want {
		t.Fatalf("failureMessage() = %q, want %q", got, want)
	}
}

func TestValidateDuplicateIDs(t *testing.T) {
	rules := []Rule{
		{ID: "dup", Assert: "true", Severity: "error"},
		{ID: "dup", Assert: "false", Severity: "error"},
	}
	if err := Validate(rules); err == nil {
		t.Fatal("expected duplicate id error")
	}
}
