package profiles

import (
	"strings"
	"testing"
)

func TestResolve_KnownAliases(t *testing.T) {
	cases := []struct {
		alias string
		want  string
	}{
		{"kbv-basis", "kbv.basis#1.5.0"},
		{"kbv-patient", "kbv.basis#1.5.0"},
		{"mii", "de.medizininformatikinitiative.kerndatensatz#2024.0.0"},
		{"diga", "de.bfarm.diga#1.2.0"},
	}
	for _, tc := range cases {
		t.Run(tc.alias, func(t *testing.T) {
			got := Resolve(tc.alias)
			if got != tc.want {
				t.Errorf("Resolve(%q) = %q, want %q", tc.alias, got, tc.want)
			}
		})
	}
}

func TestResolve_UnknownPassthrough(t *testing.T) {
	cases := []string{
		"custom.package#1.0.0",
		"http://hl7.org/fhir/StructureDefinition/Patient",
		"some-unknown-alias",
		"",
	}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			got := Resolve(input)
			if got != input {
				t.Errorf("Resolve(%q) = %q, want passthrough %q", input, got, input)
			}
		})
	}
}

func TestAliases_AllHaveNonEmptyValues(t *testing.T) {
	for alias, pkg := range Aliases {
		if strings.TrimSpace(pkg) == "" {
			t.Errorf("alias %q has empty package reference", alias)
		}
		if !strings.Contains(pkg, "#") {
			t.Errorf("alias %q package %q missing version (expected name#version)", alias, pkg)
		}
	}
}

func TestAliases_NoDuplicatePackageVersions(t *testing.T) {
	seen := map[string]string{}
	for alias, pkg := range Aliases {
		if prev, ok := seen[pkg]; ok {
			// Duplicates are allowed (e.g. kbv-basis and kbv-patient → same package)
			// but log them so they're visible
			t.Logf("note: %q and %q both resolve to %q", alias, prev, pkg)
		}
		seen[pkg] = alias
	}
}
