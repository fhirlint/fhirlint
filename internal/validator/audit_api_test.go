// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package validator

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/fhirlint/fhirlint/internal/cache"
	"github.com/fhirlint/fhirlint/internal/updatecheck"
)

// Audit against a local GitHub API (#632; cmd covers the command in #613).

func auditAPI(t *testing.T, latest, advisories int, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/hapifhir/org.hl7.fhir.core/releases/latest":
			w.WriteHeader(latest)
			_, _ = w.Write([]byte(`{"tag_name":"7.0.6"}`))
		case "/repos/hapifhir/org.hl7.fhir.core/security-advisories":
			if r.Header.Get("Accept") != "application/vnd.github+json" {
				w.WriteHeader(http.StatusNotAcceptable)
				return
			}
			w.WriteHeader(advisories)
			_, _ = w.Write([]byte(body))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	old := updatecheck.APIBase
	updatecheck.APIBase = srv.URL
	t.Cleanup(func() { updatecheck.APIBase = old })
}

func cachedVersion(t *testing.T, v string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("FHIRLINT_CACHE_DIR", "")
	path, err := cache.ValidatorVersionPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(v+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

const twoAdvisories = `[
 {"ghsa_id":"GHSA-old","summary":"fixed long ago","severity":"high",
  "vulnerabilities":[{"package":{"name":"ca.uhn.hapi.fhir:org.hl7.fhir.core"},"vulnerable_version_range":"< 6.0.0"}]},
 {"ghsa_id":"GHSA-new","summary":"affects 7.0.x","severity":"critical",
  "vulnerabilities":[{"package":{"name":"ca.uhn.hapi.fhir:org.hl7.fhir.core"},"vulnerable_version_range":">= 7.0.0, < 7.0.6"}]}
]`

func TestAudit_ReportsVersionAndAdvisories(t *testing.T) {
	cachedVersion(t, "7.0.5")
	auditAPI(t, http.StatusOK, http.StatusOK, twoAdvisories)

	r := Audit()
	if r.CurrentVersion != "7.0.5" || r.LatestVersion != "7.0.6" || !r.IsOutdated || r.VersionError != "" || r.AdvisoryError != "" {
		t.Errorf("report: %+v", r)
	}
	if len(r.Advisories) != 2 {
		t.Fatalf("advisories: %+v", r.Advisories)
	}
	if a := r.AffectingAdvisories(); len(a) != 1 || a[0].GHSAID != "GHSA-new" {
		t.Errorf("only the advisory whose range holds 7.0.5 affects it: %+v", a)
	}
}

func TestAudit_ReportsWhatFailed(t *testing.T) {
	cachedVersion(t, "7.0.6")
	auditAPI(t, http.StatusOK, http.StatusServiceUnavailable, "")
	r := Audit()
	if r.IsOutdated || r.AdvisoryError != "HTTP 503" || r.Advisories != nil {
		t.Errorf("an advisory lookup that fails is reported, not empty: %+v", r)
	}

	auditAPI(t, http.StatusOK, http.StatusOK, `{"not":"a list"}`)
	if r := Audit(); r.AdvisoryError == "" {
		t.Errorf("an unreadable answer is an error: %+v", r)
	}

	updatecheck.APIBase = "http://127.0.0.1:1"
	if r := Audit(); r.VersionError == "" || r.AdvisoryError == "" || r.LatestVersion != "" {
		t.Errorf("no API at all: %+v", r)
	}
}

func TestUnescapeProperty(t *testing.T) {
	for in, want := range map[string]string{
		`plain`:      "plain",
		`tab\there`:  "tab\there",
		`a\nb\rc\fd`: "a\nb\rc\fd",
		`äö`:         "äö",
		`\u00`:       "u00", // too short for an escape: kept as text rather than an error
		`\uZZZZ`:     "uZZZZ",
		`\:\=\\`:     `:=\`,
		`trailing\`:  `trailing\`,
	} {
		if got := unescapeProperty(in); got != want {
			t.Errorf("unescapeProperty(%q) = %q, want %q", in, got, want)
		}
	}
}
