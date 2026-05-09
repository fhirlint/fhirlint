package profiles

import "fmt"

// Aliases maps short names to FHIR package references (name#version).
var Aliases = map[string]string{
	"kbv-basis":   "kbv.basis#1.5.0",
	"kbv-patient": "kbv.basis#1.5.0",
	"mii":         "de.medizininformatikinitiative.kerndatensatz#2024.0.0",
	"diga":        "de.bfarm.diga#1.2.0",
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
	for alias, pkg := range Aliases {
		fmt.Printf("  %-20s → %s\n", alias, pkg)
	}
}
