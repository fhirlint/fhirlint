// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package fhirpath

import (
	"strings"
	"testing"
)

// String escapes, the 'in' operator and the truthiness of criteria (#616).

const escapes = `{"resourceType":"Basic","a":"it's","b":"tab\there","c":"line\nbreak","d":"back\\slash","e":"a/b","f":"q\"uote","g":"cr\rx","h":"ff\fx","i":"x`+"`"+`y","code":["x","y"],"n":[1,2]}`

func TestStringEscapes(t *testing.T) {
	for _, expr := range []string{
		`a = 'it\'s'`,
		`b = 'tab\there'`,
		`c = 'line\nbreak'`,
		`d = 'back\\slash'`,
		`e = 'a\/b'`,
		`f = 'q\"uote'`,
		`g = 'cr\rx'`,
		`h = 'ff\fx'`,
		"i = 'x\\`y'",
	} {
		if !evalBool(t, expr, escapes) {
			t.Errorf("%s: want true", expr)
		}
	}
	for _, bad := range []string{`a = 'it\qs'`, `a = 'open`, `a = 'trailing\`} {
		if _, err := Compile(bad); err == nil {
			t.Errorf("Compile(%s) must fail", bad)
		}
	}
}

func TestMembership(t *testing.T) {
	for expr, want := range map[string]bool{
		"'x' in code":     true,
		"'z' in code":     false,
		"1 in n":          true,
		"missing in code": false, // an empty left operand gives empty, which gates to false
		"'x' in missing":  false,
	} {
		if got := evalBool(t, expr, escapes); got != want {
			t.Errorf("%s: got %v, want %v", expr, got, want)
		}
	}
	p, err := Compile("code in code")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.EvalBool([]byte(escapes)); err == nil || !strings.Contains(err.Error(), "single-item left operand") {
		t.Errorf("a collection on the left of 'in' must be an error, got %v", err)
	}
}

func TestCriterionTruthiness(t *testing.T) {
	// A criterion that yields a non-boolean value counts as present.
	for expr, want := range map[string]bool{
		"code.where($this).exists()":          true,
		"code.where($this = 'x').count() = 1": true,
		"code.all($this.length() = 1)":        true,
		"code.where(false).exists()":          false,
	} {
		if got := evalBool(t, expr, escapes); got != want {
			t.Errorf("%s: got %v, want %v", expr, got, want)
		}
	}
}
