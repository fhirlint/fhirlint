// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package coverage

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func elems(populated, missing, unresolved int, reason string) []ElementReport {
	var out []ElementReport
	for i := 0; i < populated; i++ {
		out = append(out, ElementReport{ID: fmt.Sprintf("Patient.p%d", i), Populated: true})
	}
	for i := 0; i < missing; i++ {
		out = append(out, ElementReport{ID: fmt.Sprintf("Patient.m%d", i)})
	}
	for i := 0; i < unresolved; i++ {
		out = append(out, ElementReport{ID: fmt.Sprintf("Patient.u%d", i), Unresolved: true, Reason: reason})
	}
	return out
}

func TestTerminal_ProfileFigures(t *testing.T) {
	rep := Report{
		ResourcesScanned: 5,
		Unattributed:     1,
		Profiles: []ProfileReport{{
			Name: "MyPatient", Type: "Patient", Resources: 4, ByType: 2,
			Elements: elems(3, 12, 2, "value set http://x not available"),
			Warnings: []string{"snapshot missing"},
		}},
		ProfilesWithoutResources: 2,
		SkippedFiles:             []SkippedFile{{Path: "a.xml", Reason: "not JSON"}},
	}
	out := Terminal(rep, false)
	for _, want := range []string{
		"MyPatient  (Patient)",
		"(2 matched by resource type, not by meta.profile)",
		"must-support:   3/15  (20%)",
		"never populated:",
		"… and 2 more (use --verbose for the full list)",
		"not measurable: 2 element(s) — value set http://x not available",
		"! snapshot missing",
		"5 resource(s) scanned, 1 not attributed to any profile",
		"2 profile(s) had no matching resource and were not measured",
		"1 file(s) skipped:",
		"a.xml — not JSON",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if verbose := Terminal(rep, true); strings.Contains(verbose, "more (use --verbose") || !strings.Contains(verbose, "Patient.m11") {
		t.Errorf("--verbose lists every missing element:\n%s", verbose)
	}
}

func TestTerminal_SeveralUnresolvedReasonsAndNothingMeasurable(t *testing.T) {
	p := ProfileReport{Name: "P", Type: "Patient", Resources: 1}
	p.Elements = append(elems(0, 0, 1, "reason A"), elems(0, 0, 1, "reason B")...)
	out := Terminal(Report{Profiles: []ProfileReport{p}, ResourcesScanned: 1}, false)
	for _, want := range []string{"must-support:   0/0 measurable of 2", "not measurable: 2 element(s)\n", "      reason A", "      reason B"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if got := unresolvedReasons(ProfileReport{Elements: elems(0, 0, 1, "")}); len(got) != 1 || !strings.Contains(got[0], "slice membership") {
		t.Errorf("an element without a reason gets the generic one: %v", got)
	}
}

func TestTerminal_NoProfilesAndManySkipped(t *testing.T) {
	rep := Report{ResourcesScanned: 3, Unattributed: 3}
	for i := 0; i < maxListed+2; i++ {
		rep.SkippedFiles = append(rep.SkippedFiles, SkippedFile{Path: fmt.Sprintf("f%d.json", i), Reason: "unreadable"})
	}
	out := Terminal(rep, false)
	for _, want := range []string{
		"No profile with mustSupport elements matched the scanned resources.",
		"hint: resources declare no matching meta.profile",
		"… and 2 more",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestJSON_CarriesTheDerivedFigures(t *testing.T) {
	rep := Report{ResourcesScanned: 2, Profiles: []ProfileReport{{Name: "P", Type: "Patient", Resources: 2, Elements: elems(1, 2, 1, "r")}}}
	b, err := JSON(rep)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Profiles []struct {
			Covered, Measurable int
			UnresolvedCount     int     `json:"unresolvedCount"`
			Percent             float64 `json:"percent"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	p := got.Profiles[0]
	if p.Covered != 1 || p.Measurable != 3 || p.UnresolvedCount != 1 || p.Percent != 33.3 {
		t.Errorf("derived figures: %+v\n%s", p, b)
	}
}

func TestLowestPercent(t *testing.T) {
	if got := LowestPercent(Report{}); got != 100 {
		t.Errorf("no profiles: %v, want 100", got)
	}
	rep := Report{Profiles: []ProfileReport{
		{Elements: elems(1, 1, 0, "")},
		{Elements: elems(1, 3, 0, "")},
		{Elements: elems(0, 0, 2, "r")},
	}}
	if got := LowestPercent(rep); got != 25 {
		t.Errorf("lowest: %v, want 25 (a profile with nothing measurable doesn't count)", got)
	}
}
