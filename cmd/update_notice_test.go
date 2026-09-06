package cmd

import (
	"testing"

	"github.com/fhirlint/fhirlint/internal/cache"
	"github.com/fhirlint/fhirlint/internal/updatecheck"
	"github.com/spf13/viper"
	"strings"
	"time"
)

// clearNoticeEnv removes every ambient reason to suppress the notice, so each
// test controls exactly one variable. The terminal check is left alone: `go
// test` never runs with a terminal on stderr, so suppression is asserted
// directly rather than through updateNoticeSuppressed's full result.
func clearNoticeEnv(t *testing.T) {
	t.Helper()
	t.Setenv(noUpdateNotifierEnvVar, "")
	t.Setenv("CI", "")
	viper.Set("offline", false)
	t.Cleanup(func() { viper.Set("offline", false) })
}

func TestUpdateNoticeSuppressed_OptOutEnvVar(t *testing.T) {
	clearNoticeEnv(t)
	t.Setenv(noUpdateNotifierEnvVar, "1")
	if !updateNoticeSuppressed() {
		t.Errorf("%s must suppress the notice", noUpdateNotifierEnvVar)
	}
}

func TestUpdateNoticeSuppressed_CI(t *testing.T) {
	clearNoticeEnv(t)
	t.Setenv("CI", "true")
	if !updateNoticeSuppressed() {
		t.Error("a pipeline cannot act on the notice, so CI must suppress it")
	}
}

func TestUpdateNoticeSuppressed_Offline(t *testing.T) {
	clearNoticeEnv(t)
	viper.Set("offline", true)
	if !updateNoticeSuppressed() {
		t.Error("offline forbids the network call the check needs")
	}
}

// go test has no terminal on stderr, so this is the baseline case: even with
// every opt-out cleared, the notice stays quiet when nobody is watching.
func TestUpdateNoticeSuppressed_NotATerminal(t *testing.T) {
	clearNoticeEnv(t)
	if !updateNoticeSuppressed() {
		t.Error("without a terminal on stderr the notice has no reader")
	}
}

func TestFhirlintUpdate_NoLdflagMeansNoNotice(t *testing.T) {
	orig := version
	version = "" // a `go build` / `go install` build
	t.Cleanup(func() { version = orig })

	if _, _, ok := fhirlintUpdate(); ok {
		t.Error("a build without the release ldflag has no version to compare, so it must stay quiet")
	}
}

func TestFhirlintUpdate_VersionOrdering(t *testing.T) {
	for _, tc := range []struct {
		name            string
		current, latest string
		want            bool
	}{
		{"newer release available", "v1.9.0", "v1.10.0", true},
		{"up to date", "v1.9.0", "v1.9.0", false},
		{"ahead of the published release", "v1.10.0", "v1.9.0", false},
		{"patch bump", "v1.9.0", "v1.9.1", true},
		{"unparseable current", "banana", "v1.9.0", false},
		{"unparseable latest", "v1.9.0", "banana", false},
		{"lookup failed", "v1.9.0", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			origVersion, origLatest := version, latestFhirlintRelease
			version = tc.current
			latestFhirlintRelease = func(string) string { return tc.latest }
			t.Cleanup(func() { version, latestFhirlintRelease = origVersion, origLatest })

			_, _, ok := fhirlintUpdate()
			if ok != tc.want {
				t.Errorf("fhirlintUpdate() ok = %v, want %v (%s → %s)", ok, tc.want, tc.current, tc.latest)
			}
		})
	}
}

// "1.10.0" sorts before "1.9.0" as a string. The comparison must not do that.
func TestFhirlintUpdate_DoesNotCompareLexically(t *testing.T) {
	origVersion, origLatest := version, latestFhirlintRelease
	version = "v1.10.0"
	latestFhirlintRelease = func(string) string { return "v1.9.0" }
	t.Cleanup(func() { version, latestFhirlintRelease = origVersion, origLatest })

	if _, _, ok := fhirlintUpdate(); ok {
		t.Error("v1.10.0 is newer than v1.9.0; a lexical comparison would wrongly offer a downgrade")
	}
}

// --- a rejected release still shows, but stops recommending itself (#377) ---

func TestValidatorAction_NoRejection(t *testing.T) {
	t.Setenv(cache.DirEnvVar, t.TempDir())
	if got := validatorAction("6.10.3"); got != "run: fhirlint update" {
		t.Errorf("validatorAction() = %q, want the plain update line", got)
	}
}

func TestValidatorAction_RejectedNamesTheReasonAndDate(t *testing.T) {
	t.Setenv(cache.DirEnvVar, t.TempDir())
	updatecheck.RecordRejection("hapifhir/org.hl7.fhir.core", "6.10.3",
		"the PGP signature did not verify")

	got := validatorAction("6.10.3")
	if !strings.Contains(got, "the PGP signature did not verify") {
		t.Errorf("want the reason, got %q", got)
	}
	if !strings.Contains(got, time.Now().Format("2006-01-02")) {
		t.Errorf("want the date, got %q", got)
	}
	// The retry has to stay reachable, or an upstream fix never gets picked up.
	if !strings.Contains(got, "fhirlint update") {
		t.Errorf("want the retry to remain available, got %q", got)
	}
}

// The row itself must survive. Hiding it would leave someone stuck on an older
// JAR with no explanation.
func TestValidatorAction_RejectionDoesNotHideTheRow(t *testing.T) {
	t.Setenv(cache.DirEnvVar, t.TempDir())
	updatecheck.RecordRejection("hapifhir/org.hl7.fhir.core", "6.10.3", "nope")
	if got := validatorAction("6.10.3"); got == "" {
		t.Error("a rejected release must still produce an action line")
	}
}

// A rejection recorded for one release must not silence a different one.
func TestValidatorAction_OtherVersionUnaffected(t *testing.T) {
	t.Setenv(cache.DirEnvVar, t.TempDir())
	updatecheck.RecordRejection("hapifhir/org.hl7.fhir.core", "6.10.3", "nope")
	if got := validatorAction("6.10.4"); got != "run: fhirlint update" {
		t.Errorf("validatorAction(6.10.4) = %q, want the plain update line", got)
	}
}

// --- advisory notice (#385) ------------------------------------------------

// The advisory line is deliberately harder to silence than the update rows:
// a pipeline cannot upgrade itself, but whoever reads the log can act.
func TestAdvisoryNoticeSuppressed_NotByCIOrARedirectedLog(t *testing.T) {
	clearNoticeEnv(t)
	t.Setenv("CI", "true")

	if advisoryNoticeSuppressed() {
		t.Error("CI must not hide a security advisory, even though it hides update rows")
	}
	// go test has no terminal on stderr, so this call also covers the non-TTY
	// case that updateNoticeSuppressed does suppress on.
	if !updateNoticeSuppressed() {
		t.Error("precondition: the update rows should be suppressed here")
	}
}

func TestAdvisoryNoticeSuppressed_ByOptOut(t *testing.T) {
	clearNoticeEnv(t)
	t.Setenv(noUpdateNotifierEnvVar, "1")
	if !advisoryNoticeSuppressed() {
		t.Errorf("%s must silence the advisory line too", noUpdateNotifierEnvVar)
	}
}

func TestAdvisoryNoticeSuppressed_ByOffline(t *testing.T) {
	clearNoticeEnv(t)
	viper.Set("offline", true)
	if !advisoryNoticeSuppressed() {
		t.Error("offline forbids the lookup the advisory line needs")
	}
}
