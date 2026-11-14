// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package validator

import (
	"strings"
	"testing"
	"unicode"
)

var usCorePaths = []string{"in/us.json", "in/us2.json"}

// One US Core Patient without -ig, recorded with both validator lines (#520).
func TestImplicitPackages_RecordedStdout(t *testing.T) {
	for _, v := range []string{"6.10.3", "7.0.3"} {
		t.Run(v, func(t *testing.T) {
			got := implicitPackages(readFixture(t, "stdout-implicit-packages-"+v+".txt"), usCorePaths)
			if len(got[1]) != 0 {
				t.Errorf("us2.json loaded nothing new, got %v", got[1])
			}
			refs := got[0]
			if len(refs) != 15 {
				t.Fatalf("got %d packages, want 15: %v", len(refs), refs)
			}
			byName := map[string]PackageRef{}
			for _, r := range refs {
				byName[r.String()] = r
			}
			for _, want := range []string{"hl7.fhir.us.core#9.0.0", "us.cdc.phinvads#0.12.0", "hl7.terminology.r4#7.1.0"} {
				if _, ok := byName[want]; !ok {
					t.Errorf("%s missing", want)
				}
			}
			for name, pre := range map[string]bool{
				"hl7.fhir.uv.extensions#5.1.0-snapshot1":     true,
				"hl7.fhir.uv.extensions.r4#5.3.0-ballot-tc1": true,
				"hl7.fhir.us.core#9.0.0":                     false,
			} {
				if byName[name].PreRelease != pre {
					t.Errorf("%s: preRelease = %v", name, byName[name].PreRelease)
				}
			}
			// In the startup summary, so requested or always added.
			if _, ok := byName["hl7.terminology.r4#6.2.0"]; ok {
				t.Error("a startup package was counted as implicit")
			}
		})
	}
}

// A package from the summary that the validator loads again later is not
// implicit; a local directory is never a package.
func TestImplicitPackages_SummaryAndLocal(t *testing.T) {
	stdout := "  Load a.core#1.0 - 5 resources (00:00.1)\n" +
		"  Package Summary: [a.core#1.0, b.ext#2.0]\n" +
		"  Validate x.json\n" +
		"  Load b.ext#2.0 - 3 resources (00:00.1)\n" +
		"  Load ./ig - 1 resources (00:00.1)\n" +
		"  Load c.new#3.0.0-ballot - 3 resources (00:00.1)\n" +
		"  Load c.new#3.0.0-ballot - 3 resources (00:00.1)\n"
	got := implicitPackages(stdout, []string{"x.json"})
	if len(got[0]) != 1 || got[0][0] != (PackageRef{ID: "c.new", Version: "3.0.0-ballot", PreRelease: true}) {
		t.Errorf("got %v", got)
	}
}

func TestImplicitPackages_RunUnion(t *testing.T) {
	a, b := PackageRef{ID: "b", Version: "1"}, PackageRef{ID: "a", Version: "2"}
	got := ImplicitPackages([]*Result{{ImplicitPackages: []PackageRef{a, b}}, nil, {ImplicitPackages: []PackageRef{a}}})
	if len(got) != 2 || got[0] != b || got[1] != a {
		t.Errorf("got %v", got)
	}
}

// FuzzImplicitPackages: arbitrary stdout never panics, and every package
// belongs to a file of the run and has no control character in it.
func FuzzImplicitPackages(f *testing.F) {
	f.Add("  Package Summary: [a#1]\n  Validate a.json\n  Load b#2 - 1 resources\n")
	f.Add("  Validate b.json\n\x1b[31m  Load x\x07y#1-pre - 9 resources\n")
	paths := []string{"a.json", "b.json"}
	f.Fuzz(func(t *testing.T, stdout string) {
		for i, refs := range implicitPackages(stdout, paths) {
			if i < 0 || i >= len(paths) {
				t.Fatalf("index %d", i)
			}
			for _, r := range refs {
				if strings.ContainsFunc(r.String(), unicode.IsControl) {
					t.Fatalf("control character in %q", r.String())
				}
			}
		}
	})
}
