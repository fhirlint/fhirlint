// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package qualify

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
)

// A directory already sitting where a built-in case is written stops the
// extraction rather than running the suite without that case (#704).
func TestBuiltinCases_CaseFileCannotBeWritten(t *testing.T) {
	all, err := BuiltinCases(t.TempDir())
	if err != nil || len(all) == 0 {
		t.Fatalf("BuiltinCases: %v (%d cases)", err, len(all))
	}
	dest := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dest, filepath.FromSlash(all[0].Name)), 0o750); err != nil {
		t.Fatal(err)
	}
	if cases, err := BuiltinCases(dest); err == nil {
		t.Errorf("want an error, got %d cases", len(cases))
	}
}

// A sub-directory of the suite that cannot be read fails the load: a suite run
// on a part of the cases would pass on what it never saw (#704).
func TestLoadSuite_UnreadableSubdirectory(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX permissions and a non-root user")
	}
	dir := t.TempDir()
	for name, body := range map[string]string{
		"a.json":              `{"resourceType":"Patient"}`,
		"a.expected.json":     `{"valid":true}`,
		"sub/b.json":          `{"resourceType":"Patient"}`,
		"sub/b.expected.json": `{"valid":true}`,
	} {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Chmod(sub, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sub, 0o750) }) //nolint:gosec // restoring the test's own directory so TempDir can remove it
	if cases, err := LoadSuite(dir); err == nil || !strings.Contains(err.Error(), "sub") {
		t.Errorf("err = %v (%d cases), want the unreadable directory named", err, len(cases))
	}
}

// The expectation readers name what is missing (#704).
func TestReadExpected_Missing(t *testing.T) {
	if _, err := readExpectedFS(fstest.MapFS{}, "builtin/valid/x.expected.json"); err == nil || !strings.Contains(err.Error(), "missing expectation file builtin/valid/x.expected.json") {
		t.Errorf("readExpectedFS: %v", err)
	}
	p := filepath.Join(t.TempDir(), "x.expected.json")
	if _, err := readExpectedOS(p); err == nil || !strings.Contains(err.Error(), "x.expected.json") {
		t.Errorf("readExpectedOS: %v", err)
	}
}
