// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package qualify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// LoadSuite's failures (#634): a suite must say what is wrong with it rather
// than run an incomplete set.
func TestLoadSuite_Errors(t *testing.T) {
	if _, err := LoadSuite(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("a missing suite directory must be an error")
	}

	file := filepath.Join(t.TempDir(), "case.json")
	if err := os.WriteFile(file, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSuite(file); err == nil || !strings.Contains(err.Error(), "is not a directory") {
		t.Errorf("a file as the suite: %v", err)
	}

	// An expectation that isn't JSON fails the load and names the file.
	dir := t.TempDir()
	for name, body := range map[string]string{
		"a.json":          `{"resourceType":"Patient"}`,
		"a.expected.json": `{"valid": tru`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := LoadSuite(dir); err == nil || !strings.Contains(err.Error(), "a.expected.json") {
		t.Errorf("a broken expectation must be an error naming it: %v", err)
	}
}

// A case without an expectation is left out; the cases with one are kept,
// with the expectation as written.
func TestLoadSuite_SkipsCasesWithoutAnExpectation(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"sub/b.json":          `{"resourceType":"Patient"}`,
		"sub/b.expected.json": `{"description":"invalid on purpose","valid":false,"messageIds":["X"]}`,
		"notes.json":          `{"resourceType":"Patient"}`,
	} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cases, err := LoadSuite(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 1 || cases[0].Name != "sub/b.json" || cases[0].Expected.Valid || cases[0].Expected.MessageIDs[0] != "X" {
		t.Errorf("cases: %+v", cases)
	}
}

// BuiltinCases writes into a directory it must be able to create.
func TestBuiltinCases_UnwritableDestination(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := BuiltinCases(filepath.Join(blocker, "sub")); err == nil {
		t.Error("a destination under a file must be an error")
	}
}
