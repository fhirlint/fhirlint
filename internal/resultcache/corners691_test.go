// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package resultcache

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The corners of the result cache (#691).

// A damaged entry is a miss with an error, never a result someone trusts.
func TestGet_CorruptEntryIsNotAHit(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(entryPath(dir, "k"), []byte(`{"result":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if e, err := Get(dir, "k"); err == nil || e != nil {
		t.Errorf("got %+v, %v", e, err)
	}
}

func TestPut_CacheDirBelowAFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Put(filepath.Join(file, "cache"), "k", Entry{}); err == nil {
		t.Error("a cache dir that can't be created must fail")
	}
	// CheckDir reports such a path instead of reading it as "not there yet".
	if err := CheckDir(filepath.Join(file, "cache")); err == nil {
		t.Error("CheckDir must report a path it can't look at")
	}
	if err := CheckDir(file); err == nil || !strings.Contains(err.Error(), "is not a directory") {
		t.Errorf("CheckDir on a file: %v", err)
	}
}

// A key can't be made from something that isn't readable as a file.
func TestKey_OfADirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("opening a directory fails earlier on Windows")
	}
	if _, err := Key(t.TempDir(), KeyOpts{}); err == nil {
		t.Error("a directory has no content to key")
	}
}

// Several entries that can't be removed are counted, in the plural.
func TestClear_SeveralUnremovableEntries(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX directory modes and a non-root user")
	}
	dir := t.TempDir()
	for _, k := range []string{"a", "b"} {
		if err := Put(dir, k, Entry{}); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(dir, 0o500); err != nil { //nolint:gosec // a read-only directory needs the execute bit
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) //nolint:gosec // restoring a temp directory, which needs the execute bit
	if _, err := Clear(dir); err == nil || !strings.Contains(err.Error(), "2 cache entries could not be removed") {
		t.Errorf("got %v", err)
	}
}
