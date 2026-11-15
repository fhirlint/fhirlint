// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Every fhirlint.yml key of validate reaches the run (#628). Each case sets
// one key in the config and no flag, and checks that the value took effect:
// in the fake validator's arguments, or in what the run does. A key that is
// read under the wrong name, or not at all, fails its case, which is the class
// of bug #597 was.
//
// Each case was checked against an empty fhirlint.yml: none passes without its
// key. Not here, on purpose: watch, watch-interval and watch-settle-time (a
// watch run doesn't end), no-color (test output is no terminal, so it is plain
// either way) and package-baseline (it needs an IG with a published QA
// baseline in the package cache).

type configCase struct {
	name  string
	yaml  string            // the fhirlint.yml body
	files map[string]string // extra files in the working directory
	args  []string          // validate arguments (inputs), never the key's flag
	// jarArgs: each pair (flag, value) must appear in a validator call; an
	// empty value means the flag alone.
	jarArgs [][2]string
	check   func(t *testing.T, dir string, r fakeRun)
}

func TestFakeConfigKeysReachTheRun(t *testing.T) {
	outside := t.TempDir() // a cache dir from fhirlint.yml must not be in the repository
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(errPatient))
	}))
	defer srv.Close()
	cases := []configCase{
		// Keys that become validator arguments.
		{name: "fhir-version", yaml: "fhir-version: 5.0.0", jarArgs: [][2]string{{"-version", "5.0.0"}}},
		{name: "ig", yaml: "ig: [example.ig#1.0.0]", jarArgs: [][2]string{{"-ig", "example.ig#1.0.0"}}},
		{name: "profile", yaml: "profile: [http://example.org/StructureDefinition/p]", jarArgs: [][2]string{{"-profile", "http://example.org/StructureDefinition/p"}}},
		{name: "terminology-server", yaml: "terminology-server: https://tx.example.org/r4", jarArgs: [][2]string{{"-tx", "https://tx.example.org/r4"}}},
		{name: "no-terminology-server", yaml: "no-terminology-server: true", jarArgs: [][2]string{{"-tx", "n/a"}}},
		{name: "best-practice", yaml: "best-practice: error", jarArgs: [][2]string{{"-best-practice", "error"}}},
		{name: "locale", yaml: "locale: de", jarArgs: [][2]string{{"-locale", "de"}}},
		{name: "jurisdiction", yaml: `jurisdiction: "urn:iso:std:iso:3166#DE"`, jarArgs: [][2]string{{"-jurisdiction", "urn:iso:std:iso:3166#DE"}}},
		{name: "allow-example-urls", yaml: "allow-example-urls: true", jarArgs: [][2]string{{"-allow-example-urls", ""}}},
		{name: "display-issues-are-warnings", yaml: "display-issues-are-warnings: true", jarArgs: [][2]string{{"-display-issues-are-warnings", ""}}},
		{name: "codesystem-size-limit", yaml: "codesystem-size-limit: 500", jarArgs: [][2]string{{"-codesystem-validation-size-limit", "500"}}},
		{name: "max-messages", yaml: "max-messages: 10", jarArgs: [][2]string{{"-max-validation-messages", "10"}}},
		{name: "validation-timeout", yaml: "validation-timeout: 30s", jarArgs: [][2]string{{"-validation-timeout", ""}}},
		{name: "tx-cache", yaml: "tx-cache: txc", jarArgs: [][2]string{{"-txCache", "txc"}}},
		{name: "tx-log", yaml: "tx-log: tx.log", jarArgs: [][2]string{{"-txLog", "tx.log"}}},
		{name: "proxy", yaml: "proxy: proxy.example.org:3128", jarArgs: [][2]string{{"-proxy", "proxy.example.org:3128"}}},
		{name: "https-proxy", yaml: "https-proxy: proxy.example.org:3129", jarArgs: [][2]string{{"-https-proxy", "proxy.example.org:3129"}}},
		{name: "po", yaml: "po: [msgs.po]", files: map[string]string{"msgs.po": "msgid \"x\"\nmsgstr \"y\"\n"}, jarArgs: [][2]string{{"-po", ""}}},
		{name: "expansion-parameters", yaml: "expansion-parameters: exp.json", files: map[string]string{"exp.json": `{"resourceType":"Parameters"}`}, jarArgs: [][2]string{{"-expansion-parameters", ""}}},
		{name: "offline", yaml: "offline: true", jarArgs: [][2]string{{"-no-http-access", ""}, {"-tx", "n/a"}}},
		{name: "allow-insecure-tx", yaml: "terminology-server: http://tx.example.org/r4\nallow-insecure-tx: true", jarArgs: [][2]string{{"-tx", "http://tx.example.org/r4"}}, check: func(t *testing.T, _ string, r fakeRun) {
			if strings.Contains(r.stderr, "uses HTTP") {
				t.Errorf("allow-insecure-tx must silence the plain-HTTP warning:\n%s", r.stderr)
			}
		}},
		// The counterpart: without the key the warning is there, so the case
		// above can only pass by reading it.
		{name: "terminology-server over http warns", yaml: "terminology-server: http://tx.example.org/r4", jarArgs: [][2]string{{"-tx", "http://tx.example.org/r4"}}, check: func(t *testing.T, _ string, r fakeRun) {
			if !strings.Contains(r.stderr, "uses HTTP") {
				t.Errorf("a plain-HTTP terminology server must warn:\n%s", r.stderr)
			}
		}},
		{name: "validator-arg", yaml: `validator-arg: ["-recurse"]`, jarArgs: [][2]string{{"-recurse", ""}}},

		// Keys that change what the run does.
		{name: "fail-on", yaml: "fail-on: warning", args: []string{"w.json"}, check: wantFail},
		{name: "max-warnings", yaml: "max-warnings: 0", args: []string{"w.json"}, check: wantErr},
		{name: "severity", yaml: "severity: error", args: []string{"w.json"}, check: func(t *testing.T, _ string, r fakeRun) {
			wantPass(t, "", r)
			if strings.Contains(r.stdout, "Fake warning found") {
				t.Errorf("severity: error must hide the warning:\n%s", r.stdout)
			}
		}},
		{name: "format and output", yaml: "format: [json]\noutput: rep.json", args: []string{"w.json"}, check: func(t *testing.T, dir string, r fakeRun) {
			if data, err := os.ReadFile(filepath.Join(dir, "rep.json")); err != nil || !strings.Contains(string(data), "FAKE_WARNING_ID") { //nolint:gosec // test output
				t.Errorf("format and output from the config must write the JSON report: %v", err)
			}
		}},
		{name: "quiet", yaml: "quiet: true", args: []string{"ok.json", "w.json"}, check: func(t *testing.T, _ string, r fakeRun) {
			if strings.Contains(r.stdout, "ok.json") {
				t.Errorf("quiet must leave the valid file out:\n%s", r.stdout)
			}
		}},
		{name: "group", yaml: "group: true", args: []string{"w.json", "w2.json"}, check: func(t *testing.T, _ string, r fakeRun) {
			if !strings.Contains(r.stdout, "2 files") {
				t.Errorf("group must group the repeated warning:\n%s", r.stdout)
			}
		}},
		{name: "show-source", yaml: "show-source: true", args: []string{"w.json"}, check: func(t *testing.T, _ string, r fakeRun) {
			if !strings.Contains(r.stdout, `"note":"FAKE_WARNING"`) {
				t.Errorf("show-source must quote the line:\n%s", r.stdout)
			}
		}},
		{name: "redact", yaml: "redact: true\nformat: [json]\noutput: rep.json", args: []string{"w.json"}, check: func(t *testing.T, dir string, _ fakeRun) {
			data, err := os.ReadFile(filepath.Join(dir, "rep.json")) //nolint:gosec // test output
			if err != nil || !strings.Contains(string(data), "FAKE_WARNING_ID") || strings.Contains(string(data), "Fake warning found") {
				t.Errorf("redact must remove the message text:\n%s", data)
			}
		}},
		{name: "show-suppressed", yaml: "show-suppressed: true\nsuppress:\n  - messageId: FAKE_WARNING_ID\n    reason: known", args: []string{"w.json"}, check: func(t *testing.T, _ string, r fakeRun) {
			if !strings.Contains(r.stdout, "↷ SUPP") || !strings.Contains(r.stdout, "known") {
				t.Errorf("show-suppressed must show the suppressed finding and its reason:\n%s", r.stdout)
			}
		}},
		{name: "require-suppress-reason", yaml: "require-suppress-reason: true\nsuppress:\n  - messageId: FAKE_WARNING_ID", args: []string{"w.json"}, check: wantErr},
		{name: "exclude", yaml: "exclude: [skip/]", args: []string{"."}, check: func(t *testing.T, _ string, r fakeRun) {
			if strings.Contains(r.stdout, "bad.json") {
				t.Errorf("exclude must leave skip/ out:\n%s", r.stdout)
			}
		}},
		{name: "ignore", yaml: `ignore: ["$.note"]`, args: []string{"e.json"}, check: wantPass},
		{name: "skip-non-fhir", yaml: "skip-non-fhir: true", args: []string{"mixed"}, check: func(t *testing.T, _ string, r fakeRun) {
			wantPass(t, "", r)
			if strings.Contains(r.stdout, "package.json") {
				t.Errorf("skip-non-fhir must skip package.json:\n%s", r.stdout)
			}
		}},
		{name: "bundle-entries", yaml: "bundle-entries: true", args: []string{"b.json"}, check: func(t *testing.T, _ string, r fakeRun) {
			if !strings.Contains(r.stdout, "Files: 2") {
				t.Errorf("bundle-entries must validate the entry too:\n%s", r.stdout)
			}
		}},
		{name: "check-references", yaml: "check-references: true", args: []string{"obs.json"}, check: func(t *testing.T, _ string, r fakeRun) {
			if !strings.Contains(r.stdout, "Patient/missing") {
				t.Errorf("check-references must report the dangling reference:\n%s", r.stdout)
			}
		}},
		{name: "extract", yaml: `extract: "$.data"`, args: []string{"env.json"}, check: wantPass},
		{name: "extract-each", yaml: `extract-each: "$.items"`, args: []string{"list.json"}, check: func(t *testing.T, _ string, r fakeRun) {
			if !strings.Contains(r.stdout, "Files: 2") {
				t.Errorf("extract-each must validate each element:\n%s", r.stdout)
			}
		}},
		{name: "slowest", yaml: "slowest: 1", args: []string{"w.json"}, check: func(t *testing.T, _ string, r fakeRun) {
			if !strings.Contains(r.stdout, "No validation times") {
				t.Errorf("slowest must add its section:\n%s", r.stdout)
			}
		}},
		{name: "cache and cache-dir", yaml: "cache: true\ncache-dir: " + outside, args: []string{"w.json"}, check: func(t *testing.T, _ string, _ fakeRun) {
			if entries, _ := os.ReadDir(outside); len(entries) == 0 {
				t.Error("cache and cache-dir must write the result cache there")
			}
		}},
		{name: "baseline", yaml: "baseline: base.json", files: map[string]string{"base.json": ""}, args: []string{"e.json"}, check: wantPass},
		{name: "url", yaml: "url: [" + srv.URL + "/Patient/p2]", args: []string{}, check: func(t *testing.T, _ string, r fakeRun) {
			mustFail(t, r)
			if !strings.Contains(r.stdout, srv.URL+"/Patient/p2") {
				t.Errorf("url from the config must be fetched and validated:\n%s", r.stdout)
			}
		}},
		{name: "server", yaml: "server: http://127.0.0.1:1", args: []string{"w.json"}, check: wantErr},
		{name: "since", yaml: "since: main", args: []string{"."}, check: func(t *testing.T, _ string, r fakeRun) {
			if r.err == nil || !strings.Contains(r.err.Error(), "--since needs a git repository") { // the temp dir is none
				t.Errorf("since from the config must be applied, got %v", r.err)
			}
		}},
		{name: "tx-offline and tx-dir", yaml: "tx-offline: true\ntx-dir: no-such-recording", args: []string{"w.json"}, check: func(t *testing.T, _ string, r fakeRun) {
			if r.err == nil || !strings.Contains(r.err.Error(), "no-such-recording") {
				t.Errorf("tx-offline must look for the recording in tx-dir, got %v", r.err)
			}
		}},
		{name: "timeout", yaml: "timeout: soon", args: []string{"w.json"}, check: wantErr},
		{name: "url-timeout", yaml: "url-timeout: soon", args: []string{"w.json"}, check: wantErr},
	}

	files := map[string]string{
		"ok.json":            cleanPatient,
		"w.json":             warnPatient,
		"w2.json":            strings.Replace(warnPatient, "p3", "p4", 1),
		"e.json":             errPatient,
		"skip/bad.json":      errPatient,
		"mixed/p.json":       cleanPatient,
		"mixed/package.json": `{"name":"not-fhir","version":"1.0.0"}`,
		"b.json":             `{"resourceType":"Bundle","type":"collection","entry":[{"resource":` + cleanPatient + `}]}`,
		"obs.json":           `{"resourceType":"Observation","id":"o1","subject":{"reference":"Patient/missing"}}`,
		"env.json":           `{"data":` + cleanPatient + `,"note":"FAKE_ERROR"}`, // the error is outside the extracted part
		"list.json":          `{"items":[` + cleanPatient + `,` + cleanPatient + `]}`,
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, body := range files {
				writeFile(t, dir, name, body)
			}
			for name, body := range tc.files {
				writeFile(t, dir, name, body)
			}
			if tc.name == "baseline" {
				// A baseline that records e.json's error, made by a run first.
				prepareFake(t)
				if r := runFakeKeepEnv(t, dir, "validate", "--generate-baseline", "base.json", "e.json"); r.err != nil {
					t.Fatal(r.err)
				}
			}
			writeFile(t, dir, "fhirlint.yml", tc.yaml+"\n")
			args := tc.args
			if args == nil { // an empty, non-nil list means no inputs at all (url)
				args = []string{"ok.json"}
			}
			log := filepath.Join(t.TempDir(), "calls.jsonl")
			prepareFake(t)
			t.Setenv(fakeJavaLogEnv, log)
			r := runFakeKeepEnv(t, dir, append([]string{"validate"}, args...)...)
			if tc.check != nil {
				tc.check(t, dir, r)
			}
			if len(tc.jarArgs) > 0 {
				calls := fakeJavaCalls(t, log)
				for _, want := range tc.jarArgs {
					found := false
					for _, c := range calls {
						if want[1] == "" && slices.Contains(c, want[0]) || slices.Contains(argAfter(c, want[0]), want[1]) {
							found = true
						}
					}
					if !found {
						t.Errorf("%s from fhirlint.yml did not reach the validator as %s %s: %v\n%s", tc.name, want[0], want[1], calls, r.stderr)
					}
				}
			}
		})
	}
}

func wantPass(t *testing.T, _ string, r fakeRun) {
	t.Helper()
	if r.err != nil {
		t.Errorf("want success, got %v\n%s\n%s", r.err, r.stdout, r.stderr)
	}
}

func wantFail(t *testing.T, _ string, r fakeRun) {
	t.Helper()
	mustFail(t, r)
}

func wantErr(t *testing.T, _ string, r fakeRun) {
	t.Helper()
	if r.err == nil {
		t.Errorf("want an error, got success\n%s", r.stdout)
	}
}
