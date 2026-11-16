// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package iglock_test

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/iglock"
)

// The corners of the lock file (#691).

// A cached package whose manifest exists but can't be hashed is an error in
// every path that hashes, never a "not cached" warning: the lock would
// otherwise pass a package it never looked at.
func TestUnhashablePackageIsAnError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("reading a directory fails at open on Windows, which is the same outcome by another path")
	}
	home := t.TempDir()
	setFakeHome(t, home)
	// package.json is a directory: it exists, so it is not "not cached", and
	// reading it fails.
	if err := os.MkdirAll(filepath.Join(home, ".fhir", "packages", "demo.pkg#1.0.0", "package", "package.json"), 0o750); err != nil {
		t.Fatal(err)
	}
	const ig = "demo.pkg#1.0.0"
	entry := iglock.Entry{SHA256: "00"}
	if err := iglock.Verify(&iglock.LockFile{Packages: map[string]iglock.Entry{ig: entry}}, []string{ig}, io.Discard); err == nil || !strings.Contains(err.Error(), "hashing "+ig) {
		t.Errorf("Verify: %v", err)
	}
	if _, err := iglock.Update(&iglock.LockFile{Packages: map[string]iglock.Entry{}}, []string{ig}); err == nil || !strings.Contains(err.Error(), "hashing "+ig) {
		t.Errorf("Update: %v", err)
	}
	if err := iglock.VerifyImplicit(&iglock.LockFile{Implicit: map[string]iglock.Entry{ig: entry}}, []string{ig}, io.Discard); err == nil || !strings.Contains(err.Error(), "hashing "+ig) {
		t.Errorf("VerifyImplicit: %v", err)
	}
}

func TestRead_Corners(t *testing.T) {
	dir := t.TempDir()
	if _, err := iglock.Read(dir); err == nil {
		t.Error("reading a directory must fail")
	}
	bad := filepath.Join(dir, "bad.lock")
	if err := os.WriteFile(bad, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := iglock.Read(bad); err == nil || !strings.Contains(err.Error(), "parsing") {
		t.Errorf("a corrupt lock file: %v", err)
	}
	// A lock file without a packages section reads as an empty one that can be
	// added to, not as a nil map that panics on the first --lock.
	empty := filepath.Join(dir, "empty.lock")
	if err := os.WriteFile(empty, []byte(`{"validator":"7.0.6"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	lf, err := iglock.Read(empty)
	if err != nil {
		t.Fatal(err)
	}
	if lf.Packages == nil {
		t.Fatal("Packages must be an empty map")
	}
	lf.Packages["x#1.0.0"] = iglock.Entry{}
}
