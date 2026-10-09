package cmd

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/fhirlint/fhirlint/internal/cache"
	"github.com/fhirlint/fhirlint/internal/iglock"
	"github.com/fhirlint/fhirlint/internal/validator"
)

// fakeJAR writes a JAR whose manifest names version, or no version at all
// when version is empty.
func fakeJAR(t *testing.T, version string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "validator_cli.jar")
	f, err := os.Create(path) //nolint:gosec // path is under t.TempDir()
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("META-INF/MANIFEST.MF")
	if err != nil {
		t.Fatal(err)
	}
	manifest := "Manifest-Version: 1.0\r\n"
	if version != "" {
		manifest += "Class-Path: org.hl7.fhir.validation/" + version + "/org.hl7.fhir.validation-" + version + ".jar\r\n"
	}
	if _, err := w.Write([]byte(manifest)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// cachedValidator makes the cache claim version, as after a download of it.
func cachedValidator(t *testing.T, version string) {
	t.Helper()
	t.Setenv(cache.DirEnvVar, t.TempDir())
	p, err := cache.ValidatorVersionPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(version), 0o600); err != nil {
		t.Fatal(err)
	}
}

// The result cache key named the cached validator while --jar ran another,
// so a run with --jar 7.0.1 was served the 6.10.3 result (#450).
func TestCacheKeyValidator(t *testing.T) {
	cachedValidator(t, "6.10.3")
	jar701 := fakeJAR(t, "7.0.1")
	anonymous := fakeJAR(t, "")

	if got := cacheKeyValidator(validator.Options{JARPath: jar701}); got != "7.0.1" {
		t.Errorf("--jar: got %q, want 7.0.1", got)
	}
	if got := cacheKeyValidator(validator.Options{}); got != "6.10.3" {
		t.Errorf("cached: got %q, want 6.10.3", got)
	}
	if got := cacheKeyValidator(validator.Options{ValidatorVersion: "6.10.4"}); got != "6.10.4" {
		t.Errorf("pinned: got %q, want 6.10.4", got)
	}
	// A JAR that names no version must not share entries with the cached one,
	// nor with another unidentifiable JAR.
	got := cacheKeyValidator(validator.Options{JARPath: anonymous})
	if got == "6.10.3" || got == "" {
		t.Errorf("--jar without a version: got %q, want a key of its own", got)
	}
	if other := cacheKeyValidator(validator.Options{JARPath: fakeJAR(t, "")}); other == got {
		t.Errorf("two unidentifiable JARs share the key %q", got)
	}
}

// --lock recorded the cached validator while --jar ran another (#450).
func TestRunLockWrite_RecordsTheRunningValidator(t *testing.T) {
	cachedValidator(t, "6.10.3")
	t.Chdir(t.TempDir())

	running := validator.RunValidatorVersion(validator.Options{JARPath: fakeJAR(t, "7.0.1")})
	if err := runLockWrite(nil, running); err != nil {
		t.Fatal(err)
	}
	lf, err := iglock.Read(iglock.LockFileName)
	if err != nil {
		t.Fatal(err)
	}
	if lf.Validator != "7.0.1" {
		t.Errorf("lock records validator %q, want the --jar's 7.0.1", lf.Validator)
	}

	// And verifying against the cached version's lock now fails for the
	// --jar run instead of passing on the cache's answer.
	lf.Validator = "6.10.3"
	if err := iglock.Write(iglock.LockFileName, lf); err != nil {
		t.Fatal(err)
	}
	if err := runLockVerify(nil, running); err == nil {
		t.Error("lock for 6.10.3 verified against a 7.0.1 --jar run")
	}
}
