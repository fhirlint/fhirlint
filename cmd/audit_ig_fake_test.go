// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/igaudit"
)

// The IG half of audit against a local registry (#690).

func fakeRegistry(t *testing.T, packuments map[string]string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := packuments[strings.TrimPrefix(r.URL.Path, "/")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	orig := newIGAuditClient
	newIGAuditClient = func() *igaudit.Client {
		return &igaudit.Client{Registries: []string{srv.URL}, HTTP: srv.Client()}
	}
	t.Cleanup(func() { newIGAuditClient = orig })
}

// packumentJSON builds a registry document; deprecated names versions with a note.
func packumentJSON(latest string, versions []string, deprecated map[string]string) string {
	vs := map[string]any{}
	for _, v := range versions {
		entry := map[string]any{"version": v}
		if note, ok := deprecated[v]; ok {
			entry["deprecated"] = note
		}
		vs[v] = entry
	}
	b, _ := json.Marshal(map[string]any{"dist-tags": map[string]string{"latest": latest}, "versions": vs})
	return string(b)
}

var auditPackuments = map[string]string{
	"good.pkg":     packumentJSON("1.0.0", []string{"1.0.0"}, nil),
	"old.pkg":      packumentJSON("1.1.0", []string{"1.0.0", "1.1.0"}, nil),
	"nover.pkg":    packumentJSON("1.0.0", []string{"1.0.0"}, nil),
	"dep.pkg":      packumentJSON("1.0.0", []string{"1.0.0"}, map[string]string{"1.0.0": "use new.pkg"}),
	"ahead.pkg":    packumentJSON("1.5.0", []string{"1.5.0", "2.0.0"}, nil),
	"pre.pkg":      packumentJSON("1.1.0-ballot", []string{"1.0.0", "1.1.0-ballot"}, nil),
	"untagged.pkg": packumentJSON("1.0.0", []string{"1.0.0", "1.2.0"}, nil),
	"cal.pkg":      packumentJSON("2025-Q2", []string{"2025-Q1", "2025-Q2"}, nil),
}

const auditIGConfig = `ig:
  - good.pkg#1.0.0
  - old.pkg#1.0.0
  - gone.pkg#1.0.0
  - nover.pkg#9.9.9
  - dep.pkg#1.0.0
  - ahead.pkg#2.0.0
  - pre.pkg#1.0.0
  - untagged.pkg#1.0.0
  - cal.pkg#2025-Q1
  - unpinned.pkg
`

func TestFakeAudit_IGPackagesFromTheConfig(t *testing.T) {
	prepareFake(t)
	fakeGitHubAPI(t, "7.0.5", "[]", 0)
	fakeRegistry(t, auditPackuments)
	dir := t.TempDir()
	writeFile(t, dir, "fhirlint.yml", auditIGConfig)

	r := runFakeKeepEnv(t, dir, "audit")
	if code := auditExit(t, r); code != exitIGIssue {
		t.Fatalf("problems with IG packages only exit %d, got %d\n%s\n%s", exitIGIssue, code, r.stdout, r.stderr)
	}
	out := r.stdout + r.stderr
	for _, want := range []string{
		"IG packages (fhirlint.yml)",
		"good.pkg      1.0.0 — current",
		"old.pkg       1.0.0 → 1.1.0 available",
		"gone.pkg      1.0.0 — not found on any registry",
		"nover.pkg     9.9.9 — the package exists, but the registry has no such version",
		"dep.pkg       1.0.0 — deprecated upstream: use new.pkg",
		"ahead.pkg     2.0.0 — ahead of registry latest (1.5.0)",
		"pre.pkg       1.0.0 — current release; registry latest is a pre-release (1.1.0-ballot)",
		"also on the registry, not tagged latest: 1.2.0",
		"cal.pkg       2025-Q1 — registry latest is 2025-Q2 (versions not comparable)",
		"unpinned.pkg  not pinned — no version to check",
		"need attention",
		"note: from the ig: list",
		"IG package issue(s) found.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("audit lacks %q:\n%s", want, out)
		}
	}
}

func TestFakeAudit_IGPackagesAsJSON(t *testing.T) {
	prepareFake(t)
	fakeGitHubAPI(t, "7.0.5", "[]", 0)
	fakeRegistry(t, auditPackuments)
	dir := t.TempDir()
	writeFile(t, dir, "fhirlint.yml", auditIGConfig)

	r := runFakeKeepEnv(t, dir, "audit", "--format", "json")
	if r.err != nil {
		t.Fatalf("--format json always exits 0: %v", r.err)
	}
	var got struct {
		IGSource   string           `json:"igSource"`
		IGUnpinned []string         `json:"igUnpinned"`
		LockFile   string           `json:"lockFile"`
		IGPackages []map[string]any `json:"igPackages"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &got); err != nil {
		t.Fatalf("%v\n%s", err, r.stdout)
	}
	if got.IGSource != "fhirlint.yml" || len(got.IGUnpinned) != 1 || got.LockFile != "" || len(got.IGPackages) != 9 {
		t.Errorf("JSON: %+v", got)
	}
}

// A lock file wins over the config, and a broken one is reported, not taken
// for "no packages".
func TestFakeAudit_IGPackagesFromTheLockFile(t *testing.T) {
	prepareFake(t)
	fakeGitHubAPI(t, "7.0.5", "[]", 0)
	fakeRegistry(t, auditPackuments)
	dir := t.TempDir()
	writeFile(t, dir, "fhirlint.yml", "ig: [old.pkg#1.0.0]\n")
	writeFile(t, dir, "fhirlint.lock", `{"packages":{"good.pkg#1.0.0":{"url":"https://packages.fhir.org/good.pkg/1.0.0","sha256":"x"}}}`)

	r := runFakeKeepEnv(t, dir, "audit")
	if code := auditExit(t, r); code != 0 {
		t.Fatalf("a current lock file is clean, got exit %d\n%s%s", code, r.stdout, r.stderr)
	}
	if !strings.Contains(r.stdout, "IG packages (fhirlint.lock)") || strings.Contains(r.stdout, "old.pkg") || strings.Contains(r.stdout, "note: from the ig: list") {
		t.Errorf("the lock file must be the source:\n%s", r.stdout)
	}
	r = runFakeKeepEnv(t, dir, "audit", "--format", "json")
	if !strings.Contains(r.stdout, `"lockFile": "fhirlint.lock"`) {
		t.Errorf("JSON names the lock file:\n%s", r.stdout)
	}

	writeFile(t, dir, "fhirlint.lock", `{not json`)
	r = runFakeKeepEnv(t, dir, "audit")
	if !strings.Contains(r.stderr, "reading fhirlint.lock") {
		t.Errorf("a broken lock file must be reported:\n%s%s", r.stdout, r.stderr)
	}
	r = runFakeKeepEnv(t, dir, "audit", "--format", "json")
	if !strings.Contains(r.stdout, `"igError"`) {
		t.Errorf("JSON carries the error:\n%s", r.stdout)
	}
}

// On a terminal outside CI both components get a row, aligned as a table,
// with the advisory block below them.
func TestFakeUpdateNotice_RowsOnATerminal(t *testing.T) {
	prepareFake(t)
	fakeGitHubAPI(t, "7.0.6", "["+affecting+"]", 0)
	t.Setenv("CI", "")
	t.Setenv(noUpdateNotifierEnvVar, "")
	origTTY, origVersion, origLatest := stderrIsTerminal, version, latestFhirlintRelease
	stderrIsTerminal = func() bool { return true }
	version = "v1.0.0"
	latestFhirlintRelease = func(string) string { return "v1.2.0" }
	t.Cleanup(func() { stderrIsTerminal, version, latestFhirlintRelease = origTTY, origVersion, origLatest })
	dir := t.TempDir()
	writeFile(t, dir, "p.json", cleanPatient)

	r := runFakeKeepEnv(t, dir, "validate", "p.json")
	mustPass(t, r)
	for _, want := range []string{
		"Updates available:",
		"  fhirlint   v1.0.0 → v1.2.0",
		"https://github.com/" + fhirlintRepo + "/releases",
		"  validator  7.0.5 → 7.0.6",
		"run: fhirlint update",
		"Security: 1 published advisory/advisories affect validator 7.0.5",
	} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("notice lacks %q:\n%s", want, r.stderr)
		}
	}

	// The action column lines up across the rows.
	var actionCol []int
	for _, line := range strings.Split(r.stderr, "\n") {
		for _, action := range []string{"https://", "run: "} {
			if i := strings.Index(line, action); i > 0 && strings.Contains(line, " → ") {
				actionCol = append(actionCol, len([]rune(line[:i])))
			}
		}
	}
	if len(actionCol) != 2 || actionCol[0] != actionCol[1] {
		t.Errorf("the action column must align, got columns %v:\n%s", actionCol, r.stderr)
	}

	// Up to date on both: no block at all.
	latestFhirlintRelease = func(string) string { return "v1.0.0" }
	fakeGitHubAPI(t, "7.0.5", "[]", 0)
	r = runFakeKeepEnv(t, dir, "validate", "p.json")
	if strings.Contains(r.stderr, "Updates available") || strings.Contains(r.stderr, "Security:") {
		t.Errorf("nothing to report must print nothing:\n%s", r.stderr)
	}
}
