// Package stats summarises a FHIR dataset: how many resources of each type
// exist, which profiles they declare, and (optionally) an aggregate validation
// summary. It powers the `fhirlint stats` command.
package stats

import "sort"

// noneProfile is the bucket for resources that declare no meta.profile.
const noneProfile = "(none)"

// unknownType is the bucket for files whose resourceType could not be read.
const unknownType = "(unknown)"

// Resource is one FHIR resource's structural fingerprint.
type Resource struct {
	Type     string
	Profiles []string
}

// TypeCount / ProfileCount are sorted histogram rows.
type TypeCount struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

type ProfileCount struct {
	Profile string `json:"profile"`
	Count   int    `json:"count"`
}

// ValidationSummary aggregates validation outcomes across files.
type ValidationSummary struct {
	Files    int `json:"files"`
	Valid    int `json:"valid"`
	Warnings int `json:"warnings"`
	Errors   int `json:"errors"`
}

// Report is the full stats result.
type Report struct {
	TotalResources int                `json:"totalResources"`
	ResourceTypes  []TypeCount        `json:"resourceTypes"`
	Profiles       []ProfileCount     `json:"profiles"`
	Validation     *ValidationSummary `json:"validation,omitempty"`
}

// Compute builds the resource-type and profile histograms from the resources.
// A resource with no declared profile counts towards "(none)"; an unreadable
// resourceType counts towards "(unknown)".
func Compute(resources []Resource) *Report {
	types := map[string]int{}
	profiles := map[string]int{}

	for _, r := range resources {
		t := r.Type
		if t == "" {
			t = unknownType
		}
		types[t]++

		if len(r.Profiles) == 0 {
			profiles[noneProfile]++
			continue
		}
		for _, p := range r.Profiles {
			profiles[p]++
		}
	}

	return &Report{
		TotalResources: len(resources),
		ResourceTypes:  sortTypes(types),
		Profiles:       sortProfiles(profiles),
	}
}

// sortTypes orders by count descending, then name ascending.
func sortTypes(m map[string]int) []TypeCount {
	out := make([]TypeCount, 0, len(m))
	for t, c := range m {
		out = append(out, TypeCount{Type: t, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Type < out[j].Type
	})
	return out
}

// sortProfiles orders by count descending, then name ascending, but always
// places the "(none)" bucket last so it reads as the catch-all.
func sortProfiles(m map[string]int) []ProfileCount {
	out := make([]ProfileCount, 0, len(m))
	for p, c := range m {
		out = append(out, ProfileCount{Profile: p, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Profile == noneProfile {
			return false
		}
		if out[j].Profile == noneProfile {
			return true
		}
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Profile < out[j].Profile
	})
	return out
}
