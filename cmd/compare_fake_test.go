// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// fhirlint compare end to end against the fake validator's compare mode
// (#615).

func profileFile(t *testing.T, dir, name, url string) string {
	t.Helper()
	return writeFile(t, dir, name, `{"resourceType":"StructureDefinition","url":"`+url+`","name":"P","type":"Patient"}`)
}

func compareExit(t *testing.T, r fakeRun) int {
	t.Helper()
	switch {
	case r.err == nil:
		return 0
	case errors.Is(r.err, errValidationFailed):
		return 1
	}
	var ee *exitErr
	if errors.As(r.err, &ee) {
		return ee.code
	}
	t.Fatalf("compare failed without an exit code: %v", r.err)
	return -1
}

func TestFakeCompare_DifferencesFailAndAreListed(t *testing.T) {
	dir := t.TempDir()
	profileFile(t, dir, "a.json", "http://example.org/A")
	profileFile(t, dir, "b.json", "http://example.org/B")

	r := runFake(t, dir, "compare", "--left", "a.json", "--right", "b.json")
	if code := compareExit(t, r); code != 1 {
		t.Fatalf("differing profiles exit 1, got %d\n%s\n%s", code, r.stdout, r.stderr)
	}
	if !strings.Contains(r.stdout, "Patient.name") || !strings.Contains(r.stdout, "Fake difference between http://example.org/A and http://example.org/B") {
		t.Errorf("the difference must be listed:\n%s", r.stdout)
	}

	r = runFakeKeepEnv(t, dir, "compare", "--left", "a.json", "--right", "b.json", "--format", "json")
	var res struct {
		Left, Right string
		Messages    []struct{ Severity, Path, Message string }
	}
	if err := json.Unmarshal([]byte(r.stdout), &res); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, r.stdout)
	}
	if res.Left != "http://example.org/A" || len(res.Messages) != 1 || res.Messages[0].Severity != "error" {
		t.Errorf("JSON result: %+v", res)
	}
}

func TestFakeCompare_SameProfileIsEquivalent(t *testing.T) {
	dir := t.TempDir()
	profileFile(t, dir, "a.json", "http://example.org/A")
	r := runFake(t, dir, "compare", "--left", "a.json", "--right", "a.json")
	if code := compareExit(t, r); code != 0 {
		t.Fatalf("a profile against itself exits 0, got %d\n%s", code, r.stdout)
	}
	r = runFakeKeepEnv(t, dir, "compare", "--left", "a.json", "--right", "a.json", "--format", "html")
	if !strings.Contains(r.stdout, "Comparison report written to") || !strings.Contains(r.stdout, ".html") {
		t.Errorf("--format html names the report:\n%s", r.stdout)
	}
}

func TestFakeCompare_Errors(t *testing.T) {
	dir := t.TempDir()
	profileFile(t, dir, "a.json", "http://example.org/A")
	writeFile(t, dir, "obs.json", `{"resourceType":"Observation"}`)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"compare", "--left", "a.json", "--right", "a.json", "--format", "pdf"}, "unknown format"},
		{[]string{"compare", "--left", "a.json"}, "both --left and --right are required"},
		{[]string{"compare", "--left", "obs.json", "--right", "a.json"}, "expected a StructureDefinition"},
		{[]string{"compare", "--left", "kbv.basis#1.5.0", "--right", "a.json"}, "also pass --left-profile"},
		{[]string{"compare", "--left", "mii", "--left-profile", "X", "--right", "a.json"}, "stands for several packages"},
		{[]string{"compare", "--left", "kbv.basis#1.5.0", "--left-profile", "unresolvable", "--right", "a.json"}, "Unable to resolve"},
	} {
		r := runFake(t, dir, tc.args...)
		if code := compareExit(t, r); code != 2 || !strings.Contains(r.err.Error(), tc.want) {
			t.Errorf("%q: want exit 2 with %q, got %d %v", tc.args, tc.want, code, r.err)
		}
	}
}
