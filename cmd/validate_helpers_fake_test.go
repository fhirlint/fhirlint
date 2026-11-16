// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The helpers behind runValidate, by what reaches them (#689): a temp
// directory that can't be written, a validator that refuses to run, inputs
// --skip-non-fhir can't classify, and config that doesn't parse.

// Every input fhirlint rewrites before validation goes through a temp file.
// When there is nowhere to write one, the run fails and says why, rather than
// validating something else or nothing.
func TestFakeValidate_NoTempDirFailsTheRun(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "p.json", cleanPatient)
	writeFile(t, dir, "q.json", cleanPatient)
	writeFile(t, dir, "list.json", `{"items":[`+cleanPatient+`]}`)
	writeFile(t, dir, "w.json", `{"data":`+cleanPatient+`}`)
	writeFile(t, dir, "w.xml", `<wrap><Patient xmlns="http://hl7.org/fhir"><id value="p"/></Patient></wrap>`)
	writeFile(t, dir, "b.json", `{"resourceType":"Bundle","type":"collection","entry":[{"resource":`+cleanPatient+`}]}`)
	writeFile(t, dir, "a.ndjson", cleanPatient+"\n")
	for _, args := range [][]string{
		{"--extract-each", "$.items", "list.json"},
		{"--bundle-entries", "b.json", "p.json"},
		{"--extract", "$.data", "w.json"},
		{"--extract", "$.Patient", "w.xml"},
		{"--ignore", "id", "p.json"},
		{"a.ndjson"},
		{"a.ndjson", "p.json"},
	} {
		prepareFake(t)
		missing := filepath.Join(t.TempDir(), "gone")
		for _, v := range []string{"TMPDIR", "TMP", "TEMP"} {
			t.Setenv(v, missing)
		}
		r := runFakeKeepEnv(t, dir, append([]string{"validate"}, args...)...)
		if !errors.Is(r.err, fs.ErrNotExist) {
			t.Errorf("%q: want the missing temp dir as the error, got %v", args, r.err)
		}
	}

	// Over several files a file that can't be preprocessed is a finding of
	// its own, so the others are still validated and the run fails.
	prepareFake(t)
	for _, v := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(v, filepath.Join(t.TempDir(), "gone"))
	}
	r := runFakeKeepEnv(t, dir, "validate", "--no-color", "--ignore", "id", "p.json", "q.json")
	mustFail(t, r)
	if strings.Count(r.stdout, "creating temp file") != 2 {
		t.Errorf("want one finding per file that couldn't be preprocessed:\n%s", r.stdout)
	}
}

// A validator that fails to run fails fhirlint's run too, on every path that
// starts it: the cache, NDJSON among other files, and per-file option groups.
func TestFakeValidate_ValidatorFailureOnEveryPath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "p.json", cleanPatient)
	writeFile(t, dir, "q.json", cleanPatient)
	writeFile(t, dir, "a.ndjson", cleanPatient+"\n")
	for _, tc := range []struct {
		name, yaml string
		args       []string
	}{
		{"with --cache", "", []string{"--cache", "p.json"}},
		{"several files", "", []string{"p.json", "q.json"}},
		{"NDJSON", "", []string{"a.ndjson"}},
		{"NDJSON among files", "", []string{"a.ndjson", "p.json"}},
		{"a profile map group", "profile-map:\n  p.json: http://example.org/StructureDefinition/x\n", []string{"p.json", "q.json"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writeFile(t, dir, "fhirlint.yml", tc.yaml+"\n")
			prepareFake(t)
			t.Setenv("FAKEJAVA_REFUSE", "1")
			r := runFakeKeepEnv(t, dir, append([]string{"validate"}, tc.args...)...)
			if r.err == nil || errors.Is(r.err, errValidationFailed) {
				t.Errorf("a validator that didn't run is not a validation failure: %v", r.err)
			}
		})
	}
}

// --cache keys on the content of a --po file, so one that can't be read is
// an error, not a key that ignores it.
func TestFakeValidate_CacheNeedsReadablePOFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "p.json", cleanPatient)
	r := runFake(t, dir, "validate", "--cache", "--po", "missing.po", "p.json")
	if r.err == nil || !strings.Contains(r.err.Error(), "missing.po") {
		t.Errorf("got %v", r.err)
	}
}

// --skip-non-fhir drops only what is definitely not FHIR. JSON it can't read
// to the end goes to the validator, which reports it.
func TestFakeValidate_SkipNonFHIRKeepsMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"empty.json":     "",
		"badkey.json":    "{,}",
		"truncated.json": `{"resourceType":`,
	} {
		writeFile(t, dir, name, content)
	}
	writeFile(t, dir, "package.json", `{"name":"not-fhir"}`)
	writeFile(t, dir, "list.json", `[1,2]`)
	prepareFake(t)
	log := filepath.Join(t.TempDir(), "calls.jsonl")
	t.Setenv(fakeJavaLogEnv, log)
	_ = runFakeKeepEnv(t, dir, "validate", "--skip-non-fhir", "empty.json", "badkey.json", "truncated.json", "package.json", "list.json")
	got := map[string]bool{}
	for _, call := range fakeJavaCalls(t, log) {
		for _, a := range call {
			got[filepath.Base(a)] = true
		}
	}
	for _, want := range []string{"empty.json", "badkey.json", "truncated.json"} {
		if !got[want] {
			t.Errorf("%s must reach the validator", want)
		}
	}
	for _, skip := range []string{"package.json", "list.json"} {
		if got[skip] {
			t.Errorf("%s is not FHIR and must be skipped", skip)
		}
	}

	// Nothing left once the non-FHIR files are skipped: nothing runs.
	_ = os.Remove(log)
	mustPass(t, runFakeKeepEnv(t, dir, "validate", "--skip-non-fhir", "package.json", "list.json"))
	if _, err := os.Stat(log); err == nil {
		t.Error("the validator must not start for no files")
	}
}

// Only NDJSON among several arguments: every line is validated, and there is
// no empty run for the regular files.
func TestFakeValidate_OnlyNDJSONArguments(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.ndjson", cleanPatient+"\n")
	writeFile(t, dir, "b.ndjson", errPatient+"\n")
	r := runFake(t, dir, "validate", "--no-color", "a.ndjson", "b.ndjson")
	mustFail(t, r)
	if !strings.Contains(r.stdout, "a.ndjson:1") || !strings.Contains(r.stdout, "b.ndjson:1") {
		t.Errorf("both files' lines must be reported:\n%s", r.stdout)
	}
}

func TestFakeValidate_HelperConfigErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "p.json", cleanPatient)
	for _, tc := range []struct {
		name, yaml string
		args       []string
		want       string
	}{
		{"rules is not a list", "rules: just-a-string\n", nil, "rules config must be a list"},
		{"a rule is not a map", "rules:\n  - just-a-string\n", nil, "each rule must be a map"},
		{"lint config", "lint:\n  no-such-rule: on\n", nil, "lint config"},
		{"a lock file that isn't one", "", []string{"--lock"}, "reading fhirlint.lock"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writeFile(t, dir, "fhirlint.yml", tc.yaml+"\n")
			_ = os.Remove(filepath.Join(dir, "fhirlint.lock"))
			if tc.args != nil && tc.args[0] == "--lock" {
				writeFile(t, dir, "fhirlint.lock", "{not json")
			}
			r := runFake(t, dir, append(append([]string{"validate"}, tc.args...), "p.json")...)
			if r.err == nil || !strings.Contains(r.err.Error(), tc.want) {
				t.Errorf("want %q, got %v", tc.want, r.err)
			}
		})
	}
}
