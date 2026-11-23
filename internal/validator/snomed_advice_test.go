// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package validator

import (
	"strings"
	"testing"
)

// Real messages from validator 7.0.6 on kbv.basis#1.10.0 (2026-11-22).
const (
	clashChanged = "The code system 'http://snomed.info/sct' version 'http://snomed.info/sct/900000000000207008' " +
		"resulting from the version '' in the ValueSet include is different to the one in the value " +
		"('http://snomed.info/sct/11000274103/version/20260815')"
	clashConfirm4a = "The code provided (http://snomed.info/sct#271872005) was not found in the value set " +
		"'LebensphaseDe ValueSet' (http://fhir.de/ValueSet/lebensphase-de|1.6.0), and a code from this value set is required: " +
		clashChanged
	// The German template of Terminology_TX_Confirm_4a wraps the untranslated
	// VALUESET_VALUE_MISMATCH_CHANGED.
	clashConfirm4aDE = "Der angegebene Code (http://snomed.info/sct#271872005) befindet sich nicht im ValueSet " +
		"'LebensphaseDe ValueSet' (http://fhir.de/ValueSet/lebensphase-de|1.6.0), und es wird ein Code aus diesem ValueSet benötigt: " +
		clashChanged
)

func TestSnomedEditionAdvice(t *testing.T) {
	cases := []struct {
		name, msg, want string
	}{
		{"mismatch changed", clashChanged, "de"},
		{"inside Confirm_4a", clashConfirm4a, "de"},
		{"inside Confirm_4a, German", clashConfirm4aDE, "de"},
		{"edition without a version", strings.Replace(clashChanged, "11000274103/version/20260815", "11000274103", 1), "de"},
		{"a shared module takes the shorter name", strings.Replace(clashChanged, "11000274103", "20611000087101", 1), "ca"},
		{"another national edition", strings.Replace(clashChanged, "11000274103", "45991000052106", 1), "se"},
		{"an unnamed module goes as its id", strings.Replace(clashChanged, "11000274103", "11000274999", 1), "11000274999"},

		// -sct changes none of these.
		{"both international", strings.Replace(clashChanged, "11000274103", "900000000000207008", 1), ""},
		{"include pins the international edition",
			"The code system 'http://snomed.info/sct' version 'http://snomed.info/sct/900000000000207008' in the ValueSet include " +
				"is different to the one in the value ('http://snomed.info/sct/11000274103/version/20260815')", ""},
		{"include had a version of its own", strings.Replace(clashChanged, "version ''", "version '20250101'", 1), ""},
		{"versionless include default",
			"The code system 'http://snomed.info/sct' version 'http://snomed.info/sct/900000000000207008' for the versionless include " +
				"in the ValueSet include is different to the one in the value ('http://snomed.info/sct/11000274103/version/20260815')", ""},
		{"another edition assumed", strings.Replace(clashChanged, "900000000000207008", "83821000000107", 1), ""},
		{"unrelated", "Wrong Display Name 'Clinical Finding' for http://snomed.info/sct#404684003", ""},
	}
	for _, c := range cases {
		if got := SnomedEditionAdvice(Issue{Message: c.msg}); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestAnnotateSnomedEdition(t *testing.T) {
	r := &Result{Issues: []Issue{
		{Severity: "error", MessageID: "VALUESET_VALUE_MISMATCH_CHANGED", Message: clashChanged},
		{Severity: "error", MessageID: "Terminology_TX_Confirm_4a", Message: clashConfirm4a},
		{Severity: "error", Message: "Wrong Display Name"},
		{Severity: "error", Message: clashChanged, Redacted: true},
	}}
	AnnotateSnomedEdition([]*Result{nil, r})

	for i, want := range []bool{true, true, false, false} {
		got := r.Issues[i].Advice
		if (got != "") != want {
			t.Errorf("issue %d: advice %q, want one: %v", i, got, want)
		}
		if want && !strings.HasSuffix(got, "add --snomed-edition de") {
			t.Errorf("issue %d: advice %q does not name the flag", i, got)
		}
	}
}
