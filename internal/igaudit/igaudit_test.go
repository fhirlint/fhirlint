package igaudit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/igaudit"
)

// packumentServer serves packuments from a name -> JSON body map. Any name not
// in the map answers 404, which is what both registries do for unknown packages.
func packumentServer(t *testing.T, bodies map[string]string) *httptest.Server {
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
	return srv
}

// registry is a client over a single packument server — the case where the
// primary answers, which is every test that is not about the fallback.
func registry(t *testing.T, bodies map[string]string) *igaudit.Client {
	t.Helper()
	srv := packumentServer(t, bodies)
	return &igaudit.Client{Registries: []string{srv.URL}, HTTP: srv.Client()}
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
	// A version that cannot be ordered must never be called "outdated" — nor
	// mistaken for a ballot: "2025-Q1" is a calendar scheme, and IsPreRelease is
	// syntactic enough to say yes to it (#434).
	if p := findPackage(t, r, "odd.pkg#1.0.0"); !p.Differs || p.Outdated || p.LatestIsPreRelease {
		t.Errorf("odd.pkg: got differs=%v outdated=%v latestIsPreRelease=%v, want true/false/false",
			p.Differs, p.Outdated, p.LatestIsPreRelease)
	}
	if p := findPackage(t, r, "gone.pkg#1.0.0"); !p.NotFound || p.Error != "" {
		t.Errorf("gone.pkg: got notFound=%v err=%q, want true/empty", p.NotFound, p.Error)
	}

	if got, want := r.Problems(), 3; got != want {
		t.Errorf("Problems() = %d, want %d", got, want)
	}
}

// A pin at a version the registry does not serve is as broken as a missing
// package, and used to pass silently: the alias table pointed `mii` at
// de.medizininformatikinitiative.kerndatensatz#2024.0.0 for months, and the
// audit reported the package as fine because the *name* resolved (#336).
func TestAuditVersionMissing(t *testing.T) {
	c := registry(t, map[string]string{
		"real.pkg": packument("2.0.0", "1.0.0", "2.0.0"),
		// A packument with no versions map at all: a registry quirk, not
		// evidence about the pin, so it must not be reported as missing.
		"bare.pkg": `{"dist-tags":{"latest":"1.0.0"}}`,
	})

	r := igaudit.Audit(context.Background(), c, []string{"real.pkg#9.9.9", "real.pkg#1.0.0", "bare.pkg#1.0.0"})

	p := findPackage(t, r, "real.pkg#9.9.9")
	if !p.VersionMissing || !p.IsProblem() {
		t.Errorf("real.pkg#9.9.9: got versionMissing=%v problem=%v, want true/true", p.VersionMissing, p.IsProblem())
	}
	// The package itself resolves, so this is not the same finding as NotFound.
	if p.NotFound {
		t.Error("real.pkg#9.9.9: reported as NotFound, but the package exists")
	}
	if p := findPackage(t, r, "real.pkg#1.0.0"); p.VersionMissing {
		t.Error("real.pkg#1.0.0: a version the registry serves was reported as missing")
	}
	if p := findPackage(t, r, "bare.pkg#1.0.0"); p.VersionMissing {
		t.Error("bare.pkg#1.0.0: an empty versions map was read as evidence the pin is wrong")
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
	c := &igaudit.Client{Registries: []string{srv.URL}, HTTP: srv.Client()}

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

// A pin that matches dist-tags.latest is not the same as a pin on the newest
// release. Both cases below are real: fhir.r4.ukcore.stu2 serves 2.1.0 while
// tagging 2.0.2, and de.medizininformatikinitiative.kerndatensatz.icu serves
// 2026.0.3 and 2027.0.0 while tagging 2026.0.2. packages.fhir.org and
// packages.simplifier.net agree on both, so it is upstream declining to move
// the tag rather than a mirror lagging (#406).
func TestAuditUntaggedNewer(t *testing.T) {
	c := registry(t, map[string]string{
		"ukcore.pkg": packument("2.0.2", "2.0.1", "2.0.2", "2.1.0"),
		"icu.pkg": packument("2026.0.2",
			"2026.0.1", "2026.0.2", "2026.0.2-rc.1", "2026.0.3", "2027.0.0", "2027.0.0-ballot.rc1"),
		"kbv.pkg":   packument("1.9.0", "1.8.0", "1.9.0", "1.9.0-Expansions", "1.9.0-Resources"),
		"quiet.pkg": packument("1.6.0", "1.4.0", "1.6.0"),
	})

	r := igaudit.Audit(context.Background(), c, []string{
		"ukcore.pkg#2.0.2", "icu.pkg#2026.0.2", "kbv.pkg#1.9.0", "quiet.pkg#1.4.0",
	})

	// Current by the tag, and still two releases behind what is downloadable.
	p := findPackage(t, r, "ukcore.pkg#2.0.2")
	mustEqualVersions(t, "ukcore.pkg", p.UntaggedNewer, []string{"2.1.0"})
	if p.IsProblem() || p.Outdated {
		t.Errorf("ukcore.pkg: got problem=%v outdated=%v, want false/false — the pin follows the tag on purpose",
			p.IsProblem(), p.Outdated)
	}

	// Pre-releases are not releases: 2026.0.2-rc.1 is older than the pin anyway,
	// but 2027.0.0-ballot.rc1 would qualify on version order alone.
	p = findPackage(t, r, "icu.pkg#2026.0.2")
	mustEqualVersions(t, "icu.pkg", p.UntaggedNewer, []string{"2026.0.3", "2027.0.0"})

	// KBV ships split artifacts beside a release. They carry a suffix, so they
	// are excluded for the same reason a ballot is — neither is a version to
	// point anyone at.
	p = findPackage(t, r, "kbv.pkg#1.9.0")
	mustEqualVersions(t, "kbv.pkg", p.UntaggedNewer, nil)

	// Nothing beyond the tag: the field stays empty rather than repeating latest.
	p = findPackage(t, r, "quiet.pkg#1.4.0")
	mustEqualVersions(t, "quiet.pkg", p.UntaggedNewer, nil)
	if !p.Outdated {
		t.Errorf("quiet.pkg: got outdated=%v, want true", p.Outdated)
	}
}

// An outdated pin already has a finding telling it to move to latest. Repeating
// latest in the untagged list would double-report it, so the list starts above
// latest even when the pin sits below it.
func TestAuditUntaggedNewerExcludesLatestOnAnOutdatedPin(t *testing.T) {
	c := registry(t, map[string]string{
		"ukcore.pkg": packument("2.0.2", "2.0.1", "2.0.2", "2.1.0"),
	})

	r := igaudit.Audit(context.Background(), c, []string{"ukcore.pkg#2.0.1"})

	p := findPackage(t, r, "ukcore.pkg#2.0.1")
	if !p.Outdated || p.Latest != "2.0.2" {
		t.Fatalf("ukcore.pkg: got outdated=%v latest=%q, want true/2.0.2", p.Outdated, p.Latest)
	}
	mustEqualVersions(t, "ukcore.pkg", p.UntaggedNewer, []string{"2.1.0"})
}

// A version the comparator cannot order is skipped rather than guessed at,
// matching how Differs reports instead of assuming.
func TestAuditUntaggedNewerSkipsIncomparableVersions(t *testing.T) {
	c := registry(t, map[string]string{
		"odd.pkg": packument("1.0.0", "1.0.0", "2025-Q1", "1.1.0"),
	})

	r := igaudit.Audit(context.Background(), c, []string{"odd.pkg#1.0.0"})

	mustEqualVersions(t, "odd.pkg", findPackage(t, r, "odd.pkg#1.0.0").UntaggedNewer, []string{"1.1.0"})
}

// dist-tags.latest can be a pre-release. MII moved the tag on all 12
// Kerndatensatz modules to a 2027 ballot in September 2026, and a final pin
// underneath it was reported as "outdated → 2027.0.0-ballot available" — advice
// to move a production pin onto a draft (#434).
func TestAuditLatestIsPreReleaseIsNotOutdated(t *testing.T) {
	c := registry(t, map[string]string{
		"icu.pkg": packument("2027.0.0-ballot.3",
			"2026.0.2", "2026.0.3", "2027.0.0", "2027.0.0-ballot.3"),
	})

	r := igaudit.Audit(context.Background(), c, []string{"icu.pkg#2026.0.2"})
	p := findPackage(t, r, "icu.pkg#2026.0.2")

	if !p.LatestIsPreRelease {
		t.Errorf("icu.pkg: got latestIsPreRelease=false, want true (latest %q)", p.Latest)
	}
	if p.Outdated || p.Differs || p.Ahead {
		t.Errorf("icu.pkg: got outdated=%v differs=%v ahead=%v, want all false — a ballot is not a release to move to",
			p.Outdated, p.Differs, p.Ahead)
	}
	if p.IsProblem() {
		t.Errorf("icu.pkg: a final pin under a ballot tag must not be a finding: %+v", p)
	}

	// The latest bound on UntaggedNewer is dropped when latest is a pre-release,
	// so the finals between the pin and the ballot are listed too. With the bound
	// in place this reported 2027.0.0 alone and hid 2026.0.3.
	mustEqualVersions(t, "icu.pkg", p.UntaggedNewer, []string{"2026.0.3", "2027.0.0"})
}

// Only a *final* pin gets the softer treatment. Someone tracking a ballot round
// deliberately still wants to hear that the round moved on.
func TestAuditPreReleasePinUnderNewerPreReleaseIsOutdated(t *testing.T) {
	c := registry(t, map[string]string{
		"isik.pkg": packument("6.0.0-rc2", "6.0.0-rc1", "6.0.0-rc2"),
	})

	r := igaudit.Audit(context.Background(), c, []string{"isik.pkg#6.0.0-rc1"})
	p := findPackage(t, r, "isik.pkg#6.0.0-rc1")

	if p.LatestIsPreRelease {
		t.Errorf("isik.pkg: got latestIsPreRelease=true, want false — both sides are pre-releases")
	}
	if !p.Outdated {
		t.Errorf("isik.pkg: got outdated=false, want true (pin %q, latest %q)", p.Version, p.Latest)
	}
}

// KBV publishes 1.9.0-Expansions and 1.9.0-Resources beside 1.9.0. They are
// split artifacts, not a ballot round, and they are never the tag — so a KBV
// pin must land in the plain "current" case, not the pre-release one.
func TestAuditSplitArtifactsDoNotTriggerThePreReleaseCase(t *testing.T) {
	c := registry(t, map[string]string{
		"kbv.pkg": packument("1.9.0", "1.8.0", "1.9.0", "1.9.0-Expansions", "1.9.0-Resources"),
	})

	r := igaudit.Audit(context.Background(), c, []string{"kbv.pkg#1.9.0"})
	p := findPackage(t, r, "kbv.pkg#1.9.0")

	if p.LatestIsPreRelease || p.IsProblem() {
		t.Errorf("kbv.pkg: got latestIsPreRelease=%v problem=%v, want false/false", p.LatestIsPreRelease, p.IsProblem())
	}
	mustEqualVersions(t, "kbv.pkg", p.UntaggedNewer, nil)
}

func mustEqualVersions(t *testing.T, name string, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s: UntaggedNewer = %v, want %v", name, got, want)
	}
}

// The validator asks packages2.fhir.org first and packages.fhir.org second, and
// the two do not carry the same set: HL7 pre-releases reach the primary first
// (hl7.fhir.r6.core#6.0.0-snapshot1 sat there for a week while the fallback
// answered 404). An audit that asks only one host disagrees with validate about
// what exists (#427).
func TestAuditFallsBackToTheSecondRegistry(t *testing.T) {
	primary := packumentServer(t, map[string]string{
		"hl7.fhir.r6.core": packument("6.0.0-snapshot1", "6.0.0-snapshot1"),
	})
	secondary := packumentServer(t, map[string]string{
		"kbv.basis": packument("1.9.0", "1.9.0"),
	})
	c := &igaudit.Client{Registries: []string{primary.URL, secondary.URL}, HTTP: primary.Client()}

	r := igaudit.Audit(context.Background(), c, []string{
		"hl7.fhir.r6.core#6.0.0-snapshot1", // primary only
		"kbv.basis#1.9.0",                  // secondary only
		"does.not.exist#1.0.0",             // neither
	})

	r6 := findPackage(t, r, "hl7.fhir.r6.core#6.0.0-snapshot1")
	if r6.NotFound || r6.Error != "" || r6.IsProblem() {
		t.Errorf("primary-only package: %+v", r6)
	}
	if r6.Registry != primary.URL {
		t.Errorf("Registry = %q, want the primary %q", r6.Registry, primary.URL)
	}

	kbv := findPackage(t, r, "kbv.basis#1.9.0")
	if kbv.NotFound || kbv.Error != "" || kbv.IsProblem() {
		t.Errorf("secondary-only package: %+v", kbv)
	}
	if kbv.Registry != secondary.URL {
		t.Errorf("Registry = %q, want the secondary %q", kbv.Registry, secondary.URL)
	}

	missing := findPackage(t, r, "does.not.exist#1.0.0")
	if !missing.NotFound || missing.Error != "" {
		t.Errorf("package on neither registry: %+v, want NotFound", missing)
	}
	if missing.Registry != "" {
		t.Errorf("Registry = %q for a package nobody served", missing.Registry)
	}
}

func TestAuditNotFoundNeedsEveryRegistry(t *testing.T) {
	// The primary is down and the fallback has never heard of the package.
	// That is not "not found": the host that would know could not be asked.
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer down.Close()
	secondary := packumentServer(t, map[string]string{})
	c := &igaudit.Client{Registries: []string{down.URL, secondary.URL}, HTTP: secondary.Client()}

	r := igaudit.Audit(context.Background(), c, []string{"hl7.fhir.r6.core#6.0.0-snapshot1"})

	p := r.Packages[0]
	if p.NotFound {
		t.Error("NotFound set although one registry could not be reached")
	}
	if p.Error == "" {
		t.Error("want the primary's failure recorded in Error")
	}
	if r.Problems() != 0 || r.Errors() != 1 {
		t.Errorf("Problems() = %d, Errors() = %d; want 0 and 1", r.Problems(), r.Errors())
	}
}

func TestNewClientAsksTheValidatorsRegistriesInOrder(t *testing.T) {
	c := igaudit.NewClient()
	if len(c.Registries) != 2 || c.Registries[0] != "https://packages2.fhir.org/packages" || c.Registries[1] != "https://packages.fhir.org" {
		t.Errorf("Registries = %v, want packages2 first and packages.fhir.org second, like the JAR", c.Registries)
	}
}
