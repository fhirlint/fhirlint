// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package fhirpath

import (
	"strings"
	"testing"
)

// The evaluator's corners (#634): empty collections, booleans and integers
// from the resource, the resource-type filter, and errors that must come out
// of every operator and function that evaluates an argument.

const cornerPatient = `{"resourceType":"Patient","active":false,"multipleBirthInteger":2,
  "name":[{"family":"A","given":["x","y"]}],"birthDate":"2000-01-01"}`

func TestEvalCorners(t *testing.T) {
	for _, tc := range []struct {
		expr, doc string
		want      bool
	}{
		{"component.exists(valueQuantity.value > 100)", observation, true},
		{"component.exists(valueQuantity.value > 500)", observation, false},
		// Empty in, empty out: first, last, length and the string functions of
		// nothing are nothing, which reads as false.
		{"missing.first().exists()", observation, false},
		{"missing.last().exists()", observation, false},
		{"missing.count() = 0", observation, true},
		{"missing.length() = 1", observation, false},
		{"missing.startsWith('a')", observation, false},
		{"missing.not()", observation, false},
		{"missing = missing", observation, false},
		{"status.hasValue()", observation, true},
		{"missing.hasValue()", observation, false},
		{"component.hasValue()", observation, false}, // a complex element has no value
		// Booleans and integers as the resource carries them.
		{"active = false", cornerPatient, true},
		{"active.not()", cornerPatient, true},
		{"multipleBirthInteger = 2", cornerPatient, true},
		{"name.given.count() = 2", cornerPatient, true},
		{"name[0].family = 'A'", cornerPatient, true},
		{"name[5].exists()", cornerPatient, false},
		// The leading type filter.
		{"Observation.status = 'final'", observation, true},
		{"Observation.status = 'final'", cornerPatient, false},
		{"Patient.status.exists()", observation, false},
		// all() of nothing holds.
		{"component.all(code.exists())", cornerPatient, true},
		{"(1 = 1) = true", observation, true},
		{"name.exists() and birthDate > '1999'", cornerPatient, true},
	} {
		if got := evalBool(t, tc.expr, tc.doc); got != tc.want {
			t.Errorf("%s on %.30s: got %v, want %v", tc.expr, tc.doc, got, tc.want)
		}
	}
}

func TestEvalCornerErrors(t *testing.T) {
	for _, tc := range []struct{ expr, want string }{
		{"status.count(1)", "count() expects 0 argument(s), got 1"},
		{"status.first(1)", "first() expects 0"},
		{"status.last(1)", "last() expects 0"},
		{"status.not(1)", "not() expects 0"},
		{"status.hasValue(1)", "hasValue() expects 0"},
		{"status.length(1)", "length() expects 0"},
		{"status.toString(1)", "toString() expects 0"},
		{"status.where()", "where() expects 1"},
		{"status.all()", "all() expects 1"},
		{"status.not()", "not() expects a single boolean"},
		{"component.not()", "not() expects a single boolean"},
		{"note.text.length() = 1", "expected a single string, got 2 items"},
		{"note.text.startsWith('f')", "expected a single string"},
		// An error inside an operand or a criterion is the expression's error,
		// whichever side it is on.
		{"status.startsWith() = true", "startsWith() expects 1"},
		{"true = status.startsWith()", "startsWith() expects 1"},
		{"status.startsWith() and true", "startsWith() expects 1"},
		{"true and status.startsWith()", "startsWith() expects 1"},
		{"status.startsWith() in status", "startsWith() expects 1"},
		{"status in status.startsWith()", "startsWith() expects 1"},
		{"component.exists(valueQuantity.startsWith())", "startsWith() expects 1"},
		{"component.where(code.startsWith())", "startsWith() expects 1"},
		{"component.all(code.startsWith())", "startsWith() expects 1"},
	} {
		p, err := Compile(tc.expr)
		if err != nil {
			t.Errorf("%s: compile error %v", tc.expr, err)
			continue
		}
		if _, err := p.EvalBool([]byte(observation)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want an error with %q", tc.expr, err, tc.want)
		}
	}
}
