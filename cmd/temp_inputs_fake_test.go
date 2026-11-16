// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// A URL download and an --extract-each element live in temp files. The rules
// and the reference check read the validated files after the run, so those
// files have to survive until then, and no longer (#695).
func TestFakeValidate_RulesAndReferencesReachTempInputs(t *testing.T) {
	const patient = `{"resourceType":"Patient","id":"p1","link":[{"other":{"reference":"Patient/missing"}}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/list" {
			_, _ = w.Write([]byte(`{"items":[` + patient + `]}`))
			return
		}
		_, _ = w.Write([]byte(patient))
	}))
	defer srv.Close()

	dir := t.TempDir()
	writeFile(t, dir, "fhirlint.yml", "rules:\n  - {id: must-have-name, severity: error, assert: 'name.exists()', message: 'NAME MISSING'}\n")
	writeFile(t, dir, "list.json", `{"items":[`+patient+`]}`)

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"--url", []string{"--url", srv.URL + "/Patient/p1"}},
		{"--extract-each", []string{"--extract-each", "$.items", "list.json"}},
		{"--url with --extract-each", []string{"--extract-each", "$.items", "--url", srv.URL + "/list"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prepareFake(t)
			// A temp dir of the run's own, to see what it leaves behind.
			tmp := t.TempDir()
			for _, v := range []string{"TMPDIR", "TMP", "TEMP"} {
				t.Setenv(v, tmp)
			}
			r := runFakeKeepEnv(t, dir, append([]string{"validate", "--no-color", "--check-references"}, tc.args...)...)
			mustFail(t, r)
			if !strings.Contains(r.stdout, "NAME MISSING") {
				t.Errorf("the custom rule must run:\n%s", r.stdout)
			}
			if !strings.Contains(r.stdout, "Patient/missing") {
				t.Errorf("the reference check must run:\n%s", r.stdout)
			}
			// And the temp files are gone once the run is over.
			left, err := os.ReadDir(tmp)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range left {
				t.Errorf("%s outlived the run", e.Name())
			}
		})
	}
}

// A run that fails after the first download removes it all the same.
func TestFakeValidate_FailedURLRunLeavesNoDownloads(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/gone" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(cleanPatient))
	}))
	defer srv.Close()

	prepareFake(t)
	tmp := t.TempDir()
	for _, v := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(v, tmp)
	}
	r := runFakeKeepEnv(t, t.TempDir(), "validate", "--url", srv.URL+"/Patient/p1", "--url", srv.URL+"/gone")
	if r.err == nil || !strings.Contains(r.err.Error(), "404") {
		t.Fatalf("want the 404 of the second URL, got %v", r.err)
	}
	left, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range left {
		t.Errorf("%s outlived the failed run", e.Name())
	}
}
