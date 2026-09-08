// Package igbaseline reads the validation baseline an IG package ships about
// its own contents.
//
// Many IGs deliberately publish invalid examples to demonstrate what their
// invariants catch. de.fhir.medication#2.0.0-ballot ships 563 examples and 327
// of them fail — which looks like a broken package and is not. Nothing in the
// resources says so: the ImplementationGuide lists every example with an empty
// description, and exampleBoolean only states that something is an example,
// never that it is meant to fail. Filename conventions do not carry it either;
// in that package 368 examples are named INV-…/Invalid-…/Warning-… and only 327
// fail, because the 41 Warning-… cases legitimately produce warnings only.
//
// The publisher's own QA run does carry it. Recent IG Publisher versions write
// package/other/validation-summary.json into the tarball, keyed by
// "ResourceType/id", and that file states exactly which of a package's own
// resources it found errors in. Comparing against it turns an uninterpretable
// count into the number that matters: how much a local run disagrees with the
// build the publisher shipped (#402).
package igbaseline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fhirlint/fhirlint/internal/fhirpkg"
	"github.com/fhirlint/fhirlint/internal/validator"
)

// SummaryFile is where the IG Publisher writes the baseline inside a package.
const SummaryFile = "other/validation-summary.json"

// Counts is what the publisher recorded for one resource.
type Counts struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
}

// Baseline is one package's recorded QA result, keyed by "ResourceType/id".
type Baseline struct {
	// PackageID is the "name#version" directory the baseline was read from,
	// carried so a report can name the authority it is citing.
	PackageID string

	entries map[string]Counts
}

// Load reads the baseline out of an installed package directory — the
// "<name>#<version>" directory in the FHIR package cache, not the "package"
// subdirectory inside it.
//
// A package without the file returns (nil, nil). That is the common case, not
// an error: only 113 of the 198 packages in a representative local cache carry
// one, essentially those built by newer IG Publisher versions. de.basisprofil.r4
// and hl7.fhir.uv.ips do not, at any version. A caller gets no baseline and
// reports exactly as it did before.
func Load(pkgDir string) (*Baseline, error) {
	path := filepath.Join(pkgDir, "package", filepath.FromSlash(SummaryFile))
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is derived from the package cache layout
	if os.IsNotExist(err) {
		return nil, nil //nolint:nilnil // "no baseline" is the common case and not a failure
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var entries map[string]Counts
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &Baseline{PackageID: filepath.Base(pkgDir), entries: entries}, nil
}

// Len reports how many resources the baseline covers.
func (b *Baseline) Len() int {
	if b == nil {
		return 0
	}
	return len(b.entries)
}

// Lookup returns what the publisher recorded for a resource key, and whether
// the baseline mentions it at all.
//
// The two are different answers. A resource the baseline does not mention was
// not part of the published build — a file the user added, or one from another
// package — and nothing can be concluded about it. A resource recorded with
// zero errors is a positive statement that the build found none.
func (b *Baseline) Lookup(key string) (Counts, bool) {
	if b == nil {
		return Counts{}, false
	}
	c, ok := b.entries[key]
	return c, ok
}

// ResourceKey reads the "ResourceType/id" key for a resource file.
//
// Taken from the content rather than the filename. The cache happens to name
// examples "MedicationDispense-Example-MD-Cov-bedarf-doseRange.json", which
// looks parseable until an id containing a hyphen makes the split ambiguous —
// and that is most of them.
func ResourceKey(path string) (string, bool) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: the path is one the caller was already validating
	if err != nil {
		return "", false
	}
	var head struct {
		ResourceType string `json:"resourceType"`
		ID           string `json:"id"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return "", false
	}
	if head.ResourceType == "" || head.ID == "" {
		return "", false
	}
	return head.ResourceType + "/" + head.ID, true
}

// PackageDirFor reports which installed package a file belongs to, by locating
// it inside the FHIR package cache rooted at root.
//
// Path-based on purpose. A resource carries no record of the package that
// shipped it, and matching on id alone would let one package's baseline answer
// for another package's resource of the same name. Being inside
// "<cache>/<name>#<version>/package/" is the one unambiguous statement that a
// file came from that package.
//
// Returns "" for anything outside the cache, which is every file a user
// validates from their own project. root is taken as an argument rather than
// resolved here so that a run resolves it once, and so that a test can point it
// at a temp directory without reaching into fhirpkg.
func PackageDirFor(root, path string) string {
	if root == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return ""
	}

	parts := strings.Split(filepath.ToSlash(rel), "/")
	// <name>#<version>/package/…: the package subdirectory has to be there, or
	// this is some other file that merely lives under the cache root.
	if len(parts) < 3 || !strings.Contains(parts[0], "#") || parts[1] != "package" {
		return ""
	}
	return filepath.Join(root, parts[0])
}

// Status is what a package's own baseline says about one validated resource.
type Status int

const (
	// StatusUnknown means no baseline covers this file: it is not from an
	// installed package, or the package ships no validation-summary.json, or
	// the build did not include this resource. Nothing follows from it.
	StatusUnknown Status = iota

	// StatusExpected: the publisher's build found errors here too. The finding
	// is a demonstration, not a defect.
	StatusExpected

	// StatusNew: the publisher's build found none and this run does. Either the
	// local environment differs from the publisher's, or something really is
	// wrong. This is the number worth acting on.
	StatusNew

	// StatusMissing: the publisher's build found errors and this run does not.
	// Not good news — it usually means the local run checked less, not that
	// anything was fixed, since the package is a fixed artefact.
	StatusMissing

	// StatusAgreesClean: both found nothing.
	StatusAgreesClean
)

// Report is the outcome of comparing a run against the baselines of whichever
// packages the validated files came from.
type Report struct {
	// Status is keyed by validator.Result.Filename.
	Status map[string]Status

	Covered  int
	Expected int
	New      int
	Missing  int

	// Packages lists the package ids whose baselines were consulted, sorted.
	Packages []string
}

// Compare classifies each result against the baseline of the package it came
// from, loading each package's baseline at most once.
//
// Files outside the package cache, and packages without a baseline, come back
// as StatusUnknown and are counted in nothing. A run over a user's own project
// therefore produces an empty report, which is what lets the caller stay silent
// rather than explaining an absence.
func Compare(results []*validator.Result) *Report {
	root, err := fhirpkg.CacheRoot()
	if err != nil {
		// No cache root means no package can be identified, which is the same
		// answer as a run over files that are not in one.
		return &Report{Status: map[string]Status{}}
	}
	return CompareIn(root, results)
}

// CompareIn is Compare against an explicit package cache root.
func CompareIn(root string, results []*validator.Result) *Report {
	rep := &Report{Status: make(map[string]Status, len(results))}
	loaded := map[string]*Baseline{}
	seen := map[string]bool{}

	for _, r := range results {
		if r == nil {
			continue
		}
		path := r.SourcePath
		if path == "" {
			path = r.Filename
		}

		dir := PackageDirFor(root, path)
		if dir == "" {
			continue
		}
		b, ok := loaded[dir]
		if !ok {
			// A baseline that cannot be read is treated as absent. It is an
			// optional cross-check on someone else's artefact, and failing a
			// user's validation over a malformed file in a third-party package
			// would be a poor trade.
			b, _ = Load(dir)
			loaded[dir] = b
		}
		if b == nil {
			continue
		}

		key, ok := ResourceKey(path)
		if !ok {
			continue
		}
		counts, known := b.Lookup(key)
		if !known {
			continue
		}

		if !seen[b.PackageID] {
			seen[b.PackageID] = true
			rep.Packages = append(rep.Packages, b.PackageID)
		}
		rep.Covered++

		switch published, local := counts.Errors > 0, hasErrors(r); {
		case published && local:
			rep.Status[r.Filename] = StatusExpected
			rep.Expected++
		case !published && local:
			rep.Status[r.Filename] = StatusNew
			rep.New++
		case published && !local:
			rep.Status[r.Filename] = StatusMissing
			rep.Missing++
		default:
			rep.Status[r.Filename] = StatusAgreesClean
		}
	}

	sort.Strings(rep.Packages)
	return rep
}

func hasErrors(r *validator.Result) bool {
	for _, i := range r.Issues {
		if i.Severity == "error" || i.Severity == "fatal" {
			return true
		}
	}
	return false
}
