// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package validator

import (
	"strings"
	"testing"
)

func TestSnomedEditions_ModuleIDsAreSCTIDs(t *testing.T) {
	// The table is copied from upstream: a module id that fails the check
	// digit is a copying mistake, and isSCTID would reject it if typed.
	for name, id := range snomedEditions {
		if !isSCTID(id) {
			t.Errorf("%s: %s is not a valid SNOMED CT identifier", name, id)
		}
		if name != strings.ToLower(name) {
			t.Errorf("%s: names are looked up in lower case", name)
		}
	}
}

func TestSnomedEditionModule(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"de", "11000274103"},
		{"DE", "11000274103"},
		{"intl", "900000000000207008"},
		{"es-ES", "900000001000122104"},
		{"uk/clinical", "999000021000000109"},
		// A module id passes as written, also one the table does not name.
		{"11000274103", "11000274103"},
		{"45991000052106", "45991000052106"},
	}
	for _, c := range cases {
		got, err := snomedEditionModule(c.in)
		if err != nil {
			t.Errorf("%q: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("%q: got %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSnomedEditionModule_Rejects(t *testing.T) {
	for _, in := range []string{
		"german",      // not a name the validator knows
		"11000274104", // the German module id with a wrong check digit
		"11000247103", // two digits swapped
		"011000274103",
		"12345", // too short for an SCTID
		"http://snomed.info/sct/11000274103",
	} {
		_, err := snomedEditionModule(in)
		if err == nil {
			t.Errorf("%q: accepted", in)
			continue
		}
		if !strings.Contains(err.Error(), "de, dk") {
			t.Errorf("%q: error does not list the names: %v", in, err)
		}
	}
}
