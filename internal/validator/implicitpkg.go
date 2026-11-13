package validator

import (
	"regexp"
	"sort"
	"strings"

	"github.com/fhirlint/fhirlint/internal/fhirpkg"
)

// PackageRef is a package the validator loaded that nobody asked for (#520).
type PackageRef struct {
	ID         string `json:"id"`
	Version    string `json:"version"`
	PreRelease bool   `json:"preRelease,omitempty"`
}

// String is the package as --ig and fhirlint.lock name it, "id#version".
func (p PackageRef) String() string { return p.ID + "#" + p.Version }

// loadLine is how the validator reports loading a package, before and during
// validation alike: "  Load hl7.fhir.us.core#9.0.0 - 215 resources (00:00.002)".
// A local directory ("Load ./ig - 1 resources") has no '#' and does not match.
var loadLine = regexp.MustCompile(`^\s*Load ([^\s#]+)#(\S+) - \d+ resources`)

// packageSummaryLine lists what the validator loaded at startup: the
// requested packages and the core packages it always adds.
var packageSummaryLine = regexp.MustCompile(`^\s*Package Summary: \[(.*)\]\s*$`)

// implicitPackages finds the packages the validator loaded while validating:
// a canonical it met in an instance made it load the package that defines it,
// in the newest version, pre-releases included. They are the Load lines after
// the first "Validate <file>" line that are not in the startup Package
// Summary, keyed by the index into paths of the file the validator was on.
//
// 6.10.3 and 7.0.3 print the same lines. For one US Core Patient without -ig,
// both load 15 packages after the summary, among them hl7.fhir.us.core#9.0.0
// and the pre-releases hl7.fhir.uv.extensions#5.1.0-snapshot1 and
// hl7.fhir.uv.extensions.r4#5.3.0-ballot-tc1.
func implicitPackages(stdout string, paths []string) map[int][]PackageRef {
	startup := map[string]bool{}
	out := map[int][]PackageRef{}
	seen := map[string]bool{}
	current := -1
	validating := false
	ix := newPathIndex(paths)
	for _, l := range strings.Split(stripANSI(stdout), "\n") {
		if m := validateLine.FindStringSubmatch(l); m != nil {
			validating = true
			current = ix.lookup(m[1])
			continue
		}
		if !validating {
			if m := packageSummaryLine.FindStringSubmatch(l); m != nil {
				for _, p := range strings.Split(m[1], ",") {
					startup[strings.TrimSpace(p)] = true
				}
			}
			if m := loadLine.FindStringSubmatch(l); m != nil {
				startup[m[1]+"#"+m[2]] = true
			}
			continue
		}
		m := loadLine.FindStringSubmatch(l)
		if m == nil || current < 0 {
			continue
		}
		ref := PackageRef{ID: plainText(m[1]), Version: plainText(m[2])}
		ref.PreRelease = fhirpkg.IsPreRelease(ref.Version)
		if startup[ref.String()] || seen[ref.String()] {
			continue
		}
		seen[ref.String()] = true
		out[current] = append(out[current], ref)
	}
	return out
}

// attachImplicitPackages records on each result the packages loaded while
// the validator was on that file.
func attachImplicitPackages(results []*Result, loaded map[int][]PackageRef) {
	for i, refs := range loaded {
		if i < len(results) && results[i] != nil {
			results[i].ImplicitPackages = refs
		}
	}
}

// ImplicitPackages is the run's implicit packages across all results, sorted
// and without duplicates. A cached result contributes nothing: its run did not
// happen now.
func ImplicitPackages(results []*Result) []PackageRef {
	seen := map[string]bool{}
	var out []PackageRef
	for _, r := range results {
		if r == nil {
			continue
		}
		for _, p := range r.ImplicitPackages {
			if !seen[p.String()] {
				seen[p.String()] = true
				out = append(out, p)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Version < out[j].Version
	})
	return out
}
