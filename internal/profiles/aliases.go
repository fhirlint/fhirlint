package profiles

import (
	"fmt"
	"sort"
	"strings"
)

// Aliases maps short names to the FHIR package references they stand for.
//
// Most aliases are one package. Some cannot be: the MII Kerndatensatz has no
// umbrella package on the registry — it ships module by module — so `mii` only
// means anything as a set. That is why an alias expands to a list rather than
// to a single reference (#334).
//
// Versions are pinned to a sensible default (the registry's current release at
// the time of writing). Aliases are a convenience shortcut — pass the full
// name#version reference directly to target a different version.
var Aliases = map[string][]string{
	// German profiles
	"kbv-basis":   {"kbv.basis#1.9.0"},
	"kbv-patient": {"kbv.basis#1.9.0"},

	// DiGA profiles are published by the KBV as the MIO DiGA Toolkit, not by the
	// BfArM: there is no de.bfarm.diga package on any registry, and this alias
	// pointed at one until #335. kbv.mio.diga depends on kbv.basis 1.3.0, so a
	// run combining `diga` with `kbv-basis` loads two versions of kbv.basis.
	"diga": {"kbv.mio.diga#1.1.0"},

	// MII Kerndatensatz, module by module. The MII publishes no umbrella
	// package, so `mii` only means anything as a set (#334).
	//
	// person, fall, diagnose and prozedur are gone: they stop at 2025.0.1 and
	// their profiles moved wholesale into kerndatensatz.base for the 2026 train
	// ("Basismodule Person, Diagnose, Prozedur, Fall v2026+"). The ten profiles
	// in base are exactly the four modules' ten. The 2026 modules already
	// depend on base rather than on them, so the set now sits on one train and
	// pulls one version of kerndatensatz.meta — the split documented here
	// before #398 was a supersession mistaken for a release-train difference.
	//
	// Versions are dist-tags.latest, not the highest number published — the
	// rule the uk-core comment records.
	"mii": {
		"de.medizininformatikinitiative.kerndatensatz.base#2026.0.1",
		"de.medizininformatikinitiative.kerndatensatz.laborbefund#2026.0.3",
		"de.medizininformatikinitiative.kerndatensatz.medikation#2026.0.1",
		"de.medizininformatikinitiative.kerndatensatz.icu#2026.0.2",
		"de.medizininformatikinitiative.kerndatensatz.onkologie#2026.0.3",
		"de.medizininformatikinitiative.kerndatensatz.biobank#2026.0.1",
		"de.medizininformatikinitiative.kerndatensatz.consent#2026.0.0",
		"de.medizininformatikinitiative.kerndatensatz.molgen#2026.0.4",
		"de.medizininformatikinitiative.kerndatensatz.patho#2026.0.2",
		"de.medizininformatikinitiative.kerndatensatz.studie#2026.0.2",
		"de.medizininformatikinitiative.kerndatensatz.mikrobiologie#2025.0.2",
		"de.medizininformatikinitiative.kerndatensatz.bildgebung#2026.0.0",
	},
	// base carries Patient, Encounter, Condition, Procedure and Vitalstatus —
	// what mii-person, mii-fall, mii-diagnose and mii-prozedur used to name.
	"mii-base":          {"de.medizininformatikinitiative.kerndatensatz.base#2026.0.1"},
	"mii-laborbefund":   {"de.medizininformatikinitiative.kerndatensatz.laborbefund#2026.0.3"},
	"mii-medikation":    {"de.medizininformatikinitiative.kerndatensatz.medikation#2026.0.1"},
	"mii-icu":           {"de.medizininformatikinitiative.kerndatensatz.icu#2026.0.2"},
	"mii-onkologie":     {"de.medizininformatikinitiative.kerndatensatz.onkologie#2026.0.3"},
	"mii-biobank":       {"de.medizininformatikinitiative.kerndatensatz.biobank#2026.0.1"},
	"mii-consent":       {"de.medizininformatikinitiative.kerndatensatz.consent#2026.0.0"},
	"mii-molgen":        {"de.medizininformatikinitiative.kerndatensatz.molgen#2026.0.4"},
	"mii-patho":         {"de.medizininformatikinitiative.kerndatensatz.patho#2026.0.2"},
	"mii-studie":        {"de.medizininformatikinitiative.kerndatensatz.studie#2026.0.2"},
	"mii-mikrobiologie": {"de.medizininformatikinitiative.kerndatensatz.mikrobiologie#2025.0.2"},
	"mii-bildgebung":    {"de.medizininformatikinitiative.kerndatensatz.bildgebung#2026.0.0"},

	// gematik ISiK — the interoperability profiles German hospitals implement.
	//
	// One package, not five. The per-module de.gematik.isik-* packages are
	// frozen at 4.0.x, which is ISiK Stufe 4; the spec tags in
	// gematik/spec-ISiK-Basismodul make the mapping explicit (v.5.0.0 is
	// "ISiK Stufe 5", v.6.0.0 is "ISiK Stufe 6"). From Stufe 5 on, everything
	// ships as de.gematik.isik, which at 6.0.0 carries 178 profiles across 39
	// resource types — the five modules held 131 between them — and adds the
	// ICU and laboratory content that never had its own published package.
	//
	// There are therefore no isik-<module> aliases any more: there is nothing
	// left to narrow a load to, and an alias that pretended otherwise would
	// resolve to the whole package anyway (#398).
	"isik": {"de.gematik.isik#6.0.0"},

	// International profiles
	"us-core": {"hl7.fhir.us.core#9.0.0"}, // US Core (HL7 US Realm)
	"ips":     {"hl7.fhir.uv.ips#2.0.1"},  // International Patient Summary
	"ipa":     {"hl7.fhir.uv.ipa#1.1.0"},  // International Patient Access
	// UK Core: the registry serves a 2.1.0, but dist-tags.latest is still 2.0.2 on
	// both packages.fhir.org and packages.simplifier.net, so 2.1.0 is not (yet)
	// the published current release. Pin the tag, not the highest number.
	"uk-core": {"fhir.r4.ukcore.stu2#2.0.2"}, // UK Core (NHS England, STU2)
}

// Resolve returns the package references an alias stands for. Anything that is
// not a known alias passes through unchanged as a single element, so a full
// name#version reference or a canonical URL works wherever an alias does.
//
// The returned slice is a copy: callers append to their own result lists, and
// handing out the table's backing array would let one of them corrupt it.
func Resolve(alias string) []string {
	resolved, ok := Aliases[alias]
	if !ok {
		return []string{alias}
	}
	out := make([]string, len(resolved))
	copy(out, resolved)
	return out
}

// ResolveAll resolves every value in a list, flattening aliases that stand for
// several packages. This is what every caller that accepts repeated --profile
// or --ig values wants.
func ResolveAll(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, Resolve(v)...)
	}
	return out
}

// Expands reports whether an alias stands for more than one package. Callers
// that can only handle a single package use it to reject the alias with a
// message naming what it expands to, rather than silently taking the first.
func Expands(alias string) bool {
	return len(Aliases[alias]) > 1
}

func List() {
	fmt.Println("Built-in profile aliases:")
	fmt.Println()
	aliases := make([]string, 0, len(Aliases))
	for alias := range Aliases {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)

	const indent = "  "
	// Derived, not fixed: a hardcoded width silently breaks the arrow column as
	// soon as an alias outgrows it, which isik-dokumentenaustausch did (#368).
	width := 0
	for _, alias := range aliases {
		width = max(width, len(alias))
	}

	for _, alias := range aliases {
		pkgs := Aliases[alias]
		fmt.Printf("%s%-*s → %s\n", indent, width, alias, pkgs[0])
		// A multi-package alias lists the rest under the first, aligned with it,
		// so the arrow column still reads as one entry per line.
		for _, pkg := range pkgs[1:] {
			fmt.Printf("%s%s   %s\n", indent, strings.Repeat(" ", width), pkg)
		}
	}
}
