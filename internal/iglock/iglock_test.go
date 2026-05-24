package iglock_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/iglock"
)

func TestParseIGID(t *testing.T) {
	cases := []struct {
		ig      string
		name    string
		version string
	}{
		{"kbv.basis#1.5.0", "kbv.basis", "1.5.0"},
		{"hl7.fhir.r4.core#4.0.1", "hl7.fhir.r4.core", "4.0.1"},
		{"kbv.basis", "", ""},
		{"kbv.basis#latest", "", ""},
		{"#1.0.0", "", ""},
		{"kbv.basis#", "", ""},
		{"/local/path#1.0.0", "", ""},
		{"", "", ""},
	}
	for _, tc := range cases {
		n, v := iglock.ParseIGID(tc.ig)
		if n != tc.name || v != tc.version {
			t.Errorf("ParseIGID(%q) = (%q, %q), want (%q, %q)", tc.ig, n, v, tc.name, tc.version)
		}
	}
}

func TestPackageURL(t *testing.T) {
	url := iglock.PackageURL("kbv.basis", "1.5.0")
	if url != "https://packages.fhir.org/kbv.basis/1.5.0" {
		t.Errorf("unexpected URL: %q", url)
	}
}

func TestReadWrite_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fhirlint.lock")

	original := &iglock.LockFile{
		Packages: map[string]iglock.Entry{
			"kbv.basis#1.5.0": {
				URL:    "https://packages.fhir.org/kbv.basis/1.5.0",
				SHA256: "abc123",
			},
		},
	}

	if err := iglock.Write(path, original); err != nil {
		t.Fatalf("Write: %v", err)
	}

	read, err := iglock.Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(read.Packages) != 1 {
		t.Fatalf("expected 1 package, got %d", len(read.Packages))
	}
	e := read.Packages["kbv.basis#1.5.0"]
	if e.SHA256 != "abc123" || e.URL != "https://packages.fhir.org/kbv.basis/1.5.0" {
		t.Errorf("round-trip mismatch: %+v", e)
	}
}

func TestRead_ReturnsNilForMissingFile(t *testing.T) {
	lf, err := iglock.Read("/nonexistent/fhirlint.lock")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if lf != nil {
		t.Errorf("expected nil LockFile for missing file, got: %+v", lf)
	}
}

func TestRead_ReturnsErrorForInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fhirlint.lock")
	if err := os.WriteFile(path, []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := iglock.Read(path)
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestVerify_MatchingHash(t *testing.T) {
	dir := t.TempDir()

	pkgDir := filepath.Join(dir, ".fhir", "packages", "kbv.basis#1.5.0", "package")
	if err := os.MkdirAll(pkgDir, 0750); err != nil {
		t.Fatal(err)
	}
	manifest := `{"name":"kbv.basis","version":"1.5.0"}`
	manifestPath := filepath.Join(pkgDir, "package.json")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)

	hash, err := iglock.HashPackageFromPath(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	lf := &iglock.LockFile{
		Packages: map[string]iglock.Entry{
			"kbv.basis#1.5.0": {URL: "https://packages.fhir.org/kbv.basis/1.5.0", SHA256: hash},
		},
	}

	var w strings.Builder
	if err := iglock.Verify(lf, []string{"kbv.basis#1.5.0"}, &w); err != nil {
		t.Errorf("Verify should pass with matching hash, got: %v", err)
	}
}

func TestVerify_MismatchedHash(t *testing.T) {
	dir := t.TempDir()

	pkgDir := filepath.Join(dir, ".fhir", "packages", "kbv.basis#1.5.0", "package")
	if err := os.MkdirAll(pkgDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{"name":"kbv.basis"}`), 0600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", dir)

	lf := &iglock.LockFile{
		Packages: map[string]iglock.Entry{
			"kbv.basis#1.5.0": {SHA256: "wrong_hash"},
		},
	}

	var w strings.Builder
	if err := iglock.Verify(lf, []string{"kbv.basis#1.5.0"}, &w); err == nil {
		t.Error("Verify should fail on hash mismatch")
	}
}

func TestVerify_MissingPackageInCache_Skips(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	lf := &iglock.LockFile{
		Packages: map[string]iglock.Entry{
			"kbv.basis#1.5.0": {SHA256: "anyhash"},
		},
	}

	var w strings.Builder
	if err := iglock.Verify(lf, []string{"kbv.basis#1.5.0"}, &w); err != nil {
		t.Errorf("Verify should skip packages not in cache, got: %v", err)
	}
	if !strings.Contains(w.String(), "not in FHIR package cache") {
		t.Errorf("expected warning about missing package, got: %q", w.String())
	}
}

func TestVerify_IGNotInLockFile_Warns(t *testing.T) {
	lf := &iglock.LockFile{Packages: map[string]iglock.Entry{}}

	var w strings.Builder
	if err := iglock.Verify(lf, []string{"kbv.basis#1.5.0"}, &w); err != nil {
		t.Errorf("Verify should not fail for unknown IG, got: %v", err)
	}
	if !strings.Contains(w.String(), "not in lock file") {
		t.Errorf("expected warning about IG not in lock file, got: %q", w.String())
	}
}

func TestVerify_SkipsNonVersionedIGs(t *testing.T) {
	lf := &iglock.LockFile{Packages: map[string]iglock.Entry{}}
	var w strings.Builder
	err := iglock.Verify(lf, []string{"/local/path", "kbv.basis", "kbv.basis#latest"}, &w)
	if err != nil {
		t.Errorf("Verify should skip non-versioned IGs, got: %v", err)
	}
	if w.Len() != 0 {
		t.Errorf("expected no output for non-versioned IGs, got: %q", w.String())
	}
}

func TestUpdate_AddsEntries(t *testing.T) {
	dir := t.TempDir()

	pkgDir := filepath.Join(dir, ".fhir", "packages", "kbv.basis#1.5.0", "package")
	if err := os.MkdirAll(pkgDir, 0750); err != nil {
		t.Fatal(err)
	}
	manifest := `{"name":"kbv.basis","version":"1.5.0"}`
	if err := os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)

	lf := &iglock.LockFile{Packages: make(map[string]iglock.Entry)}
	n, err := iglock.Update(lf, []string{"kbv.basis#1.5.0", "/local/path", "kbv.basis"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 entry updated, got %d", n)
	}
	e, ok := lf.Packages["kbv.basis#1.5.0"]
	if !ok {
		t.Fatal("kbv.basis#1.5.0 not in lock file after update")
	}
	if e.SHA256 == "" {
		t.Error("SHA256 should not be empty")
	}
	if e.URL != "https://packages.fhir.org/kbv.basis/1.5.0" {
		t.Errorf("unexpected URL: %q", e.URL)
	}
}

func TestUpdate_ErrorForMissingPackage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	lf := &iglock.LockFile{Packages: make(map[string]iglock.Entry)}
	_, err := iglock.Update(lf, []string{"missing.package#1.0.0"})
	if err == nil {
		t.Error("expected error for package not in cache")
	}
}

