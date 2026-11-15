// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The result cache's paths besides a plain hit and miss (#669). --cache is an
// optimisation: it must never fail a run, and never fail it quietly either.

// validatedInputs counts the input files of every validation run the fake
// validator was asked for.
func validatedInputs(t *testing.T, log string) int {
	t.Helper()
	if _, err := os.Stat(log); os.IsNotExist(err) {
		return 0
	}
	n := 0
	for _, c := range fakeJavaCalls(t, log) {
		for _, a := range c {
			if strings.HasSuffix(a, ".json") && !strings.Contains(a, "fhirlint-result") {
				n++
			}
		}
	}
	return n
}

func TestFakeCache_DefaultDirectoryUnderHome(t *testing.T) {
	prepareFake(t)
	dir := t.TempDir()
	writeFile(t, dir, "p.json", cleanPatient)
	log := filepath.Join(t.TempDir(), "calls.jsonl")
	t.Setenv(fakeJavaLogEnv, log)
	mustPass(t, runFakeKeepEnv(t, dir, "validate", "--cache", "p.json"))
	r := runFakeKeepEnv(t, dir, "validate", "--no-color", "--cache", "p.json")
	mustPass(t, r)
	if !strings.Contains(r.stdout, "cached") || validatedInputs(t, log) != 1 {
		t.Errorf("the second run must come from the default cache: %d inputs validated\n%s", validatedInputs(t, log), r.stdout)
	}
	if entries, _ := os.ReadDir(filepath.Join(os.Getenv("HOME"), ".fhirlint", "result-cache")); len(entries) == 0 {
		t.Error("the default cache is ~/.fhirlint/result-cache")
	}
}

// A cache-dir from fhirlint.yml inside the validated files is refused, with
// a warning, and the run goes on without it.
func TestFakeCache_ConfigDirInsideTheInputIsRefused(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "fhir/p.json", cleanPatient)
	writeFile(t, dir, "fhirlint.yml", "cache: true\ncache-dir: fhir/.cache\n")
	r := runFake(t, dir, "validate", "fhir")
	mustPass(t, r)
	if !strings.Contains(r.stderr, "--cache is set but") || !strings.Contains(r.stderr, "running without it") {
		t.Errorf("want a warning about the refused cache:\n%s", r.stderr)
	}
}

// An entry that can't be read is one warning and a fresh validation.
func TestFakeCache_UnreadableEntryWarns(t *testing.T) {
	prepareFake(t)
	dir, cacheDir := t.TempDir(), t.TempDir()
	writeFile(t, dir, "p.json", cleanPatient)
	mustPass(t, runFakeKeepEnv(t, dir, "validate", "--cache", "--cache-dir", cacheDir, "p.json"))
	entries, _ := filepath.Glob(filepath.Join(cacheDir, "*.json"))
	if len(entries) != 1 {
		t.Fatalf("want one entry, got %v", entries)
	}
	if err := os.WriteFile(entries[0], []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := runFakeKeepEnv(t, dir, "validate", "--no-color", "--cache", "--cache-dir", cacheDir, "p.json")
	mustPass(t, r)
	if !strings.Contains(r.stderr, "the cache could not be read") || strings.Contains(r.stdout, "cached") {
		t.Errorf("a corrupt entry must warn and revalidate:\n%s\n%s", r.stdout, r.stderr)
	}
}

// A cache that can't be written warns once, and the run passes.
func TestFakeCache_UnwritableDirectoryWarns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions are not enforced the same way on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root writes anywhere")
	}
	prepareFake(t)
	dir, cacheDir := t.TempDir(), t.TempDir()
	writeFile(t, dir, "a.json", cleanPatient)
	writeFile(t, dir, "b.json", cleanPatient)
	if err := os.Chmod(cacheDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(cacheDir, 0o700) })
	r := runFakeKeepEnv(t, dir, "validate", "--cache", "--cache-dir", cacheDir, "a.json", "b.json")
	mustPass(t, r)
	if n := strings.Count(r.stderr, "results could not be written"); n != 1 {
		t.Errorf("want one warning for the run, got %d:\n%s", n, r.stderr)
	}
}

// A crash is not a verdict on the file: it is never cached, so the next run
// validates it again while the good file comes from the cache.
func TestFakeCache_ACrashIsNotCached(t *testing.T) {
	prepareFake(t)
	dir, cacheDir := t.TempDir(), t.TempDir()
	writeFile(t, dir, "ok.json", cleanPatient)
	writeFile(t, dir, "crash.json", `{"resourceType":"Patient","note":"FAKE_CRASH"}`)
	log := filepath.Join(t.TempDir(), "calls.jsonl")
	t.Setenv(fakeJavaLogEnv, log)
	_ = runFakeKeepEnv(t, dir, "validate", "--cache", "--cache-dir", cacheDir, "ok.json", "crash.json")
	if entries, _ := filepath.Glob(filepath.Join(cacheDir, "*.json")); len(entries) != 1 {
		t.Errorf("only the good file may be cached, got %d entries", len(entries))
	}
	before := validatedInputs(t, log)
	_ = runFakeKeepEnv(t, dir, "validate", "--cache", "--cache-dir", cacheDir, "ok.json", "crash.json")
	if got := validatedInputs(t, log) - before; got != 1 {
		t.Errorf("the second run must validate only the crashed file again, validated %d", got)
	}
}

// The content of a .po file is in the key: editing it revalidates.
func TestFakeCache_POFileContentIsInTheKey(t *testing.T) {
	prepareFake(t)
	dir, cacheDir := t.TempDir(), t.TempDir()
	writeFile(t, dir, "p.json", cleanPatient)
	writeFile(t, dir, "msgs.po", "msgid \"a\"\nmsgstr \"b\"\n")
	log := filepath.Join(t.TempDir(), "calls.jsonl")
	t.Setenv(fakeJavaLogEnv, log)
	run := func() { mustPass(t, runFakeKeepEnv(t, dir, "validate", "--cache", "--cache-dir", cacheDir, "--po", "msgs.po", "p.json")) }
	run()
	run()
	if n := validatedInputs(t, log); n != 1 {
		t.Errorf("the same .po must hit the cache, validated %d", n)
	}
	writeFile(t, dir, "msgs.po", "msgid \"a\"\nmsgstr \"c\"\n")
	run()
	if n := validatedInputs(t, log); n != 2 {
		t.Errorf("an edited .po must revalidate, validated %d", n)
	}
	if r := runFakeKeepEnv(t, dir, "validate", "--cache", "--cache-dir", cacheDir, "--expansion-parameters", "missing.json", "p.json"); r.err == nil {
		t.Error("an expansion-parameters file that can't be fingerprinted must be an error")
	}
}
