package fhirpath

import (
	"strings"
	"testing"
)

const observation = `{
  "resourceType": "Observation",
  "status": "final",
  "effectiveDateTime": "2026-11-14T10:00:00Z",
  "valueQuantity": {"value": 72.5, "unit": "kg", "system": "http://unitsofmeasure.org", "code": "kg"},
  "component": [
    {"code": {"text": "systolic"}, "valueQuantity": {"value": 120, "code": "mm[Hg]"}},
    {"code": {"text": "diastolic"}, "valueQuantity": {"value": 80, "code": "mm[Hg]"}}
  ],
  "note": [{"text": "first"}, {"text": "last"}]
}`

// Comparisons, the three-valued logic and the string functions, each with a
// case that must hold and one that must not (#595).
func TestEvalComparisonsAndLogic(t *testing.T) {
	for expr, want := range map[string]bool{
		"valueQuantity.value > 70":    true,
		"valueQuantity.value >= 72.5": true,
		"valueQuantity.value < 72.5":  false,
		"valueQuantity.value <= 72":   false,
		"component.first().valueQuantity.value > component.last().valueQuantity.value": true,
		"status > 'amended'":                                     true,
		"status < 'amended'":                                     false,
		"effectiveDateTime >= '2026-01-01'":                      true,
		"status != 'preliminary'":                                true,
		"status != 'final'":                                      false,
		"note.last().text = 'last'":                              true,
		"note.first().text.endsWith('st')":                       true,
		"note.first().text.contains('irs')":                      true,
		"note.first().text.contains('xyz')":                      false,
		"status.length() = 5":                                    true,
		"(status = 'final').not()":                               false,
		"missing.exists().not()":                                 true,
		"status = 'final' xor status = 'amended'":                true,
		"status = 'final' xor status.exists()":                   false,
		"missing = 1 or status = 'final'":                        true,
		"status = 'x' or missing = 1":                            false,
		"status = 'x' or status = 'y'":                           false,
		"status = 'x' implies missing = 1":                       true,
		"missing = 1 implies status = 'final'":                   true,
		"status = 'final' implies status.exists()":               true,
		"status = 'final' implies status = 'x'":                  false,
		"valueQuantity.toString() = '72.5 \\'kg\\''":             true,
		"status.toString() = 'final'":                            true,
		"valueQuantity.value.toString() = '72.5'":                true,
		"(status = 'final').toString() = 'true'":                 true,
		"component.all(valueQuantity.value > 50)":                true,
		"component.where(valueQuantity.value > 100).count() = 1": true,
	} {
		if got := evalBool(t, expr, observation); got != want {
			t.Errorf("%s: got %v, want %v", expr, got, want)
		}
	}
}

func TestEvalTypeAndArgumentErrors(t *testing.T) {
	for _, tc := range []struct{ expr, want string }{
		{"valueQuantity.value > 'x'", "cannot compare number with string"},
		{"status > 1", "cannot compare string with number"},
		{"valueQuantity > 1", "object"},
		{"status.startsWith()", "expects 1 argument(s), got 0"},
		{"status.exists(1, 2)", "expects 0 to 1 arguments, got 2"},
		{"status.empty(1)", "expects 0 argument(s), got 1"},
		{"status.startsWith(1)", "single string argument"},
		{"status.matches('(')", "matches()"},
		{"component.code.text = 'x'", "single-item operands"},
		{"valueQuantity = valueQuantity", "cannot compare complex"},
		{"component.code.text.toString() = 'x'", "toString() expects a single item"},
	} {
		p, err := Compile(tc.expr)
		if err != nil {
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("%s: compile error %v, want %q", tc.expr, err, tc.want)
			}
			continue
		}
		_, err = p.EvalBool([]byte(observation))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want an error with %q", tc.expr, err, tc.want)
		}
	}
}

func TestFormatScalarAndTypeName(t *testing.T) {
	for v, want := range map[any]string{"s": "s", true: "true", 1.5: "1.5", nil: ""} {
		if got := formatScalar(v); got != want {
			t.Errorf("formatScalar(%v) = %q, want %q", v, got, want)
		}
	}
	if got := formatScalar(int64(3)); got != "3" {
		t.Errorf("formatScalar of another type: %q", got)
	}
	for v, want := range map[any]string{true: "boolean", 1.0: "number", "s": "string", nil: "null"} {
		if got := typeName(v); got != want {
			t.Errorf("typeName(%v) = %q, want %q", v, got, want)
		}
	}
	if got := typeName(int64(1)); got != "int64" {
		t.Errorf("typeName of another type: %q", got)
	}
}
