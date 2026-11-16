// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package fhirpath

import (
	"strings"
	"testing"
)

// The parser and evaluator corners still open after #634 (#691).

// A syntax error on the right of any operator, or inside brackets, parens and
// arguments, is the expression's error: none is silently read as a shorter
// expression.
func TestParseErrorsOnEverySide(t *testing.T) {
	for _, tc := range []struct{ expr, want string }{
		{"a implies )", "position"},
		{"a or )", "position"},
		{"a xor )", "position"},
		{"a and )", "position"},
		{"a in )", "position"},
		{"a = )", "position"},
		{"a != )", "position"},
		{"a > )", "position"},
		{"a <= )", "position"},
		{"a.)", "expected identifier after '.'"},
		{"a[b]", "expected integer index"},
		{"a[1", "expected ']'"},
		{"(a", "expected ')'"},
		{"(a = )", "position"},
		{"a.where(b = )", "position"},
		{"a.where(b c)", "expected ')' to close where("},
		{"a b", "unexpected \"b\""},
	} {
		_, err := Compile(tc.expr)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: got %v, want an error with %q", tc.expr, err, tc.want)
		}
	}
}

// Errors raised under an index, a path step or a comparison come out.
func TestEvalErrorsUnderIndexAndSteps(t *testing.T) {
	for _, tc := range []struct{ expr, want string }{
		{"status.startsWith()[0].exists()", "startsWith() expects 1"},
		{"status.startsWith().exists()", "startsWith() expects 1"},
		{"status.startsWith('a', 'b')", "startsWith() expects 1"},
		{"status.startsWith(status.startsWith())", "startsWith() expects 1"},
		{"status.startsWith(note.text)", "startsWith() expects a single string argument"},
		{"status.matches('(')", "matches(): "},
		{"component.first() in component", "cannot compare complex (object) values"},
		{"component.count().startsWith('a')", "expected a string, got"},
		{"component.count() > status.startsWith()", "startsWith() expects 1"},
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

// Navigating into a primitive or a value that isn't an object yields nothing,
// and a repeated pattern reuses its compiled regex.
func TestEvalNavigationAndRegexReuse(t *testing.T) {
	for _, tc := range []struct {
		expr string
		want bool
	}{
		{"status.code.exists()", false},
		{"note.text.first().matches('^f')", true},
		{"note.text.first().matches('^f') and note.text.last().matches('^f').not()", true},
		{"(status in status) = true", true},
		{"missing in status", false},
		// Unknown or unknown, and unknown implies unknown, stay unknown (empty).
		{"(missing or missing).exists()", false},
		{"(missing implies missing).exists()", false},
	} {
		if got := evalBool(t, tc.expr, observation); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.expr, got, tc.want)
		}
	}
}

// toString() renders a Quantity as value and unit, but an object with any
// element a Quantity doesn't have is not one, however Quantity-like it looks.
func TestToStringQuantityLookalike(t *testing.T) {
	doc := `{"resourceType":"Observation","valueQuantity":{"value":1.0,"code":"kg"},
	  "valueRange":{"value":1.0,"code":"kg","low":{"value":0}}}`
	if !evalBool(t, "valueQuantity.toString() = '1.0 \\'kg\\''", doc) {
		t.Error("a Quantity renders as value and quoted unit")
	}
	if evalBool(t, "valueRange.toString() = '1.0 \\'kg\\''", doc) {
		t.Error("an object with a non-Quantity element must not render as a Quantity")
	}
}
