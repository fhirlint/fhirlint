// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package validator

import (
	"fmt"
	"sort"
	"strings"
)

// snomedEditions are the names the validator's -sct accepts for a SNOMED CT
// edition, with the module id each stands for. Taken from
// SnomedUtilities.getCodeFromAlias in org.hl7.fhir.model, identical in 6.10.4
// and 7.0.6. Mapped here rather than passed on by name so that a misspelt name
// fails before the JVM starts: the validator takes a name it does not know as
// a module id and sends it to the terminology server as is.
//
// Upstream maps "ca" and "ca-fr" to the same module; the table keeps that.
var snomedEditions = map[string]string{
	"intl":        "900000000000207008",
	"us":          "731000124108",
	"us+icd10cm":  "5991000124107",
	"uk/clinical": "999000021000000109",
	"uk":          "83821000000107",
	"au":          "32506021000036107",
	"at":          "11000234105",
	"ca":          "20611000087101",
	"ca-en":       "20621000087109",
	"ca-fr":       "20611000087101",
	"nl":          "11000146104",
	"se":          "45991000052106",
	"es":          "449081005",
	"es-es":       "900000001000122104",
	"ar":          "11000221109",
	"dk":          "554471000005108",
	"be":          "11000172109",
	"ee":          "11000181102",
	"fi":          "11000229106",
	"de":          "11000274103",
	"in":          "1121000189102",
	"ie":          "11000220105",
	"nz":          "21000210109",
	"no":          "51000202101",
	"kr":          "11000267109",
	"ch":          "2011000195101",
	"uy":          "5631000179106",
}

// SnomedEditionNames lists the edition names --snomed-edition accepts, sorted.
func SnomedEditionNames() []string {
	names := make([]string, 0, len(snomedEditions))
	for n := range snomedEditions {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// snomedEditionModule resolves --snomed-edition to the module id the validator
// gets as -sct: a name from snomedEditions, in any case, or a module id
// itself, which must be a well-formed SNOMED CT identifier. Empty stays empty.
func snomedEditionModule(edition string) (string, error) {
	if edition == "" {
		return "", nil
	}
	if id, ok := snomedEditions[strings.ToLower(edition)]; ok {
		return id, nil
	}
	if isSCTID(edition) {
		return edition, nil
	}
	return "", fmt.Errorf("--snomed-edition %q is neither an edition name nor a SNOMED CT module id — names: %s",
		edition, strings.Join(SnomedEditionNames(), ", "))
}

// validateSnomedEdition rejects a --snomed-edition the validator would take
// for a module id that does not exist.
func validateSnomedEdition(edition string) error {
	_, err := snomedEditionModule(edition)
	return err
}

// isSCTID reports whether s has the shape of a SNOMED CT identifier: 6 to 18
// digits, no leading zero, and a valid Verhoeff check digit at the end. A
// mistyped module id fails the check digit far more often than not.
func isSCTID(s string) bool {
	if len(s) < 6 || len(s) > 18 || s[0] == '0' {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return verhoeffValid(s)
}

// The Verhoeff tables: d is the multiplication table of the dihedral group
// D5, p the position-dependent permutation.
var (
	verhoeffD = [10][10]byte{
		{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
		{1, 2, 3, 4, 0, 6, 7, 8, 9, 5},
		{2, 3, 4, 0, 1, 7, 8, 9, 5, 6},
		{3, 4, 0, 1, 2, 8, 9, 5, 6, 7},
		{4, 0, 1, 2, 3, 9, 5, 6, 7, 8},
		{5, 9, 8, 7, 6, 0, 4, 3, 2, 1},
		{6, 5, 9, 8, 7, 1, 0, 4, 3, 2},
		{7, 6, 5, 9, 8, 2, 1, 0, 4, 3},
		{8, 7, 6, 5, 9, 3, 2, 1, 0, 4},
		{9, 8, 7, 6, 5, 4, 3, 2, 1, 0},
	}
	verhoeffP = [8][10]byte{
		{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
		{1, 5, 7, 6, 2, 8, 3, 0, 9, 4},
		{5, 8, 0, 3, 7, 9, 6, 1, 4, 2},
		{8, 9, 1, 6, 0, 4, 3, 5, 2, 7},
		{9, 4, 5, 3, 1, 2, 6, 8, 7, 0},
		{4, 2, 8, 6, 5, 7, 3, 9, 0, 1},
		{2, 7, 9, 3, 8, 0, 6, 4, 1, 5},
		{7, 0, 4, 6, 9, 1, 3, 2, 5, 8},
	}
)

// verhoeffValid checks the Verhoeff check digit of a string of digits, the
// scheme SNOMED CT identifiers use.
func verhoeffValid(digits string) bool {
	var c byte
	for i := 0; i < len(digits); i++ {
		n := digits[len(digits)-1-i] - '0'
		c = verhoeffD[c][verhoeffP[i%8][n]]
	}
	return c == 0
}
