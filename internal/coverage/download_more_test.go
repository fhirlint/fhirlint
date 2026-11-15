// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package coverage_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/coverage"
	"github.com/fhirlint/fhirlint/internal/registry"
)

// The corners of Fetch the main tests leave out (#631).

// packumentServer serves the archive and the packument body given verbatim.
func packumentServer(t *testing.T, name, version string, archive []byte, packument string) *coverage.Downloader {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + name:
			_, _ = w.Write([]byte(packument))
		case "/" + name + "/" + version:
			_, _ = w.Write(archive)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return &coverage.Downloader{Registries: []string{srv.URL}, HTTP: srv.Client()}
}

func TestFetch_UnusablePackumentWarnsAndInstalls(t *testing.T) {
	archive := makeTarGz(t, []tarEntry{{name: "package/StructureDefinition-p.json", body: sdBody}})
	for _, tc := range []struct{ name, packument, warning string }{
		{"not JSON", `<html>`, "parsing package metadata"},
		{"version not listed", `{"versions":{"0.9.0":{"dist":{"shasum":"abc"}}}}`, "no checksum published"},
		{"no shasum", `{"versions":{"1.0.0":{"dist":{}}}}`, "no checksum published"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := packumentServer(t, "demo.pkg", "1.0.0", archive, tc.packument)
			cache := t.TempDir()
			warnings, err := d.Fetch(context.Background(), cache, "demo.pkg", "1.0.0")
			if err != nil {
				t.Fatal(err)
			}
			if len(warnings) != 1 || !strings.Contains(warnings[0], tc.warning) || !strings.Contains(warnings[0], "installed unverified") {
				t.Errorf("want one warning about %q, got %v", tc.warning, warnings)
			}
			if _, err := os.Stat(coverage.PackageDir(cache, "demo.pkg", "1.0.0")); err != nil {
				t.Errorf("not installed: %v", err)
			}
		})
	}
}

// oversizedEntry is a gzipped tar whose only header declares more than the
// per-file limit. The payload is never written: the extractor must refuse on
// the header, before reading it.
func oversizedEntry(t *testing.T) []byte {
	t.Helper()
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	if err := tw.WriteHeader(&tar.Header{Name: "package/huge.json", Mode: 0o644, Size: 65 << 20, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	// No Close: it would complain about the missing payload. The header block
	// is already in the buffer.
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write(tarBuf.Bytes())
	_ = gz.Close()
	return buf.Bytes()
}

func TestFetch_ArchiveLimits(t *testing.T) {
	// One more entry than allowed. Symlinks, which are skipped, so the test
	// writes nothing to disk while the count still runs.
	entries := make([]tarEntry, 100_001)
	for i := range entries {
		entries[i] = tarEntry{name: "package/l", typeflag: tar.TypeSymlink, linkname: "x"}
	}
	for _, tc := range []struct {
		name    string
		archive []byte
		want    string
	}{
		{"too many entries", makeTarGz(t, entries), "more than 100000 entries"},
		{"an entry over the per-file limit", oversizedEntry(t), "per-file limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := registryServer(t, "demo.pkg", "1.0.0", tc.archive, sha1hex(tc.archive))
			cache := t.TempDir()
			_, err := d.Fetch(context.Background(), cache, "demo.pkg", "1.0.0")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
			if _, err := os.Stat(coverage.PackageDir(cache, "demo.pkg", "1.0.0")); !os.IsNotExist(err) {
				t.Errorf("a refused archive must leave nothing in the cache: %v", err)
			}
		})
	}
}

func TestFetch_RejectsBadEntryNames(t *testing.T) {
	for _, name := range []string{"/etc/passwd", `package\..\..\evil.json`} {
		archive := makeTarGz(t, []tarEntry{{name: name, body: "x"}})
		d := registryServer(t, "demo.pkg", "1.0.0", archive, sha1hex(archive))
		if _, err := d.Fetch(context.Background(), t.TempDir(), "demo.pkg", "1.0.0"); err == nil {
			t.Errorf("entry %q must be refused", name)
		}
	}
}

// Another process installed the same package while this one downloaded: the
// rename loses the race, and that is a success, not an error.
func TestFetch_LosingTheInstallRaceIsFine(t *testing.T) {
	archive := makeTarGz(t, []tarEntry{{name: "package/StructureDefinition-p.json", body: sdBody}})
	d := registryServer(t, "demo.pkg", "1.0.0", archive, sha1hex(archive))
	cache := t.TempDir()
	final := filepath.Join(cache, "demo.pkg#1.0.0")
	if err := os.MkdirAll(filepath.Join(final, "package"), 0o750); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(final, "package", "theirs.json")
	if err := os.WriteFile(marker, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Fetch(context.Background(), cache, "demo.pkg", "1.0.0"); err != nil {
		t.Fatalf("an install already in place must not fail the fetch: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("the other install must be left alone: %v", err)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(cache, ".fhirlint-download-*")); len(leftovers) != 0 {
		t.Errorf("the temporary directory must be removed: %v", leftovers)
	}
}

func TestNewDownloader_AsksTheValidatorsRegistries(t *testing.T) {
	d := coverage.NewDownloader()
	if !slices.Equal(d.Registries, registry.Default()) || d.HTTP == nil || d.HTTP.Timeout == 0 {
		t.Errorf("NewDownloader: %+v", d)
	}
	// A zero Downloader falls back to the same defaults. With the context
	// already cancelled it fails fast, without a request reaching anyone.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (&coverage.Downloader{}).Fetch(ctx, t.TempDir(), "demo.pkg", "1.0.0")
	if err == nil || !strings.Contains(err.Error(), "demo.pkg#1.0.0") {
		t.Errorf("a cancelled fetch must fail and name the package, got %v", err)
	}
}
