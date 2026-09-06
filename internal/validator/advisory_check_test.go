package validator

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fhirlint/fhirlint/internal/cache"
)

// installed points the cache at a temp dir holding one validator version.
func installed(t *testing.T, version string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(cache.DirEnvVar, dir)
	if version != "" {
		if err := os.WriteFile(filepath.Join(dir, "validator_version.txt"), []byte(version), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// stubAdvisories replaces the lookup and counts how often it ran, so "the cache
// was used" can be asserted rather than assumed.
func stubAdvisories(t *testing.T, list []Advisory, err error) *int {
	t.Helper()
	calls := 0
	orig := advisoryFetcher
	advisoryFetcher = func() ([]Advisory, error) {
		calls++
		return list, err
	}
	t.Cleanup(func() { advisoryFetcher = orig })
	return &calls
}

func advisory(id, severity, rng string) Advisory {
	return Advisory{
		GHSAID:          id,
		Severity:        severity,
		Vulnerabilities: []Vulnerability{{VulnerableVersionRange: rng}},
	}
}

func TestAffectingAdvisories_CountsOnlyWhatAffectsTheInstalledVersion(t *testing.T) {
	installed(t, "6.9.11")
	stubAdvisories(t, []Advisory{
		advisory("GHSA-a", "high", "<= 6.9.11"),
		advisory("GHSA-b", "medium", "<= 6.9.11"),
		advisory("GHSA-c", "critical", "<= 6.5.0"), // older, does not apply
	}, nil)

	got, ok := AffectingAdvisories()
	if !ok {
		t.Fatal("want a summary")
	}
	if got.Count != 2 {
		t.Errorf("Count = %d, want 2 (the third advisory predates this version)", got.Count)
	}
	if got.Highest != "high" {
		t.Errorf("Highest = %q, want high — critical applies to a version we do not run", got.Highest)
	}
	if got.Version != "6.9.11" {
		t.Errorf("Version = %q, want the installed one", got.Version)
	}
}

func TestAffectingAdvisories_CachesForADay(t *testing.T) {
	installed(t, "6.9.11")
	calls := stubAdvisories(t, []Advisory{advisory("GHSA-a", "high", "<= 6.9.11")}, nil)

	for i := 0; i < 3; i++ {
		if _, ok := AffectingAdvisories(); !ok {
			t.Fatalf("call %d: want a summary", i)
		}
	}
	if *calls != 1 {
		t.Errorf("the advisory list was fetched %d times; the cache should make it 1", *calls)
	}
}

// Pinning a different release must re-check. A count computed for 6.9.11 says
// nothing about 6.10.4, and reusing it would be worse than not caching at all.
func TestAffectingAdvisories_VersionChangeInvalidatesTheCache(t *testing.T) {
	dir := installed(t, "6.9.11")
	calls := stubAdvisories(t, []Advisory{advisory("GHSA-a", "high", "<= 6.9.11")}, nil)

	first, _ := AffectingAdvisories()
	if first.Count != 1 {
		t.Fatalf("Count = %d, want 1", first.Count)
	}
	if err := os.WriteFile(filepath.Join(dir, "validator_version.txt"), []byte("6.10.4"), 0600); err != nil {
		t.Fatal(err)
	}
	second, ok := AffectingAdvisories()
	if !ok {
		t.Fatal("want a summary for the new version")
	}
	if second.Count != 0 {
		t.Errorf("Count = %d, want 0 — the advisory does not affect 6.10.4", second.Count)
	}
	if *calls != 2 {
		t.Errorf("fetched %d times, want 2 (a version change must re-check)", *calls)
	}
}

func TestAffectingAdvisories_StaleCacheRefetches(t *testing.T) {
	dir := installed(t, "6.9.11")
	stale := AdvisorySummary{
		Version:     "6.9.11",
		Count:       99,
		LastChecked: time.Now().Add(-2 * advisoryCheckInterval),
	}
	data, _ := json.Marshal(stale)
	if err := os.WriteFile(filepath.Join(dir, "advisory_check.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	stubAdvisories(t, []Advisory{advisory("GHSA-a", "high", "<= 6.9.11")}, nil)

	got, _ := AffectingAdvisories()
	if got.Count != 1 {
		t.Errorf("Count = %d, want the refetched 1, not the stale 99", got.Count)
	}
}

// A courtesy check must never turn into an error the user has to deal with.
func TestAffectingAdvisories_SilentOnFailure(t *testing.T) {
	installed(t, "6.9.11")
	stubAdvisories(t, nil, errors.New("network is down"))

	if _, ok := AffectingAdvisories(); ok {
		t.Error("a failed lookup must report nothing rather than an empty summary")
	}
}

func TestAffectingAdvisories_NoJARInstalled(t *testing.T) {
	installed(t, "")
	calls := stubAdvisories(t, []Advisory{advisory("GHSA-a", "high", "<= 6.9.11")}, nil)

	if _, ok := AffectingAdvisories(); ok {
		t.Error("with no validator installed there is nothing to report on")
	}
	if *calls != 0 {
		t.Error("must not fetch when there is no version to compare against")
	}
}

func TestWorseSeverity(t *testing.T) {
	for _, tc := range []struct{ a, b, want string }{
		{"", "high", "high"},
		{"high", "critical", "critical"},
		{"critical", "high", "critical"},
		{"medium", "moderate", "medium"}, // same rank, first wins
		{"high", "low", "high"},
		{"high", "unrecognised", "high"}, // an unknown severity must not outrank a known one
		{"", "", ""},
	} {
		if got := worseSeverity(tc.a, tc.b); got != tc.want {
			t.Errorf("worseSeverity(%q, %q) = %q, want %q", tc.a, tc.b, got, tc.want)
		}
	}
}
