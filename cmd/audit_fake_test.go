// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/cache"
	"github.com/fhirlint/fhirlint/internal/updatecheck"
)

// fhirlint audit and the update notices against a local GitHub API (#613).

const (
	affecting    = `{"ghsa_id":"GHSA-aaaa-bbbb-cccc","summary":"Path traversal in the validator","severity":"high","html_url":"https://github.com/advisories/GHSA-aaaa-bbbb-cccc","vulnerabilities":[{"package":{"name":"ca.uhn.hapi.fhir:org.hl7.fhir.validation"},"vulnerable_version_range":"< 7.0.6"}]}`
	notAffecting = `{"ghsa_id":"GHSA-dddd-eeee-ffff","summary":"Old XXE","severity":"critical","html_url":"https://github.com/advisories/GHSA-dddd-eeee-ffff","vulnerabilities":[{"package":{"name":"ca.uhn.hapi.fhir:org.hl7.fhir.validation"},"vulnerable_version_range":"< 6.0.0"}]}`
)

// fakeGitHubAPI serves the validator's latest release and advisories, and
// pins the cached validator at 7.0.5. A status of 0 means 200.
func fakeGitHubAPI(t *testing.T, latest, advisories string, status int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		switch r.URL.Path {
		case "/repos/hapifhir/org.hl7.fhir.core/releases/latest":
			_, _ = fmt.Fprintf(w, `{"tag_name":%q}`, latest)
		case "/repos/hapifhir/org.hl7.fhir.core/security-advisories":
			_, _ = w.Write([]byte(advisories))
		default:
			_, _ = w.Write([]byte(`{"tag_name":"1.0.0"}`))
		}
	}))
	t.Cleanup(srv.Close)
	orig := updatecheck.APIBase
	updatecheck.APIBase = srv.URL
	t.Cleanup(func() { updatecheck.APIBase = orig })

	dir := t.TempDir()
	t.Setenv(cache.DirEnvVar, dir)
	p, err := cache.ValidatorVersionPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("7.0.5\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func auditExit(t *testing.T, r fakeRun) int {
	t.Helper()
	if r.err == nil {
		return 0
	}
	var ee *exitErr
	if !errors.As(r.err, &ee) {
		t.Fatalf("audit failed without an exit code: %v", r.err)
	}
	return ee.code
}

func TestFakeAudit_UpToDateAndClean(t *testing.T) {
	prepareFake(t)
	fakeGitHubAPI(t, "7.0.5", "["+notAffecting+"]", 0)
	r := runFakeKeepEnv(t, t.TempDir(), "audit")
	if code := auditExit(t, r); code != 0 {
		t.Fatalf("exit %d:\n%s\n%s", code, r.stdout, r.stderr)
	}
	for _, want := range []string{"current:  7.0.5", "✓ up to date", "none affect your version (7.0.5)", "No issues found."} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output lacks %q:\n%s", want, r.stdout)
		}
	}
}

func TestFakeAudit_OnlyAffectingAdvisoriesAreReported(t *testing.T) {
	prepareFake(t)
	fakeGitHubAPI(t, "7.0.5", "["+affecting+","+notAffecting+"]", 0)
	r := runFakeKeepEnv(t, t.TempDir(), "audit")
	if code := auditExit(t, r); code != exitJARIssue {
		t.Fatalf("an advisory against the installed version exits %d, got %d", exitJARIssue, code)
	}
	if !strings.Contains(r.stderr, "GHSA-aaaa-bbbb-cccc") || strings.Contains(r.stderr, "GHSA-dddd-eeee-ffff") {
		t.Errorf("only the advisory that affects 7.0.5 is reported:\n%s", r.stderr)
	}
	if !strings.Contains(r.stdout, "2 advisory/advisories total, 1 not affecting") {
		t.Errorf("the count of all advisories:\n%s", r.stdout)
	}
	if !strings.Contains(r.stderr, "1 issue(s) found.") || strings.Contains(r.stderr, "Error:") {
		t.Errorf("one count line and no second error line:\n%s", r.stderr)
	}
}

func TestFakeAudit_OutdatedAndJSON(t *testing.T) {
	prepareFake(t)
	fakeGitHubAPI(t, "7.0.6", "[]", 0)
	r := runFakeKeepEnv(t, t.TempDir(), "audit")
	if code := auditExit(t, r); code != exitJARIssue || !strings.Contains(r.stderr, "outdated") {
		t.Errorf("an outdated JAR: exit %d\n%s", code, r.stderr)
	}

	// JSON is the whole report and exits 0, so a pipeline decides for itself.
	r = runFakeKeepEnv(t, t.TempDir(), "audit", "--format", "json")
	if code := auditExit(t, r); code != 0 {
		t.Fatalf("--format json exits 0, got %d", code)
	}
	var rep map[string]any
	if err := json.Unmarshal([]byte(r.stdout), &rep); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, r.stdout)
	}
	if !strings.Contains(r.stdout, "7.0.6") {
		t.Errorf("the JSON report carries the latest version:\n%s", r.stdout)
	}
}

func TestFakeAudit_UnreachableAPIIsSaidNotHidden(t *testing.T) {
	prepareFake(t)
	fakeGitHubAPI(t, "", "", http.StatusInternalServerError)
	r := runFakeKeepEnv(t, t.TempDir(), "audit")
	if !strings.Contains(r.stderr, "could not fetch latest version") || !strings.Contains(r.stderr, "could not reach advisory API") {
		t.Errorf("an unreachable API must be named, not read as clean:\n%s", r.stderr)
	}
}

func TestFakeUpdateNotice_AdvisoryLineInCI(t *testing.T) {
	prepareFake(t)
	fakeGitHubAPI(t, "7.0.6", "["+affecting+"]", 0)
	dir := t.TempDir()
	writeFile(t, dir, "p.json", cleanPatient)

	// In CI the update rows are hidden, but the advisory stays visible.
	t.Setenv(noUpdateNotifierEnvVar, "")
	r := runFakeKeepEnv(t, dir, "validate", "p.json")
	mustPass(t, r)
	if !strings.Contains(r.stderr, "Security: 1 published advisory/advisories affect validator 7.0.5 (high or worse). Run: fhirlint audit") {
		t.Errorf("the advisory line must show in CI:\n%s", r.stderr)
	}
	if strings.Contains(r.stderr, "Updates available") {
		t.Errorf("update rows are hidden in CI:\n%s", r.stderr)
	}

	// The opt-out silences both.
	t.Setenv(noUpdateNotifierEnvVar, "1")
	r = runFakeKeepEnv(t, dir, "validate", "p.json")
	if strings.Contains(r.stderr, "Security:") {
		t.Errorf("%s=1 silences the advisory line:\n%s", noUpdateNotifierEnvVar, r.stderr)
	}
}
