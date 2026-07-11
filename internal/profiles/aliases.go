package profiles

import (
	"fmt"
	"sort"
)

// Aliases maps short names to FHIR package references (name#version).
//
// Versions are pinned to a sensible default (the registry's current release at
// the time of writing). Aliases are a convenience shortcut — pass the full
// name#version reference directly to target a different version.
var Aliases = map[string]string{
	// German profiles
	"kbv-basis":   "kbv.basis#1.5.0",
	"kbv-patient": "kbv.basis#1.5.0",
	"mii":         "de.medizininformatikinitiative.kerndatensatz#2024.0.0",
	"diga":        "de.bfarm.diga#1.2.0",

	// International profiles
	"us-core": "hl7.fhir.us.core#9.0.0",    // US Core (HL7 US Realm)
	"ips":     "hl7.fhir.uv.ips#2.0.1",     // International Patient Summary
	"ipa":     "hl7.fhir.uv.ipa#1.1.0",     // International Patient Access
	"uk-core": "fhir.r4.ukcore.stu2#2.0.2", // UK Core (NHS England, STU2)
}

func Resolve(alias string) string {
	if resolved, ok := Aliases[alias]; ok {
		return resolved
	}
	return alias
}

func List() {
	fmt.Println("Built-in profile aliases:")
	fmt.Println()
	aliases := make([]string, 0, len(Aliases))
	for alias := range Aliases {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	for _, alias := range aliases {
		fmt.Printf("  %-20s → %s\n", alias, Aliases[alias])
	}
}
