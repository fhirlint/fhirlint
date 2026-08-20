package profiles

import (
	"slices"
	"strings"
	"testing"
)

func TestResolve_KnownAliases(t *testing.T) {
	cases := []struct {
		alias string
		want  []string
	}{
		{"kbv-basis", []string{"kbv.basis#1.5.0"}},
		{"kbv-patient", []string{"kbv.basis#1.5.0"}},
		{"diga", []string{"de.bfarm.diga#1.2.0"}},
		{"mii-person", []string{"de.medizininformatikinitiative.kerndatensatz.person#2025.0.1"}},
		{"mii-laborbefund", []string{"de.medizininformatikinitiative.kerndatensatz.laborbefund#2026.0.3"}},
		{"us-core", []string{"hl7.fhir.us.core#9.0.0"}},
		{"ips", []string{"hl7.fhir.uv.ips#2.0.1"}},
		{"ipa", []string{"hl7.fhir.uv.ipa#1.1.0"}},
		{"uk-core", []string{"fhir.r4.ukcore.stu2#2.0.2"}},
	}
	for _, tc := range cases {
		t.Run(tc.alias, func(t *testing.T) {
			got := Resolve(tc.alias)
			if !slices.Equal(got, tc.want) {
				t.Errorf("Resolve(%q) = %q, want %q", tc.alias, got, tc.want)
			}
		})
	}
}

// The MII Kerndatensatz has no umbrella package, so `mii` is only meaningful as
// a set of modules. This is the case the multi-package alias exists for (#334).
func TestResolve_MIIExpandsToModules(t *testing.T) {
	got := Resolve("mii")
	if len(got) < 2 {
		t.Fatalf("Resolve(\"mii\") = %q, want several module packages", got)
	}
	for _, pkg := range got {
		if !strings.HasPrefix(pkg, "de.medizininformatikinitiative.kerndatensatz.") {
			t.Errorf("Resolve(\"mii\") contains %q, which is not a Kerndatensatz module", pkg)
		}
	}
	if !Expands("mii") {
		t.Error("Expands(\"mii\") = false, want true")
	}
}

// Every per-module alias must name exactly the reference the aggregate uses.
// Two pins for the same module would load two versions of it in one run.
func TestAliases_ModuleAliasesMatchAggregate(t *testing.T) {
	aggregate := Resolve("mii")
	for alias, pkgs := range Aliases {
		if !strings.HasPrefix(alias, "mii-") {
			continue
		}
		if len(pkgs) != 1 {
			t.Errorf("alias %q resolves to %d packages, want exactly 1", alias, len(pkgs))
			continue
		}
		if !slices.Contains(aggregate, pkgs[0]) {
			t.Errorf("alias %q resolves to %q, which is not in the mii aggregate %q", alias, pkgs[0], aggregate)
		}
	}
}

func TestExpands_SinglePackageAndUnknown(t *testing.T) {
	if Expands("kbv-basis") {
		t.Error("Expands(\"kbv-basis\") = true, want false for a single-package alias")
	}
	if Expands("custom.package#1.0.0") {
		t.Error("Expands on an unknown value = true, want false")
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
			if !slices.Equal(got, []string{input}) {
				t.Errorf("Resolve(%q) = %q, want passthrough [%q]", input, got, input)
			}
		})
	}
}

// Resolve hands out a copy: a caller appending to the returned slice must not
// be able to reach into the alias table.
func TestResolve_ReturnsCopy(t *testing.T) {
	got := Resolve("mii")
	original := Resolve("mii")
	got[0] = "tampered"
	if !slices.Equal(Resolve("mii"), original) {
		t.Errorf("mutating a Resolve result changed the table: %q", Resolve("mii"))
	}
}

func TestResolveAll_FlattensAndPassesThrough(t *testing.T) {
	got := ResolveAll([]string{"us-core", "mii-person", "custom.package#1.0.0"})
	want := []string{
		"hl7.fhir.us.core#9.0.0",
		"de.medizininformatikinitiative.kerndatensatz.person#2025.0.1",
		"custom.package#1.0.0",
	}
	if !slices.Equal(got, want) {
		t.Errorf("ResolveAll = %q, want %q", got, want)
	}

	multi := ResolveAll([]string{"mii"})
	if len(multi) != len(Resolve("mii")) {
		t.Errorf("ResolveAll([mii]) returned %d packages, want %d", len(multi), len(Resolve("mii")))
	}

	if got := ResolveAll(nil); len(got) != 0 {
		t.Errorf("ResolveAll(nil) = %q, want empty", got)
	}
}

func TestAliases_AllHaveNonEmptyValues(t *testing.T) {
	for alias, pkgs := range Aliases {
		if len(pkgs) == 0 {
			t.Errorf("alias %q resolves to no package at all", alias)
			continue
		}
		for _, pkg := range pkgs {
			if strings.TrimSpace(pkg) == "" {
				t.Errorf("alias %q has an empty package reference", alias)
			}
			if !strings.Contains(pkg, "#") {
				t.Errorf("alias %q package %q missing version (expected name#version)", alias, pkg)
			}
		}
	}
}

// A single alias must not name the same package twice: the JAR would load it
// twice, and the duplicate would be silent.
func TestAliases_NoDuplicateWithinAnAlias(t *testing.T) {
	for alias, pkgs := range Aliases {
		seen := map[string]bool{}
		for _, pkg := range pkgs {
			if seen[pkg] {
				t.Errorf("alias %q names %q more than once", alias, pkg)
			}
			seen[pkg] = true
		}
	}
}
