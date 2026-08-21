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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fhirlint/fhirlint/internal/iglock"
)

// DefaultRegistry is the canonical FHIR package registry. It is the same host
// iglock records in each lock entry's URL.
const DefaultRegistry = "https://packages.fhir.org"

// defaultTimeout bounds a single packument request. The registry is a plain
// static-metadata host, so a slow response is a sign of trouble rather than of
// a large payload.
const defaultTimeout = 5 * time.Second

// maxConcurrent bounds how many packuments are fetched at once. Lock files hold
// a handful of packages, so this is about not hammering a community-run
// registry rather than about throughput.
const maxConcurrent = 4

// Client fetches packuments from a FHIR package registry.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// NewClient returns a Client pointed at the default registry.
func NewClient() *Client {
	return &Client{
		BaseURL: DefaultRegistry,
		HTTP:    &http.Client{Timeout: defaultTimeout},
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

	Deprecated      bool   `json:"deprecated,omitempty"`
	DeprecationNote string `json:"deprecationNote,omitempty"`

	// NotFound means the registry has no such package. That is a stronger
	// signal than being outdated: a pin that cannot be resolved will not
	// survive a cold cache.
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

	pkg, err := c.fetch(ctx, name)
	switch {
	case errors.Is(err, errNotFound):
		p.NotFound = true
		return p
	case err != nil:
		p.Error = err.Error()
		return p
	}

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
	if p.Latest == "" || p.Latest == version {
		return p
	}

	switch cmp, ok := CompareVersions(version, p.Latest); {
	case !ok:
		p.Differs = true
	case cmp < 0:
		p.Outdated = true
	case cmp > 0:
		p.Ahead = true
	}
	return p
}

// errNotFound distinguishes "the registry does not know this package" from
// "the registry could not be reached", which are different findings.
var errNotFound = errors.New("package not found in registry")

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

func (c *Client) fetch(ctx context.Context, name string) (packument, error) {
	var pkg packument

	// Package names are dotted identifiers, but they arrive from a file on disk
	// and are pasted straight into a URL path, so escape rather than trust.
	endpoint := strings.TrimSuffix(c.BaseURL, "/") + "/" + url.PathEscape(name)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return pkg, err
	}
	req.Header.Set("Accept", "application/json")

	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout}
	}

	resp, err := client.Do(req)
	if err != nil {
		return pkg, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return pkg, errNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return pkg, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&pkg); err != nil {
		return pkg, fmt.Errorf("parsing packument: %w", err)
	}
	return pkg, nil
}

// CompareVersions orders two package versions, returning -1, 0 or 1 along with
// whether the comparison could be made at all.
//
// FHIR IG versions are usually semver ("1.5.0", "2025.0.0", "1.00.000"), but
// the registry does not enforce it and publishers do deviate. Rather than
// guess, a version is only ordered when its core is at least two dot-separated
// integers, with an optional pre-release suffix. Everything else is reported as
// incomparable and the caller says "differs" instead of "outdated".
//
// The two-segment floor is what keeps a scheme change from being read as a
// version bump: "2025-Q1" would otherwise parse as core 2025 with pre-release
// Q1 and rank above "1.0.0", turning "this publisher started versioning by
// quarter" into a confident "you are 2024 releases behind". Single-integer
// schemes such as "20250101" are rejected for the same reason. Being told two
// versions differ is a weaker statement than being told which is newer, but it
// is never the wrong one.
//
// Pre-release handling follows semver only as far as it is unambiguous: a
// release outranks the same core version with a pre-release suffix. Two
// different pre-releases of the same core version are ordered lexically, which
// is an approximation and part of why this returns an ok flag at all.
func CompareVersions(a, b string) (int, bool) {
	aCore, aPre := splitPreRelease(a)
	bCore, bPre := splitPreRelease(b)

	aSeg, ok := numericSegments(aCore)
	if !ok {
		return 0, false
	}
	bSeg, ok := numericSegments(bCore)
	if !ok {
		return 0, false
	}

	for i := 0; i < len(aSeg) || i < len(bSeg); i++ {
		av, bv := 0, 0
		if i < len(aSeg) {
			av = aSeg[i]
		}
		if i < len(bSeg) {
			bv = bSeg[i]
		}
		if av != bv {
			return sign(av - bv), true
		}
	}

	switch {
	case aPre == "" && bPre == "":
		return 0, true
	case aPre == "":
		return 1, true // release beats pre-release
	case bPre == "":
		return -1, true
	case aPre == bPre:
		return 0, true
	case aPre < bPre:
		return -1, true
	default:
		return 1, true
	}
}

func splitPreRelease(v string) (core, pre string) {
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		return v[:i], v[i+1:]
	}
	return v, ""
}

// minVersionSegments is the two-segment floor described on CompareVersions.
const minVersionSegments = 2

func numericSegments(core string) ([]int, bool) {
	parts := strings.Split(core, ".")
	if len(parts) < minVersionSegments {
		return nil, false
	}
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

func sign(n int) int {
	if n < 0 {
		return -1
	}
	return 1
}
