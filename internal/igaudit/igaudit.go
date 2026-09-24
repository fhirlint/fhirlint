// Package igaudit checks the IG packages recorded in fhirlint.lock against the
// FHIR package registry.
//
// Pinning IG packages makes a run reproducible, but it says nothing about
// whether the pins are still the right ones: a project can sit on an old
// package version indefinitely without anything pointing it out. This package
// answers that question from the lock file alone, without touching the
// validator or the package cache.
package igaudit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fhirlint/fhirlint/internal/fhirpkg"
	"github.com/fhirlint/fhirlint/internal/iglock"
	"github.com/fhirlint/fhirlint/internal/registry"
)

// defaultTimeout bounds a single packument request. The registry is a plain
// static-metadata host, so a slow response is a sign of trouble rather than of
// a large payload.
const defaultTimeout = 5 * time.Second

// maxConcurrent bounds how many packuments are fetched at once. Lock files hold
// a handful of packages, so this is about not hammering a community-run
// registry rather than about throughput.
const maxConcurrent = 4

// Client fetches packuments from the FHIR package registries.
type Client struct {
	// Registries are asked in order, and a package is only not found when every
	// one of them says so. The default is the validator's own order — see
	// package registry for why there are two and why the order matters.
	Registries []string
	HTTP       *http.Client
}

// NewClient returns a Client pointed at the registries the validator uses.
func NewClient() *Client {
	return &Client{
		Registries: registry.Default(),
		HTTP:       &http.Client{Timeout: defaultTimeout},
	}
}

// PackageReport is the outcome for one package in the lock file.
type PackageReport struct {
	// ID is the lock file key, i.e. "name#version".
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`

	// Latest is the registry's dist-tags.latest, empty when it could not be read.
	Latest string `json:"latest,omitempty"`

	// Outdated is set only when the pinned version was established to be older
	// than Latest. A version that merely differs from Latest without being
	// comparable is reported through Differs instead, so "outdated" never
	// overstates what was actually determined.
	Outdated bool `json:"outdated,omitempty"`

	// Differs marks a pinned version that is not Latest but could not be ordered
	// against it — FHIR IG versioning is not reliably semver.
	Differs bool `json:"differs,omitempty"`

	// Ahead marks a pinned version newer than the registry's latest, which
	// happens with pre-release pins. Not a problem, but worth showing.
	Ahead bool `json:"ahead,omitempty"`

	// LatestIsPreRelease means dist-tags.latest names a pre-release — a ballot,
	// a release candidate — while the pin is a final. Upstream has not blessed a
	// newer release, so the pin is not outdated: there is nothing to move to.
	//
	// Kept distinct from Ahead, which is the mirror image (a pre-release pin
	// against a final tag). Both say "the pin is not the tag, and that is fine",
	// but for opposite reasons, and a reader needs to know which.
	LatestIsPreRelease bool `json:"latestIsPreRelease,omitempty"`

	// UntaggedNewer lists final versions the registry serves that outrank both
	// the pin and dist-tags.latest — published without being blessed.
	//
	// Not a problem: pinning the tag rather than the highest number is the right
	// rule, and this is what upstream declined to make current. It is here
	// because without it a pin can read as "current" for a year while newer
	// releases sit on the registry, and finding that out took a hand-read of the
	// packument (#406).
	UntaggedNewer []string `json:"untaggedNewer,omitempty"`

	Deprecated      bool   `json:"deprecated,omitempty"`
	DeprecationNote string `json:"deprecationNote,omitempty"`

	// Registry is the registry that answered for this package, as a base URL.
	// The two default registries serve the same metadata for everything they
	// both carry, so this is recorded for the day they do not, and for a
	// package only one of them has.
	Registry string `json:"registry,omitempty"`

	// NotFound means no registry has such a package. That is a stronger
	// signal than being outdated: a pin that cannot be resolved will not
	// survive a cold cache. It takes a 404 from every registry — a registry
	// that could not be reached is reported through Error instead.
	NotFound bool `json:"notFound,omitempty"`

	// VersionMissing means the package exists but the registry does not serve
	// the pinned version. It fails exactly like NotFound — nothing resolves on
	// a cold cache — but points somewhere else: the name is right and the
	// version is wrong, which is what a hand-written pin usually gets wrong.
	VersionMissing bool `json:"versionMissing,omitempty"`

	// Error records why this package could not be checked. It is distinct from
	// NotFound: an unreachable registry is not evidence about the package.
	Error string `json:"error,omitempty"`
}

// IsProblem reports whether this package needs the reader's attention.
// A version that is merely ahead of the registry's latest does not.
func (p PackageReport) IsProblem() bool {
	return p.Outdated || p.Differs || p.Deprecated || p.NotFound || p.VersionMissing
}

// Report is the result of auditing every package in a lock file.
type Report struct {
	Packages []PackageReport
}

// Problems counts the packages needing attention. Packages that could not be
// checked at all are not counted: an unreachable registry is reported as an
// error rather than silently turning into a finding.
func (r Report) Problems() int {
	n := 0
	for _, p := range r.Packages {
		if p.IsProblem() {
			n++
		}
	}
	return n
}

// Errors counts the packages that could not be checked.
func (r Report) Errors() int {
	n := 0
	for _, p := range r.Packages {
		if p.Error != "" {
			n++
		}
	}
	return n
}

// Audit checks every package ID against the registry. IDs are the lock file
// keys ("name#version"). The returned reports are sorted by ID so that output
// is stable across runs — lock file packages live in a map.
func Audit(ctx context.Context, c *Client, ids []string) Report {
	sorted := make([]string, len(ids))
	copy(sorted, ids)
	sort.Strings(sorted)

	reports := make([]PackageReport, len(sorted))
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	for i, id := range sorted {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			reports[i] = checkOne(ctx, c, id)
		}()
	}
	wg.Wait()

	return Report{Packages: reports}
}

func checkOne(ctx context.Context, c *Client, id string) PackageReport {
	name, version := iglock.ParseIGID(id)
	if name == "" {
		// Lock files are written with "name#version" keys, so this only happens
		// with a hand-edited file. Say so instead of silently skipping it.
		return PackageReport{ID: id, Error: "not a name#version package id"}
	}

	p := PackageReport{ID: id, Name: name, Version: version}

	pkg, from, err := c.fetch(ctx, name)
	switch {
	case errors.Is(err, registry.ErrNotFound):
		p.NotFound = true
		return p
	case err != nil:
		p.Error = err.Error()
		return p
	}
	p.Registry = from

	// A packument with no versions map at all is a registry quirk rather than
	// evidence about the pin, so only a populated list can contradict it.
	if len(pkg.Versions) > 0 {
		if _, ok := pkg.Versions[version]; !ok {
			p.VersionMissing = true
		}
	}

	if note, ok := pkg.deprecation(version); ok {
		p.Deprecated = true
		p.DeprecationNote = note
	}

	p.Latest = pkg.DistTags.Latest
	if p.Latest == "" {
		return p
	}
	// A pre-release tag over a final pin is not a version to move to: upstream is
	// mid-ballot and has blessed nothing newer, so this is reported rather than
	// counted as a finding.
	//
	// Guarded by the comparison rather than checked on its own, because
	// IsPreRelease is deliberately syntactic: "2025-Q1" is a whole calendar
	// versioning scheme, not a pre-release, and it must keep reaching Differs.
	cmp, ordered := CompareVersions(version, p.Latest)
	p.LatestIsPreRelease = ordered && cmp < 0 &&
		fhirpkg.IsPreRelease(p.Latest) && !fhirpkg.IsPreRelease(version)

	p.UntaggedNewer = pkg.untaggedNewer(version, p.Latest, p.LatestIsPreRelease)
	if p.Latest == version {
		return p
	}

	switch {
	case p.LatestIsPreRelease:
		// Already classified above, and deliberately none of the three below.
	case !ordered:
		p.Differs = true
	case cmp < 0:
		p.Outdated = true
	case cmp > 0:
		p.Ahead = true
	}
	return p
}

// packument is the npm-style metadata document the FHIR registry serves for a
// package. Only the fields fhirlint needs are modelled.
type packument struct {
	DistTags struct {
		Latest string `json:"latest"`
	} `json:"dist-tags"`
	Versions map[string]struct {
		Deprecated json.RawMessage `json:"deprecated"`
	} `json:"versions"`
}

// untaggedNewer returns the final versions this packument serves that outrank
// both the pinned version and the latest tag, oldest first.
//
// Both bounds are needed. Against latest alone, a pin that is merely outdated
// would list the versions the Outdated finding is already about; against the
// pin alone, latest itself would show up. What is left is exactly the set that
// no existing field describes: released, downloadable, and not what upstream
// says is current.
//
// The latest bound is dropped when the caller established that latest is a
// pre-release over a final pin. The bound is there to avoid restating what
// Outdated already says, and in that case there is no Outdated finding to
// restate — LatestIsPreRelease is reported instead. Keeping it would hide every
// final between the pin and the ballot, which is the interesting part: MII's
// icu module serves 2026.0.3 and 2027.0.0 under a 2027.0.0-ballot.3 tag, and
// only the second outranks it (#434).
//
// Pre-releases are excluded, and so is anything CompareVersions cannot order —
// consistent with Differs, which reports rather than guesses.
func (p packument) untaggedNewer(version, latest string, latestIsPreRelease bool) []string {
	ceiling := latest
	if latestIsPreRelease {
		ceiling = ""
	}
	var newer []string
	for v := range p.Versions {
		if fhirpkg.IsPreRelease(v) {
			continue
		}
		if ceiling != "" {
			if cmp, ok := fhirpkg.CompareVersions(v, ceiling); !ok || cmp <= 0 {
				continue
			}
		}
		if cmp, ok := fhirpkg.CompareVersions(v, version); !ok || cmp <= 0 {
			continue
		}
		newer = append(newer, v)
	}
	sort.Slice(newer, func(i, j int) bool {
		if cmp, ok := fhirpkg.CompareVersions(newer[i], newer[j]); ok {
			return cmp < 0
		}
		return newer[i] < newer[j]
	})
	return newer
}

// deprecation reports whether the given version carries a deprecation marker.
//
// Best-effort by design: the field is part of the npm packument format that the
// FHIR registry follows, but the registry does not populate it today. Reading
// it costs nothing and means the check starts working the day it does, while
// its absence is correctly reported as "not deprecated".
func (p packument) deprecation(version string) (string, bool) {
	v, ok := p.Versions[version]
	if !ok {
		return "", false
	}
	return deprecationNote(v.Deprecated)
}

// deprecationNote interprets npm's `deprecated` field, which is either a
// boolean or a free-text reason. An empty string is npm's way of undoing a
// deprecation, so it counts as not deprecated.
func deprecationNote(raw json.RawMessage) (string, bool) {
	s := strings.TrimSpace(string(raw))
	switch s {
	case "", "null", "false":
		return "", false
	case "true":
		return "", true
	}
	var note string
	if err := json.Unmarshal(raw, &note); err != nil || note == "" {
		return "", false
	}
	return note, true
}

// fetch returns the packument for name and the registry it came from. The
// error is registry.ErrNotFound only when every registry answered 404.
func (c *Client) fetch(ctx context.Context, name string) (packument, string, error) {
	var pkg packument

	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout}
	}

	// Package names are dotted identifiers, but they arrive from a file on disk
	// and are pasted straight into a URL path, so escape rather than trust.
	resp, from, err := registry.Get(ctx, client, c.Registries, url.PathEscape(name), "application/json")
	if err != nil {
		return pkg, "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if err := json.NewDecoder(resp.Body).Decode(&pkg); err != nil {
		return pkg, "", fmt.Errorf("parsing packument from %s: %w", registry.Host(from), err)
	}
	return pkg, from, nil
}

// CompareVersions orders two package versions, returning -1, 0 or 1 along with
// whether the comparison could be made at all.
//
// The implementation lives in internal/fhirpkg, which sits below both this
// package and the package cache that also has to order versions. It was here
// first, and fhirpkg resolving ranges by string order was the result of the two
// never meeting (#390). This wrapper stays because callers and the
// registry-tagged alias tests use the name.
func CompareVersions(a, b string) (int, bool) { return fhirpkg.CompareVersions(a, b) }
