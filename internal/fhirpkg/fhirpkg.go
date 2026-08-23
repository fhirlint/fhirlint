// Package fhirpkg reads the FHIR package cache — the ~/.fhir/packages directory
// the validator JAR populates and the rest of the FHIR toolchain shares.
//
// fhirlint only ever reads it. The directory belongs to no single tool: the IG
// publisher, sushi and the validator all write there, so anything that removes
// or renames entries risks pulling the ground out from under a concurrent run.
package fhirpkg

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CacheRoot returns the FHIR package cache directory.
//
// Deliberately not configurable. This is not fhirlint's cache — the validator
// JAR resolves it from its own user.home and reads no environment variable for
// it, so an override only fhirlint honoured would point the two at different
// directories. That divergence is exactly what #351 was about, and it is worse
// than the inconvenience it would solve. FHIRLINT_CACHE_DIR moves fhirlint's
// own cache and does not move this one.
func CacheRoot() (string, error) {
	home, err := homeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".fhir", "packages"), nil
}

// homeDir is a var so tests can point the cache at a temp directory without
// introducing a user-facing override that the JAR would not follow.
var homeDir = os.UserHomeDir

// Manifest is the part of a package's package.json fhirlint cares about.
type Manifest struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Title        string            `json:"title"`
	Description  string            `json:"description"`
	Canonical    string            `json:"canonical"`
	FHIRVersions []string          `json:"fhirVersions"`
	Dependencies map[string]string `json:"dependencies"`
}

// Package is one installed package: its manifest plus what the cache knows
// about it that the manifest does not.
type Package struct {
	Manifest
	// ID is the cache directory name, "name#version". It is the authority on
	// the installed version — a handful of published packages carry a manifest
	// whose version disagrees with the directory they install into.
	ID string
	// Dir is the package directory inside the cache.
	Dir string
	// Bytes is the size on disk, 0 when it could not be measured.
	Bytes int64
}

// ManifestPath returns the path to a cached package's package.json.
func ManifestPath(name, version string) (string, error) {
	root, err := CacheRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, name+"#"+version, "package", "package.json"), nil
}

// SplitID splits "name#version". The version may be empty, which is how an
// unpinned IG reference reaches here.
func SplitID(id string) (name, version string) {
	idx := strings.LastIndex(id, "#")
	if idx < 0 {
		return id, ""
	}
	return id[:idx], id[idx+1:]
}

// ErrNoCache reports that the package cache directory does not exist. Nothing
// has been validated on this machine yet, which is a state to explain rather
// than an error to fail on.
var ErrNoCache = errors.New("no FHIR package cache")

// List returns every package in the cache, sorted by name then version.
//
// A directory that cannot be read or whose manifest will not parse is skipped
// rather than failing the listing: the cache is written by other tools, and one
// bad entry must not hide the other sixty.
func List(withSizes bool) ([]Package, error) {
	root, err := CacheRoot()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w at %s", ErrNoCache, root)
		}
		return nil, err
	}

	var pkgs []Package
	for _, e := range entries {
		if !e.IsDir() || !strings.Contains(e.Name(), "#") {
			continue
		}
		dir := filepath.Join(root, e.Name())
		m, err := readManifest(filepath.Join(dir, "package", "package.json"))
		if err != nil {
			continue
		}
		name, version := SplitID(e.Name())
		// The directory name wins over the manifest: it is what the validator
		// resolves against.
		m.Name, m.Version = name, version

		p := Package{Manifest: m, ID: e.Name(), Dir: dir}
		if withSizes {
			p.Bytes = dirSize(dir)
		}
		pkgs = append(pkgs, p)
	}

	sort.Slice(pkgs, func(i, j int) bool {
		if a, b := foldName(pkgs[i].Name), foldName(pkgs[j].Name); a != b {
			return a < b
		}
		return pkgs[i].Version < pkgs[j].Version
	})
	return pkgs, nil
}

// Load returns one installed package, or false when it is not in the cache.
func Load(name, version string) (Package, bool) {
	root, err := CacheRoot()
	if err != nil {
		return Package{}, false
	}
	id := name + "#" + version
	dir := filepath.Join(root, id)
	m, err := readManifest(filepath.Join(dir, "package", "package.json"))
	if err != nil {
		return Package{}, false
	}
	m.Name, m.Version = name, version
	return Package{Manifest: m, ID: id, Dir: dir}, true
}

// foldName is the key packages are grouped under. Older KBV releases published
// under capitalised names (KBV.Basis) and later ones lowercase (kbv.basis);
// they are the same package upstream and belong together in a listing. The
// directory itself is never renamed — only the grouping folds.
func foldName(name string) string { return strings.ToLower(name) }

// Group is one logical package and every version of it in the cache.
type Group struct {
	Name string // display name, from the newest entry
	// MixedCase is true when the cache holds this package under more than one
	// spelling, which means two copies of the same thing on disk.
	MixedCase bool
	Versions  []Package
	Bytes     int64
}

// Grouped folds a listing into one entry per logical package.
func Grouped(pkgs []Package) []Group {
	byKey := map[string]*Group{}
	var order []string
	for _, p := range pkgs {
		k := foldName(p.Name)
		g, ok := byKey[k]
		if !ok {
			g = &Group{Name: p.Name}
			byKey[k] = g
			order = append(order, k)
		}
		if g.Name != p.Name {
			g.MixedCase = true
			// Versions arrive in ascending order, so the last spelling seen is
			// the newest one — which is what upstream publishes under today and
			// what a user would type.
			g.Name = p.Name
		}
		g.Versions = append(g.Versions, p)
		g.Bytes += p.Bytes
	}
	out := make([]Group, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	return out
}

func readManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path) //nolint:gosec // a path inside the package cache
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// dirSize sums the regular files under dir. Errors yield 0 rather than failing
// the listing — a size is a nicety, the entry is not.
func dirSize(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // an unreadable subtree just does not count
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}
