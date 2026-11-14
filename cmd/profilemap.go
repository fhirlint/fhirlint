// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/fhirlint/fhirlint/internal/rules/fhirpath"
	"github.com/fhirlint/fhirlint/internal/validator"
)

// profileMapEntry is one profile of a profile-map key, applied always or only
// when its FHIRPath condition holds for the resource (#524).
type profileMapEntry struct {
	Profile string
	// When is the compiled condition; nil for an unconditional entry.
	When *fhirpath.Program
}

// profileMapping is the parsed profile-map: resource type or filename glob to
// the entries for it.
type profileMapping map[string][]profileMapEntry

// parseProfileMap reads the profile-map value from the config. Each key takes
// a profile URL, a {profile, when} map, or a list mixing both. A condition
// that does not compile is a config error: a silently ignored condition would
// apply a profile to everything, or to nothing.
func parseProfileMap(raw any) (profileMapping, error) {
	if raw == nil {
		return nil, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("profile-map must be a map of resource type or filename glob to profiles")
	}
	out := make(profileMapping, len(m))
	for key, v := range m {
		items, isList := v.([]any)
		if !isList {
			items = []any{v}
		}
		for i, item := range items {
			e, err := parseProfileMapEntry(item)
			if err != nil {
				return nil, fmt.Errorf("profile-map %q, entry %d: %w", key, i+1, err)
			}
			out[key] = append(out[key], e)
		}
	}
	return out, nil
}

func parseProfileMapEntry(item any) (profileMapEntry, error) {
	switch tv := item.(type) {
	case string:
		return profileMapEntry{Profile: tv}, nil
	case map[string]any:
		profile, _ := tv["profile"].(string)
		if profile == "" {
			return profileMapEntry{}, fmt.Errorf("an entry with keys needs a \"profile\"")
		}
		for k := range tv {
			if k != "profile" && k != "when" {
				return profileMapEntry{}, fmt.Errorf("unknown key %q (allowed: profile, when)", k)
			}
		}
		e := profileMapEntry{Profile: profile}
		if w, ok := tv["when"]; ok {
			expr, isString := w.(string)
			if !isString || expr == "" {
				return profileMapEntry{}, fmt.Errorf("\"when\" must be a FHIRPath expression")
			}
			prog, err := fhirpath.Compile(expr)
			if err != nil {
				return profileMapEntry{}, fmt.Errorf("\"when\" %q: %w (supported FHIRPath: see docs/rules.md)", expr, err)
			}
			e.When = prog
		}
		return e, nil
	default:
		return profileMapEntry{}, fmt.Errorf("must be a profile URL or a {profile, when} map")
	}
}

// AppliedProfile records why profile-map added a profile to a file, for the
// JSON report.
type appliedProfile = validator.AppliedProfile

// profileMapRecorder remembers, per validated path, which profile-map entries
// applied, so that the results can say so after the run. Files are validated
// in groups across several code paths; recording at the one place that
// resolves the map is simpler than threading reasons through all of them.
type profileMapRecorder struct {
	mu      sync.Mutex
	applied map[string][]appliedProfile
	warned  map[string]bool
}

var profileMapApplied = &profileMapRecorder{}

func (r *profileMapRecorder) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.applied = map[string][]appliedProfile{}
	r.warned = map[string]bool{}
}

func (r *profileMapRecorder) record(path string, a []appliedProfile) {
	if len(a) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.applied == nil {
		r.applied = map[string][]appliedProfile{}
	}
	r.applied[filepath.Clean(path)] = a
}

// warnOnce prints a warning once per run, keyed by what it is about.
func (r *profileMapRecorder) warnOnce(key, format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.warned == nil {
		r.warned = map[string]bool{}
	}
	if r.warned[key] {
		return
	}
	r.warned[key] = true
	fmt.Fprintf(os.Stderr, format, args...)
}

// attach sets AppliedProfiles on each result, by the path the validator read
// and, for preprocessed input, by the user's file.
func (r *profileMapRecorder) attach(results []*validator.Result) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, res := range results {
		if res == nil {
			continue
		}
		for _, p := range []string{res.SourcePath, res.Filename} {
			if p == "" {
				continue
			}
			if a, ok := r.applied[filepath.Clean(p)]; ok {
				res.AppliedProfiles = a
				break
			}
		}
	}
}

// resolveProfileEntries returns the profiles profile-map adds to filePath and
// why: resource type keys first, then filename globs in key order. Every
// matching entry whose condition holds applies. Conditions need the resource
// as JSON; for anything else the conditional entries are skipped with a
// warning, and the unconditional ones still apply.
func resolveProfileEntries(filePath string, pm profileMapping) []appliedProfile {
	if len(pm) == 0 {
		return nil
	}
	relSlash := filepath.ToSlash(filePath)
	type match struct {
		key   string
		entry profileMapEntry
	}
	var matches []match
	if rt := peekResourceType(filePath); rt != "" {
		for _, e := range pm[rt] {
			matches = append(matches, match{rt, e})
		}
	}
	globs := make([]string, 0, len(pm))
	for k := range pm {
		if isFilenameGlob(k) {
			globs = append(globs, k)
		}
	}
	sort.Strings(globs)
	for _, k := range globs {
		if matchesExclude(relSlash, k) {
			for _, e := range pm[k] {
				matches = append(matches, match{k, e})
			}
		}
	}

	var data []byte
	var readErr error
	read := false
	var out []appliedProfile
	for _, m := range matches {
		if m.entry.When == nil {
			out = append(out, appliedProfile{Profile: m.entry.Profile, Reason: "profile-map: " + m.key})
			continue
		}
		if !read {
			read = true
			data, readErr = os.ReadFile(filePath) //nolint:gosec // a file fhirlint is validating
		}
		if readErr != nil || !looksLikeJSON(data) {
			profileMapApplied.warnOnce("xml:"+filePath,
				"warn: profile-map: %s is not JSON, so the conditional entries for %q do not apply to it; FHIRPath conditions need JSON input\n",
				filePath, m.key)
			continue
		}
		ok, err := m.entry.When.EvalBool(data)
		if err != nil {
			profileMapApplied.warnOnce("eval:"+filePath+"\x00"+m.entry.When.String(),
				"warn: profile-map %q: condition %q failed on %s (%v); the profile is not applied\n",
				m.key, m.entry.When.String(), filePath, err)
			continue
		}
		if ok {
			out = append(out, appliedProfile{Profile: m.entry.Profile, Reason: "profile-map: " + m.key + " when " + m.entry.When.String()})
		}
	}
	profileMapApplied.record(filePath, out)
	return out
}

func looksLikeJSON(data []byte) bool {
	t := bytes.TrimLeft(data, " \t\r\n\ufeff")
	return len(t) > 0 && t[0] == '{'
}
