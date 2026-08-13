package validator

import "strings"

// FHIRVersion describes one FHIR release fhirlint accepts.
type FHIRVersion struct {
	ID          string // what --fhir-version takes, e.g. "4.0.1"
	Name        string // release name, e.g. "R4"
	TxPath      string // path segment the terminology server serves it under
	CorePackage string // the hl7.fhir.rX.core package for this release
}

// FHIRVersions is the single source of truth for which FHIR releases fhirlint
// supports, in ascending order.
//
// The --fhir-version gate, the release names `fhirlint version` prints, the
// default terminology endpoint, every flag help string, every shell completion
// and the fhirlint.yml schema are all derived from this table. Adding a release
// used to mean finding eleven hardcoded copies of the same list; now it is one
// entry here, `go run . config schema > fhirlint.schema.json`, and the
// --fhir-version row in README.md — which is prose and cannot be derived, so a
// test checks it against this table instead (#306).
//
// TxPath and CorePackage are spelled out rather than derived from Name,
// because for R4B they disagree: there is an hl7.fhir.r4b.core package, but
// tx.fhir.org serves no /r4b endpoint and the JAR maps R4B to /r4 — confirmed
// against the JAR. Deriving either from the other would be right twice and
// wrong once. A release added here needs both verified rather than guessed.
var FHIRVersions = []FHIRVersion{
	{ID: "4.0.1", Name: "R4", TxPath: "/r4", CorePackage: "hl7.fhir.r4.core"},
	{ID: "4.3.0", Name: "R4B", TxPath: "/r4", CorePackage: "hl7.fhir.r4b.core"},
	{ID: "5.0.0", Name: "R5", TxPath: "/r5", CorePackage: "hl7.fhir.r5.core"},
}

// DefaultFHIRVersion is what fhirlint validates against when nothing is
// specified. It is deliberately not "the newest": R4 is what most published
// implementation guides still target, so it is the one that surprises fewest
// users.
const DefaultFHIRVersion = "4.0.1"

// FHIRVersionIDs returns the accepted version strings, in table order.
// Suitable for shell completions and enum definitions.
func FHIRVersionIDs() []string {
	out := make([]string, len(FHIRVersions))
	for i, v := range FHIRVersions {
		out[i] = v.ID
	}
	return out
}

// FHIRVersionList renders the accepted versions for flag help and error
// messages: "4.0.1, 4.3.0, 5.0.0".
func FHIRVersionList() string {
	return strings.Join(FHIRVersionIDs(), ", ")
}

// LookupFHIRVersion finds a release by its version string.
func LookupFHIRVersion(id string) (FHIRVersion, bool) {
	for _, v := range FHIRVersions {
		if v.ID == id {
			return v, true
		}
	}
	return FHIRVersion{}, false
}

// FHIRVersionName returns the release name for a version string, and the string
// itself when it names no release fhirlint knows — a version echoed back
// unchanged is more useful than an empty field.
func FHIRVersionName(id string) string {
	if v, ok := LookupFHIRVersion(id); ok {
		return v.Name
	}
	return id
}
