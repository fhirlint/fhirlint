// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha1" //nolint:gosec // the registry publishes SHA-1; the test has to match it
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/coverage"
)

// coverage's download, output and package-reading paths (#690).

// covArchive is example.cov#1.0.0 as the registry serves it.
func covArchive(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range map[string]string{
		"package/package.json":                        `{"name":"example.cov","version":"1.0.0","fhirVersions":["4.0.1"]}`,
		"package/StructureDefinition-CovPatient.json": covProfileSD,
	} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// coverageRegistry serves example.cov#1.0.0 with the given packument; an empty
// packument answers 404 everywhere. It counts the archive downloads.
func coverageRegistry(t *testing.T, packument string, archive []byte) *int {
	t.Helper()
	downloads := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case packument == "":
			w.WriteHeader(http.StatusNotFound)
		case r.URL.Path == "/example.cov":
			_, _ = w.Write([]byte(packument))
		case r.URL.Path == "/example.cov/1.0.0":
			downloads++
			_, _ = w.Write(archive)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	orig := newCoverageDownloader
	newCoverageDownloader = func() *coverage.Downloader {
		return &coverage.Downloader{Registries: []string{srv.URL}, HTTP: srv.Client()}
	}
	t.Cleanup(func() { newCoverageDownloader = orig })
	return &downloads
}

func TestFakeCoverage_DownloadsAMissingPackageOnce(t *testing.T) {
	prepareFake(t)
	archive := covArchive(t)
	sum := sha1.Sum(archive) //nolint:gosec // see the import
	downloads := coverageRegistry(t, `{"versions":{"1.0.0":{"dist":{"shasum":"`+hex.EncodeToString(sum[:])+`"}}}}`, archive)
	dir := t.TempDir()
	writeFile(t, dir, "data/p1.json", covClaimed)

	r := runFakeKeepEnv(t, dir, "coverage", "--ig", "example.cov#1.0.0", "data")
	if r.err != nil {
		t.Fatalf("coverage after a download: %v\n%s", r.err, r.stderr)
	}
	if !strings.Contains(r.stderr, "Downloading IG package example.cov#1.0.0") || strings.Contains(r.stdout, "Downloading") {
		t.Errorf("the download notice goes to stderr, not into the report:\nstdout:\n%s\nstderr:\n%s", r.stdout, r.stderr)
	}
	if strings.Contains(r.stderr, "warning:") {
		t.Errorf("a matching checksum gives no warning:\n%s", r.stderr)
	}
	if !strings.Contains(r.stdout, "must-support:   2/3") {
		t.Errorf("the downloaded profile must be measured:\n%s", r.stdout)
	}

	r = runFakeKeepEnv(t, dir, "coverage", "--ig", "example.cov#1.0.0", "data")
	if r.err != nil || *downloads != 1 || strings.Contains(r.stderr, "Downloading") {
		t.Errorf("the second run reads the cache: err %v, %d download(s)\n%s", r.err, *downloads, r.stderr)
	}
}

func TestFakeCoverage_DownloadWithoutAChecksumWarns(t *testing.T) {
	prepareFake(t)
	coverageRegistry(t, `{"versions":{"1.0.0":{}}}`, covArchive(t))
	dir := t.TempDir()
	writeFile(t, dir, "data/p1.json", covClaimed)

	r := runFakeKeepEnv(t, dir, "coverage", "--ig", "example.cov#1.0.0", "data")
	if r.err != nil {
		t.Fatalf("an unverifiable checksum is a warning, not a failure: %v\n%s", r.err, r.stderr)
	}
	if !strings.Contains(r.stderr, "warning: could not verify the checksum of example.cov#1.0.0") {
		t.Errorf("the unverified install must be said:\n%s", r.stderr)
	}
}

func TestFakeCoverage_DownloadFailures(t *testing.T) {
	prepareFake(t)
	dir := t.TempDir()
	writeFile(t, dir, "data/p1.json", covClaimed)
	pkgDir := filepath.Join(os.Getenv("HOME"), ".fhir", "packages", "example.cov#1.0.0")

	coverageRegistry(t, `{"versions":{"1.0.0":{"dist":{"shasum":"0000"}}}}`, covArchive(t))
	r := runFakeKeepEnv(t, dir, "coverage", "--ig", "example.cov#1.0.0", "data")
	if r.err == nil || !strings.Contains(r.err.Error(), "checksum mismatch") {
		t.Errorf("a wrong checksum must fail: %v", r.err)
	}
	if _, err := os.Stat(pkgDir); err == nil { //nolint:gosec // a path under the test's temp dir
		t.Error("a package with a wrong checksum must not be installed")
	}

	coverageRegistry(t, "", nil)
	r = runFakeKeepEnv(t, dir, "coverage", "--ig", "example.cov#1.0.0", "data")
	if r.err == nil || !strings.Contains(r.err.Error(), "does not exist on any registry") {
		t.Errorf("a package no registry has must say so: %v", r.err)
	}
}

func TestFakeCoverage_PackagesItCannotUse(t *testing.T) {
	prepareFake(t)
	dir := t.TempDir()
	writeFile(t, dir, "data/p1.json", covClaimed)
	cache := filepath.Join(os.Getenv("HOME"), ".fhir", "packages")
	// A package without profiles, a package directory that is a file, and a
	// stray file in the cache root, which the supporting-package scan skips.
	writeFile(t, filepath.Join(cache, "empty.pkg#1.0.0", "package"), "package.json", `{"name":"empty.pkg","version":"1.0.0"}`)
	writeFile(t, filepath.Join(cache, "broken.pkg#1.0.0"), "package", "not a directory")
	writeFile(t, cache, "stray.txt", "x")

	r := runFakeKeepEnv(t, dir, "coverage", "--ig", "empty.pkg#1.0.0", "--offline", "data")
	if r.err == nil || !strings.Contains(r.err.Error(), "no profiles found in [empty.pkg#1.0.0]") {
		t.Errorf("a package with no profiles must say so: %v", r.err)
	}
	r = runFakeKeepEnv(t, dir, "coverage", "--ig", "broken.pkg#1.0.0", "--offline", "data")
	if r.err == nil || !strings.Contains(r.err.Error(), "reading package broken.pkg#1.0.0") {
		t.Errorf("an unreadable package must be an error: %v", r.err)
	}
}

func TestFakeCoverage_OutputAndFormat(t *testing.T) {
	prepareFake(t)
	cacheCoverageIG(t)
	dir := t.TempDir()
	writeFile(t, dir, "data/p1.json", covClaimed)

	r := runFakeKeepEnv(t, dir, "coverage", "--ig", "example.cov#1.0.0", "--format", "json", "--output", "cov.json", "data")
	if r.err != nil || strings.Contains(r.stdout, "mustSupport") {
		t.Fatalf("--output writes the report to the file, not stdout: %v\n%s", r.err, r.stdout)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "cov.json")); err != nil || !strings.Contains(string(data), coverageProfileURL) { //nolint:gosec // a path under the test's temp dir
		t.Errorf("the report file must hold the JSON report: %v\n%s", err, data)
	}

	r = runFakeKeepEnv(t, dir, "coverage", "--ig", "example.cov#1.0.0", "--output", filepath.Join("missing", "cov.txt"), "data")
	if r.err == nil {
		t.Error("an unwritable --output must be an error")
	}
	r = runFakeKeepEnv(t, dir, "coverage", "--ig", "example.cov#1.0.0", "--format", "xml", "data")
	if r.err == nil || !strings.Contains(r.err.Error(), `unknown format "xml"`) {
		t.Errorf("an unknown format must be an error: %v", r.err)
	}
}
