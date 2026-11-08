// Package pkgindex answers "which installed package defines this canonical?"
// from the package/.index.json files in the FHIR package cache (#523).
//
// "The profile could not be found" is the most common newcomer problem with
// the validator, and the answer is always the same: load the package that
// defines it with --ig. The cache often has that package already, from an
// earlier run or another project.
package pkgindex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fhirlint/fhirlint/internal/fhirpkg"
)

// Index maps canonical URLs, with and without "|version", to the packages
// ("id#version") that define them.
type Index struct {
	byURL map[string][]string
}

// Build reads the index of every package in the cache. A package without a
// readable index is skipped: the cache is written by other tools, and one bad
// entry must not hide the rest. A missing cache gives an empty index.
func Build() *Index {
	ix := &Index{byURL: map[string][]string{}}
	root, err := fhirpkg.CacheRoot()
	if err != nil {
		return ix
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return ix
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.Contains(e.Name(), "#") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, e.Name(), "package", ".index.json")) //nolint:gosec // under the package cache root
		if err != nil {
			continue
		}
		for _, key := range parseIndex(data) {
			ix.add(key, e.Name())
		}
	}
	return ix
}

func (ix *Index) add(key, pkg string) {
	for _, p := range ix.byURL[key] {
		if p == pkg {
			return
		}
	}
	ix.byURL[key] = append(ix.byURL[key], pkg)
}

// parseIndex returns the lookup keys of one .index.json: each resource's url,
// and url|version when it states a version.
func parseIndex(data []byte) []string {
	var idx struct {
		Files []struct {
			URL     string `json:"url"`
			Version string `json:"version"`
		} `json:"files"`
	}
	if json.Unmarshal(data, &idx) != nil {
		return nil
	}
	var keys []string
	for _, f := range idx.Files {
		if f.URL == "" {
			continue
		}
		keys = append(keys, f.URL)
		if f.Version != "" {
			keys = append(keys, f.URL+"|"+f.Version)
		}
	}
	return keys
}

// Lookup returns the packages that define canonical, best first: final
// releases before pre-releases, then the highest version. A canonical with
// "|version" matches that version when some package has it, and any version
// otherwise.
func (ix *Index) Lookup(canonical string) []string {
	found := ix.byURL[canonical]
	if len(found) == 0 {
		if url, _, ok := strings.Cut(canonical, "|"); ok {
			found = ix.byURL[url]
		}
	}
	out := append([]string(nil), found...)
	sort.SliceStable(out, func(i, j int) bool { return better(out[i], out[j]) })
	return out
}

// better orders two package ids for a suggestion: a final release before a
// pre-release, a higher version before a lower one, and otherwise by name.
func better(a, b string) bool {
	an, av := fhirpkg.SplitID(a)
	bn, bv := fhirpkg.SplitID(b)
	if ap, bp := fhirpkg.IsPreRelease(av), fhirpkg.IsPreRelease(bv); ap != bp {
		return !ap
	}
	if an == bn {
		if cmp, ok := fhirpkg.CompareVersions(av, bv); ok && cmp != 0 {
			return cmp > 0
		}
	}
	return a < b
}
