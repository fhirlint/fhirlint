// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The error and output paths of compare, stats and qualify (#690).

func TestFakeCompare_JSONToAFile(t *testing.T) {
	dir := t.TempDir()
	profileFile(t, dir, "a.json", "http://example.org/A")
	profileFile(t, dir, "b.json", "http://example.org/B")

	r := runFake(t, dir, "compare", "--left", "a.json", "--right", "b.json", "--format", "json", "--output", "cmp.json")
	if code := compareExit(t, r); code != 1 {
		t.Fatalf("differences still exit 1 with --output, got %d\n%s", code, r.stderr)
	}
	if !strings.Contains(r.stdout, "JSON comparison written to cmp.json") || strings.Contains(r.stdout, "Patient.name") {
		t.Errorf("--output writes the JSON to the file and says where:\n%s", r.stdout)
	}
	var res struct{ Messages []json.RawMessage }
	data, err := os.ReadFile(filepath.Join(dir, "cmp.json")) //nolint:gosec // a path under the test's temp dir
	if err != nil || json.Unmarshal(data, &res) != nil || len(res.Messages) != 1 {
		t.Errorf("the file must hold the result: %v\n%s", err, data)
	}

	r = runFakeKeepEnv(t, dir, "compare", "--left", "a.json", "--right", "b.json", "--format", "json", "--output", filepath.Join("missing", "cmp.json"))
	if code := compareExit(t, r); code != 2 || !strings.Contains(r.err.Error(), "writing ") {
		t.Errorf("an unwritable --output is exit 2, got %d %v", code, r.err)
	}
}

func TestFakeCompare_ProfileFilesItCannotRead(t *testing.T) {
	dir := t.TempDir()
	profileFile(t, dir, "a.json", "http://example.org/A")
	writeFile(t, dir, "notjson.xml", `<StructureDefinition/>`)
	writeFile(t, dir, "nourl.json", `{"resourceType":"StructureDefinition","name":"P"}`)
	for _, tc := range []struct{ file, want string }{
		{"notjson.xml", "not a JSON StructureDefinition"},
		{"nourl.json", "has no url"},
	} {
		r := runFake(t, dir, "compare", "--left", tc.file, "--right", "a.json")
		if code := compareExit(t, r); code != 2 || !strings.Contains(r.err.Error(), tc.want) || !strings.Contains(r.err.Error(), "left profile") {
			t.Errorf("%s: want exit 2 with %q, got %d %v", tc.file, tc.want, code, r.err)
		}
	}
	// --left-profile names the canonical, so the file need not carry one.
	r := runFake(t, dir, "compare", "--left", "nourl.json", "--left-profile", "http://example.org/A", "--right", "a.json")
	if code := compareExit(t, r); code != 0 {
		t.Errorf("an explicit canonical overrides the file's, got %d %v\n%s", code, r.err, r.stdout)
	}
}

func TestCompareSeverityMarker(t *testing.T) {
	for sev, want := range map[string]string{"error": "✗", "warning": "⚠", "information": "~", "": "~"} {
		if got := compareSeverityMarker(sev); got != want {
			t.Errorf("%q: got %q, want %q", sev, got, want)
		}
	}
}

func TestFakeStats_InputsItCannotCount(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "data/p1.json", cleanPatient)
	writeFile(t, dir, "data/w.json", warnPatient)
	writeFile(t, dir, "data/map.fml", "map \"http://example.org/m\" = m")

	r := runFake(t, dir, "stats", "data")
	mustPass(t, r)
	if !strings.Contains(r.stderr, "note: 1 FHIR Mapping Language file(s) not counted") {
		t.Errorf("a format fhirlint cannot read must be said, not counted as zero:\n%s", r.stderr)
	}
	if !strings.Contains(r.stdout, "Warnings  1") {
		t.Errorf("the validation summary counts the warning:\n%s", r.stdout)
	}

	// A file whose resourceType cannot be read is counted, visibly, as unknown.
	writeFile(t, dir, "bad/broken.json", `{"resourceType":`)
	r = runFakeKeepEnv(t, dir, "stats", "--no-validate", "bad")
	if r.err != nil || !strings.Contains(r.stdout, "(unknown)  1") {
		t.Errorf("a broken file is an unknown resource, not dropped: %v\n%s", r.err, r.stdout)
	}
	// A file that cannot be read at all fails the count.
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		locked := writeFile(t, dir, "locked/p.json", cleanPatient)
		if err := os.Chmod(locked, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(locked, 0o600) })
		r = runFakeKeepEnv(t, dir, "stats", "--no-validate", "locked")
		if r.err == nil || !strings.Contains(r.err.Error(), "reading ") {
			t.Errorf("an unreadable file must be an error: %v", r.err)
		}
	}
	if r := runFakeKeepEnv(t, dir, "stats", "--no-validate", "nowhere"); r.err == nil {
		t.Error("a path that does not exist must be an error")
	}

	// An unreadable .fhirlintignore is a warning; the count goes on.
	if err := os.Mkdir(filepath.Join(dir, ".fhirlintignore"), 0o700); err != nil {
		t.Fatal(err)
	}
	r = runFakeKeepEnv(t, dir, "stats", "--no-validate", "data")
	mustPass(t, r)
	if !strings.Contains(r.stderr, "warning: reading .fhirlintignore") {
		t.Errorf("the unreadable ignore file must be reported:\n%s", r.stderr)
	}
}

func TestFakeStats_OutputAndFailures(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "data/p1.json", cleanPatient)

	r := runFake(t, dir, "stats", "--no-validate", "--format", "json", "--output", "s.json", "data")
	mustPass(t, r)
	if data, err := os.ReadFile(filepath.Join(dir, "s.json")); err != nil || !strings.Contains(string(data), "Patient") { //nolint:gosec // a path under the test's temp dir
		t.Errorf("--output must hold the JSON: %v\n%s", err, data)
	}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"stats", "--no-validate", "--format", "json", "--output", filepath.Join("missing", "s.json"), "data"}, "json output"},
		{[]string{"stats", "--no-validate", "--format", "xml", "data"}, `unknown format "xml"`},
	} {
		r := runFakeKeepEnv(t, dir, tc.args...)
		var ee *exitErr
		if r.err == nil || !strings.Contains(r.err.Error(), tc.want) || !errors.As(r.err, &ee) || ee.code != 2 {
			t.Errorf("%q: want exit 2 with %q, got %v", tc.args, tc.want, r.err)
		}
	}

	t.Setenv("FAKEJAVA_REFUSE", "1")
	r = runFakeKeepEnv(t, dir, "stats", "data")
	if r.err == nil || !strings.Contains(r.err.Error(), "validation summary") {
		t.Errorf("a validator that fails must fail stats, not report zero errors: %v\n%s", r.err, r.stdout)
	}
}

func TestFakeQualify_OutputFormats(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "suite/ok.json", cleanPatient)
	writeFile(t, dir, "suite/ok.expected.json", `{"valid": true}`)

	// Two formats into one --output get an extension each.
	r := runFake(t, dir, "qualify", "--test-suite", "suite", "--format", "json", "--format", "html", "--output", "q")
	if r.err == nil {
		t.Fatalf("the fake does not qualify, so this still fails:\n%s", r.stdout)
	}
	for _, f := range []string{"q.json", "q.html"} {
		if info, err := os.Stat(filepath.Join(dir, f)); err != nil || info.Size() == 0 {
			t.Errorf("%s must be written: %v", f, err)
		}
	}
	data, _ := os.ReadFile(filepath.Join(dir, "q.json")) //nolint:gosec // a path under the test's temp dir
	var rep struct {
		JARSHA256   string `json:"jarSha256"`
		Terminology string `json:"terminology"`
	}
	if err := json.Unmarshal(data, &rep); err != nil || len(rep.JARSHA256) != 64 || rep.Terminology != "offline" {
		t.Errorf("the report records the JAR digest and the terminology: %v %+v\n%s", err, rep, data)
	}

	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"qualify", "--format", "pdf"}, `unknown format "pdf"`},
		{[]string{"qualify", "--format", "json", "--output", filepath.Join("missing", "q.json")}, "json report"},
		{[]string{"qualify", "--format", "html", "--output", filepath.Join("missing", "q.html")}, "html report"},
		{[]string{"qualify", "--test-suite", "nowhere"}, "loading test suite"},
	} {
		r := runFakeKeepEnv(t, dir, tc.args...)
		var ee *exitErr
		if r.err == nil || !strings.Contains(r.err.Error(), tc.want) || !errors.As(r.err, &ee) || ee.code != 2 {
			t.Errorf("%q: want exit 2 with %q, got %v", tc.args, tc.want, r.err)
		}
	}
}
