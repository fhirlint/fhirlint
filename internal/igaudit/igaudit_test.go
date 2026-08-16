package igaudit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/igaudit"
)

// registry serves packuments from a name -> JSON body map. Any name not in the
// map answers 404, which is what packages.fhir.org does for unknown packages.
func registry(t *testing.T, bodies map[string]string) *igaudit.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		body, ok := bodies[name]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return &igaudit.Client{BaseURL: srv.URL, HTTP: srv.Client()}
}

func packument(latest string, versions ...string) string {
	var sb strings.Builder
	sb.WriteString(`{"dist-tags":{"latest":"` + latest + `"},"versions":{`)
	for i, v := range versions {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`"` + v + `":{"version":"` + v + `"}`)
	}
	sb.WriteString("}}")
	return sb.String()
}

func findPackage(t *testing.T, r igaudit.Report, id string) igaudit.PackageReport {
	t.Helper()
	for _, p := range r.Packages {
		if p.ID == id {
			return p
		}
	}
	t.Fatalf("no report for %q", id)
	return igaudit.PackageReport{}
}

func TestAuditClassifiesPackages(t *testing.T) {
	c := registry(t, map[string]string{
		"kbv.basis":        packument("1.6.0", "1.4.0", "1.6.0"),
		"hl7.fhir.r4.core": packument("4.0.1", "4.0.1"),
		"ahead.pkg":        packument("1.0.0", "1.0.0", "2.0.0"),
		"odd.pkg":          packument("2025-Q1", "1.0.0", "2025-Q1"),
	})

	ids := []string{
		"kbv.basis#1.4.0",
		"hl7.fhir.r4.core#4.0.1",
		"ahead.pkg#2.0.0",
		"odd.pkg#1.0.0",
		"gone.pkg#1.0.0",
	}
	r := igaudit.Audit(context.Background(), c, ids)

	if got := len(r.Packages); got != len(ids) {
		t.Fatalf("got %d reports, want %d", got, len(ids))
	}

	if p := findPackage(t, r, "kbv.basis#1.4.0"); !p.Outdated || p.Latest != "1.6.0" {
		t.Errorf("kbv.basis: got outdated=%v latest=%q, want true/1.6.0", p.Outdated, p.Latest)
	}
	if p := findPackage(t, r, "hl7.fhir.r4.core#4.0.1"); p.IsProblem() {
		t.Errorf("hl7.fhir.r4.core: current version reported as a problem: %+v", p)
	}
	// Ahead of the registry's latest is worth showing but is not a finding:
	// pinning a pre-release deliberately must not fail an audit.
	if p := findPackage(t, r, "ahead.pkg#2.0.0"); !p.Ahead || p.IsProblem() {
		t.Errorf("ahead.pkg: got ahead=%v problem=%v, want true/false", p.Ahead, p.IsProblem())
	}
	// A version that cannot be ordered must never be called "outdated".
	if p := findPackage(t, r, "odd.pkg#1.0.0"); !p.Differs || p.Outdated {
		t.Errorf("odd.pkg: got differs=%v outdated=%v, want true/false", p.Differs, p.Outdated)
	}
	if p := findPackage(t, r, "gone.pkg#1.0.0"); !p.NotFound || p.Error != "" {
		t.Errorf("gone.pkg: got notFound=%v err=%q, want true/empty", p.NotFound, p.Error)
	}

	if got, want := r.Problems(), 3; got != want {
		t.Errorf("Problems() = %d, want %d", got, want)
	}
}

func TestAuditSortsByID(t *testing.T) {
	c := registry(t, map[string]string{
		"a.pkg": packument("1.0.0", "1.0.0"),
		"m.pkg": packument("1.0.0", "1.0.0"),
		"z.pkg": packument("1.0.0", "1.0.0"),
	})

	// Lock file packages come out of a map, so the input order is arbitrary and
	// the report has to impose its own or the output churns between runs.
	r := igaudit.Audit(context.Background(), c, []string{"z.pkg#1.0.0", "a.pkg#1.0.0", "m.pkg#1.0.0"})

	want := []string{"a.pkg#1.0.0", "m.pkg#1.0.0", "z.pkg#1.0.0"}
	for i, w := range want {
		if r.Packages[i].ID != w {
			t.Fatalf("position %d: got %q, want %q", i, r.Packages[i].ID, w)
		}
	}
}

func TestAuditUnreachableRegistryIsNotAFinding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := &igaudit.Client{BaseURL: srv.URL, HTTP: srv.Client()}

	r := igaudit.Audit(context.Background(), c, []string{"kbv.basis#1.4.0"})

	p := r.Packages[0]
	if p.Error == "" {
		t.Fatal("want an error recorded for an unreachable registry")
	}
	if p.IsProblem() {
		t.Error("a registry failure must not be reported as a package problem")
	}
	if r.Errors() != 1 {
		t.Errorf("Errors() = %d, want 1", r.Errors())
	}
	if r.Problems() != 0 {
		t.Errorf("Problems() = %d, want 0", r.Problems())
	}
}

func TestAuditDeprecation(t *testing.T) {
	// The FHIR registry does not populate `deprecated` today, so both the npm
	// string and boolean spellings are covered to make sure the check works the
	// day it starts appearing.
	c := registry(t, map[string]string{
		"noted.pkg":  `{"dist-tags":{"latest":"1.0.0"},"versions":{"1.0.0":{"deprecated":"use other.pkg"}}}`,
		"bool.pkg":   `{"dist-tags":{"latest":"1.0.0"},"versions":{"1.0.0":{"deprecated":true}}}`,
		"undone.pkg": `{"dist-tags":{"latest":"1.0.0"},"versions":{"1.0.0":{"deprecated":""}}}`,
		"plain.pkg":  packument("1.0.0", "1.0.0"),
	})

	r := igaudit.Audit(context.Background(), c,
		[]string{"noted.pkg#1.0.0", "bool.pkg#1.0.0", "undone.pkg#1.0.0", "plain.pkg#1.0.0"})

	if p := findPackage(t, r, "noted.pkg#1.0.0"); !p.Deprecated || p.DeprecationNote != "use other.pkg" {
		t.Errorf("noted.pkg: got deprecated=%v note=%q", p.Deprecated, p.DeprecationNote)
	}
	if p := findPackage(t, r, "bool.pkg#1.0.0"); !p.Deprecated || p.DeprecationNote != "" {
		t.Errorf("bool.pkg: got deprecated=%v note=%q", p.Deprecated, p.DeprecationNote)
	}
	// An empty string is npm's way of undoing a deprecation.
	if p := findPackage(t, r, "undone.pkg#1.0.0"); p.Deprecated {
		t.Error("undone.pkg: empty deprecated string must not count as deprecated")
	}
	if p := findPackage(t, r, "plain.pkg#1.0.0"); p.Deprecated {
		t.Error("plain.pkg: absent deprecated field must not count as deprecated")
	}
}

func TestAuditRejectsNonPackageID(t *testing.T) {
	c := registry(t, map[string]string{})

	r := igaudit.Audit(context.Background(), c, []string{"hand-edited-nonsense"})

	if p := r.Packages[0]; p.Error == "" {
		t.Errorf("want an error for a malformed lock file key, got %+v", p)
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
		ok   bool
	}{
		{"1.4.0", "1.6.0", -1, true},
		{"1.6.0", "1.4.0", 1, true},
		{"1.5.0", "1.5.0", 0, true},
		// Zero-padded segments are common in German IG packages.
		{"1.00.000", "1.0.0", 0, true},
		{"1.00.001", "1.0.0", 1, true},
		// Differing segment counts compare as if the shorter were zero-padded.
		{"2025.0", "2025.0.0", 0, true},
		{"1.5", "1.5.1", -1, true},
		// A release outranks the same core version as a pre-release.
		{"1.0.0", "1.0.0-ballot", 1, true},
		{"1.0.0-ballot", "1.0.0", -1, true},
		{"1.0.0-alpha1", "1.0.0-beta1", -1, true},
		// Anything that is not a dotted run of at least two integers is
		// incomparable, which is what keeps "outdated" from being claimed on a
		// guess. "2025-Q1" is the case that matters: read as core 2025 with a
		// pre-release, it would outrank every 1.x release and turn a change of
		// versioning scheme into a bogus "you are far behind".
		{"2025-Q1", "1.0.0", 0, false},
		{"20250101", "20250501", 0, false},
		{"latest", "1.0.0", 0, false},
		{"", "1.0.0", 0, false},
		{"1.0.x", "1.0.0", 0, false},
	}

	for _, tc := range cases {
		got, ok := igaudit.CompareVersions(tc.a, tc.b)
		if ok != tc.ok {
			t.Errorf("CompareVersions(%q, %q): comparable = %v, want %v", tc.a, tc.b, ok, tc.ok)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
