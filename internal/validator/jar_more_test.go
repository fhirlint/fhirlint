// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package validator

import (
	"archive/zip"
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/fhirlint/fhirlint/internal/cache"
)

// The corners of the JAR cache and download (#691).

func zipWith(t *testing.T, files map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return writeTempJAR(t, buf.Bytes())
}

// The version is read from the manifest's class path; anything else reads as
// unknown, never as a guess.
func TestVersionFromJARManifest(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
		want string
	}{
		{"versioned class path", zipWith(t, map[string]string{"META-INF/MANIFEST.MF": "Class-Path: org.hl7.fhir.validation/7.0.6/x.jar\r\n"}), "7.0.6"},
		{"no manifest", zipWith(t, map[string]string{"org/x.class": "x"}), ""},
		{"manifest without the version", zipWith(t, map[string]string{"META-INF/MANIFEST.MF": "Manifest-Version: 1.0\r\n"}), ""},
		{"not a zip", writeTempJAR(t, []byte("not a jar")), ""},
	} {
		if got := versionFromJARManifest(tc.path); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

// A path that can't be read is an error, not "not a JAR".
func TestIsValidJAR_UnreadablePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("reading a directory fails differently on Windows")
	}
	if _, err := isValidJAR(t.TempDir()); err == nil {
		t.Error("reading a directory must be an error")
	}
}

func TestRejectionReason(t *testing.T) {
	for err, want := range map[error]string{
		ErrSignatureMismatch:         ErrSignatureMismatch.Error(),
		ErrDigestMismatch:            ErrDigestMismatch.Error(),
		errors.New("something else"): "it failed verification",
	} {
		if got := rejectionReason(err); got != want {
			t.Errorf("rejectionReason(%v) = %q, want %q", err, got, want)
		}
	}
}

// A cached JAR that can't be read is downloaded again rather than reported as
// a validator failure (#316).
func TestEnsureJAR_UnreadableCacheIsReDownloaded(t *testing.T) {
	requireEnforcedFileModes(t)
	signer := testSigner(t)
	g := newFakeGitHub(t, "7.0.5", map[string]release{"7.0.5": signed(t, signer, "7.0.5")})
	path := cachedJAR(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, jarBytes(t, "7.0.5"), 0o000); err != nil {
		t.Fatal(err)
	}
	got, err := EnsureJAR("", "")
	if err != nil || got != path || g.downloads.Load() != 1 {
		t.Fatalf("an unreadable cache must be replaced by a fresh download: %q %v (%d downloads)", got, err, g.downloads.Load())
	}
}

// A side JAR that can't be fetched, or placed, fails the comparison.
func TestEnsureSideJAR_Failures(t *testing.T) {
	_ = newFakeGitHub(t, "7.0.5", map[string]release{})
	if _, err := EnsureSideJAR("7.0.9"); err == nil {
		t.Error("a release that isn't published must fail")
	}
	t.Setenv(cache.DirEnvVar, t.TempDir()) // a fresh cache for the next case
	dir, err := cache.Dir()
	if err != nil {
		t.Fatal(err)
	}
	// validators/ is a file, so the release directory can't be made.
	if err := os.WriteFile(filepath.Join(dir, "validators"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	// It fails on creating the directory, before any download is tried.
	var pe *fs.PathError
	if _, err := EnsureSideJAR("7.0.5"); !errors.As(err, &pe) || pe.Op != "mkdir" {
		t.Errorf("a release directory that can't be created must fail there, got %v", err)
	}
}
