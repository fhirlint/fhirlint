// Package coverage reports which elements of a profile are actually exercised
// by a set of FHIR resources, with an emphasis on the mustSupport elements.
//
// The validator only ever sees one instance at a time and only checks what is
// present, so it cannot answer the question this package exists for: across a
// whole example set, which parts of the profile has nobody ever populated? An
// IG can ship thirty green examples and still leave half its mustSupport
// elements untouched.
package coverage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fhirlint/fhirlint/internal/fhirpkg"
)

// StructureDefinition is the subset of a FHIR StructureDefinition this package
// needs. Everything else in the resource is ignored.
type StructureDefinition struct {
	URL            string    `json:"url"`
	Name           string    `json:"name"`
	ID             string    `json:"id"`
	Type           string    `json:"type"`
	Kind           string    `json:"kind"`
	Derivation     string    `json:"derivation"`
	BaseDefinition string    `json:"baseDefinition"`
	Snapshot       elemList  `json:"snapshot"`
	Differential   elemList  `json:"differential"`
	Package        string    `json:"-"` // "name#version" the definition was read from
	elementIndex   elemIndex `json:"-"`
}

type elemList struct {
	Element []*Element `json:"element"`
}

// Element is one ElementDefinition.
type Element struct {
	ID          string
	Path        string
	SliceName   string
	MustSupport bool
	Slicing     *Slicing
	Types       []ElementType

	// Pattern and Fixed hold the pattern[x] / fixed[x] value when the element
	// declares one. They are the primary evidence for deciding whether an
	// instance belongs to a slice.
	Pattern json.RawMessage
	Fixed   json.RawMessage

	// BindingValueSet is the value set an element binds to, if any. It is never
	// used to decide slice membership — that would need the value set expanded,
	// which is terminology work and coverage runs offline. It is kept so the
	// report can say why such a slice could not be measured.
	BindingValueSet string
}

// Slicing is an ElementDefinition.slicing.
type Slicing struct {
	Discriminator []Discriminator `json:"discriminator"`
	Rules         string          `json:"rules"`
}

// Discriminator names how instances are told apart within a sliced element.
type Discriminator struct {
	Type string `json:"type"` // value | pattern | type | profile | exists
	Path string `json:"path"` // "$this", "url", "coding.code", …
}

// ElementType is one ElementDefinition.type entry.
type ElementType struct {
	Code    string   `json:"code"`
	Profile []string `json:"profile"`
}

// UnmarshalJSON reads the fields above and, separately, the pattern[x] and
// fixed[x] keys. Those cannot be modelled as struct tags because the type is
// part of the key name — patternIdentifier, fixedUri, patternHumanName and so
// on — so they are picked out of the raw object instead.
func (e *Element) UnmarshalJSON(data []byte) error {
	var plain struct {
		ID          string        `json:"id"`
		Path        string        `json:"path"`
		SliceName   string        `json:"sliceName"`
		MustSupport bool          `json:"mustSupport"`
		Slicing     *Slicing      `json:"slicing"`
		Types       []ElementType `json:"type"`
		Binding     *struct {
			ValueSet string `json:"valueSet"`
		} `json:"binding"`
	}
	if err := json.Unmarshal(data, &plain); err != nil {
		return err
	}
	e.ID, e.Path, e.SliceName = plain.ID, plain.Path, plain.SliceName
	e.MustSupport, e.Slicing, e.Types = plain.MustSupport, plain.Slicing, plain.Types
	if plain.Binding != nil {
		e.BindingValueSet = plain.Binding.ValueSet
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for k, v := range raw {
		switch {
		case isTypedKey(k, "pattern"):
			e.Pattern = v
		case isTypedKey(k, "fixed"):
			e.Fixed = v
		}
	}
	return nil
}

// isTypedKey reports whether key is prefix followed by a capitalised type name,
// i.e. a FHIR polymorphic key such as "patternIdentifier". The capital matters:
// "path" must not be mistaken for a "pattern" key.
func isTypedKey(key, prefix string) bool {
	if !strings.HasPrefix(key, prefix) || len(key) == len(prefix) {
		return false
	}
	c := key[len(prefix)]
	return c >= 'A' && c <= 'Z'
}

// Elements returns the definition's snapshot, or its differential when no
// snapshot is present.
//
// Snapshot-less packages are not an edge case: German IGs frequently publish
// without one — every StructureDefinition in gematik's ISiK 4.0.3, for
// instance. The differential still carries every element the profile itself
// constrains, which is where mustSupport is declared; what it lacks is what the
// profile inherits, and that is what the base chain walk in Registry.Resolve
// recovers.
func (sd *StructureDefinition) Elements() []*Element {
	if len(sd.Snapshot.Element) > 0 {
		return sd.Snapshot.Element
	}
	return sd.Differential.Element
}

// HasSnapshot reports whether the definition shipped with a snapshot.
func (sd *StructureDefinition) HasSnapshot() bool {
	return len(sd.Snapshot.Element) > 0
}

type elemIndex map[string]*Element

// index returns the definition's elements keyed by element ID, built once.
func (sd *StructureDefinition) index() elemIndex {
	if sd.elementIndex == nil {
		sd.elementIndex = make(elemIndex)
		for _, e := range sd.Elements() {
			if e.ID != "" {
				sd.elementIndex[e.ID] = e
			}
		}
	}
	return sd.elementIndex
}

// Registry holds StructureDefinitions keyed by canonical URL.
type Registry struct {
	byURL map[string]*StructureDefinition
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{byURL: make(map[string]*StructureDefinition)}
}

// Get returns the definition with the given canonical URL, if loaded. A URL
// carrying a |version suffix is matched on the bare URL: fhirlint holds one
// version of a package at a time, so the suffix cannot disambiguate anything
// and dropping it resolves references that would otherwise dangle.
func (r *Registry) Get(url string) (*StructureDefinition, bool) {
	if sd, ok := r.byURL[url]; ok {
		return sd, true
	}
	if i := strings.Index(url, "|"); i >= 0 {
		sd, ok := r.byURL[url[:i]]
		return sd, ok
	}
	return nil, false
}

// ProfilesFrom returns the loaded definitions that constrain another one and
// came from one of the named packages, sorted by URL.
//
// The package filter is not a nicety. Supporting packages are loaded so that
// slice definitions can be resolved across them, and without this every profile
// in the cache — CDS Hooks, CQL Library, whatever else is installed — would be
// measured against a dataset that was never meant for it.
//
// Base resource definitions (derivation "specialization") are excluded either
// way: the mustSupport coverage of core Patient is not a question anyone has.
func (r *Registry) ProfilesFrom(packages ...string) []*StructureDefinition {
	want := make(map[string]bool, len(packages))
	for _, p := range packages {
		want[p] = true
	}

	var out []*StructureDefinition
	for _, sd := range r.byURL {
		if sd.Derivation != "constraint" {
			continue
		}
		if len(want) > 0 && !want[sd.Package] {
			continue
		}
		out = append(out, sd)
	}
	sortByURL(out)
	return out
}

// LoadPackage reads every StructureDefinition in a FHIR package directory —
// the "package" subdirectory of an entry in ~/.fhir/packages — into the
// registry. Files that are not StructureDefinitions, or not JSON at all, are
// skipped: a FHIR package holds examples and terminology alongside profiles.
//
// Definitions already present are not replaced, so packages named first win.
// That keeps the IGs the user asked about authoritative over whatever else the
// cache happens to hold.
func (r *Registry) LoadPackage(dir, label string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}

	n := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name())) //nolint:gosec // path from a cache directory the caller named
		if err != nil {
			continue
		}
		// Cheap rejection before the full parse: a package directory is mostly
		// examples and terminology, and unmarshalling all of it into the
		// element structs is the expensive part of loading.
		if !strings.Contains(string(data), `"StructureDefinition"`) {
			continue
		}
		var sd StructureDefinition
		if err := json.Unmarshal(data, &sd); err != nil {
			continue
		}
		if sd.URL == "" || sd.Type == "" {
			continue
		}
		if _, exists := r.byURL[sd.URL]; exists {
			continue
		}
		sd.Package = label
		r.byURL[sd.URL] = &sd
		n++
	}
	return n, nil
}

// PackageDir returns the directory holding a cached package's files.
func PackageDir(cacheRoot, name, version string) string {
	return filepath.Join(cacheRoot, name+"#"+version, "package")
}

// DefaultCacheRoot is where the FHIR package cache lives.
func DefaultCacheRoot() (string, error) { return fhirpkg.CacheRoot() }

// ErrPackageNotCached reports a package that is not in the local FHIR package
// cache and could not be fetched, because downloading was switched off.
type ErrPackageNotCached struct {
	ID  string
	Dir string
}

func (e *ErrPackageNotCached) Error() string {
	return fmt.Sprintf("IG package %s is not in the local FHIR package cache (%s) and --offline forbids downloading it — "+
		"drop --offline to fetch it from the package registry, or validate against it once to populate the cache", e.ID, e.Dir)
}
