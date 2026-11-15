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

// A --url input combined with the preprocessing flags, and stdin (#668).

func urlServer(t *testing.T, routes map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/fhir+json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFakeValidate_URLWithExtractEachAndIgnore(t *testing.T) {
	list := `{"items":[` + cleanPatient + `,` + errPatient + `]}`
	srv := urlServer(t, map[string]string{"/list": list, "/env": `{"data":` + errPatient + `}`, "/p": errPatient})
	dir := t.TempDir()

	// Each element of the fetched document, labelled by its place in it.
	r := runFake(t, dir, "validate", "--no-color", "--url", srv.URL+"/list", "--extract-each", "$.items")
	mustFail(t, r)
	if !strings.Contains(r.stdout, "Files: 2") || !strings.Contains(r.stdout, "Fake error found") {
		t.Errorf("--extract-each over a URL must validate each element:\n%s", r.stdout)
	}

	// --ignore applies to the fetched document before the elements are
	// taken out, so its path is one of the outer document.
	mustPass(t, runFakeKeepEnv(t, dir, "validate", "--url", srv.URL+"/list", "--extract-each", "$.items", "--ignore", "$.items[1].note"))

	// --extract and --ignore on a plain --url.
	mustFail(t, runFakeKeepEnv(t, dir, "validate", "--url", srv.URL+"/env", "--extract", "$.data"))
	mustPass(t, runFakeKeepEnv(t, dir, "validate", "--url", srv.URL+"/p", "--ignore", "$.note"))

	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--url", srv.URL + "/list", "--url", srv.URL + "/p", "--extract-each", "$.items"}, "--extract-each can only be used with a single --url"},
		{[]string{"--url", srv.URL + "/missing", "--extract-each", "$.items"}, "HTTP 404"},
		{[]string{"--url", srv.URL + "/p", "--watch"}, "--watch is not compatible with --url"},
		{[]string{"--url", srv.URL + "/p", "--since", "HEAD"}, "--since is not compatible with --url"},
	} {
		r := runFakeKeepEnv(t, dir, append([]string{"validate"}, tc.args...)...)
		if r.err == nil || !strings.Contains(r.err.Error(), tc.want) {
			t.Errorf("%q: want %q, got %v", tc.args, tc.want, r.err)
		}
	}
}

// stdin, JSON and XML: the temp file keeps the format, and the report says
// where the input came from.
func TestFakeValidate_StdinFormats(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		fails      bool
	}{
		{"json", errPatient, true},
		{"xml", `<Patient xmlns="http://hl7.org/fhir"><id value="p"/></Patient>`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			in := writeFile(t, dir, "in.txt", tc.body)
			f, err := os.Open(in) //nolint:gosec // test file
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = f.Close() }()
			old := os.Stdin
			os.Stdin = f
			defer func() { os.Stdin = old }()

			r := runFake(t, dir, "validate", "--no-color", "-")
			if (r.err != nil) != tc.fails {
				t.Fatalf("want failure %v, got %v\n%s", tc.fails, r.err, r.stdout)
			}
			if !strings.Contains(r.stdout, "▶ stdin") {
				t.Errorf("the report must name stdin:\n%s", r.stdout)
			}
		})
	}
	dir := t.TempDir()
	if r := runFake(t, dir, "validate", "--since", "HEAD", "-"); r.err == nil || !strings.Contains(r.err.Error(), "--since needs a file or directory argument") {
		t.Errorf("--since with stdin: %v", r.err)
	}
}
