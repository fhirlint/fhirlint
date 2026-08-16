package coverage_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha1" //nolint:gosec // asserting against the registry's published digest format
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/coverage"
)

type tarEntry struct {
	name     string
	body     string
	typeflag byte
	linkname string
}

func makeTarGz(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	for _, e := range entries {
		typeflag := e.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		hdr := &tar.Header{
			Name:     e.name,
			Mode:     0o644,
			Size:     int64(len(e.body)),
			Typeflag: typeflag,
			Linkname: e.linkname,
		}
		if typeflag != tar.TypeReg {
			hdr.Size = 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
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

func sha1hex(b []byte) string {
	sum := sha1.Sum(b) //nolint:gosec // matching the registry's published digest
	return hex.EncodeToString(sum[:])
}

// registryServer serves a packument at /<name> and the archive at
// /<name>/<version>, mirroring packages.fhir.org.
func registryServer(t *testing.T, name, version string, archive []byte, shasum string) *coverage.Downloader {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + name:
			if shasum == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"versions":{"` + version + `":{"dist":{"shasum":"` + shasum + `"}}}}`))
		case "/" + name + "/" + version:
			_, _ = w.Write(archive)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return &coverage.Downloader{Registry: srv.URL, HTTP: srv.Client()}
}

const sdBody = `{"resourceType":"StructureDefinition","url":"https://example.org/p","type":"Patient","derivation":"constraint"}`

func TestFetchInstallsPackage(t *testing.T) {
	archive := makeTarGz(t, []tarEntry{
		{name: "package/", typeflag: tar.TypeDir},
		{name: "package/package.json", body: `{"name":"demo.pkg","version":"1.0.0"}`},
		{name: "package/StructureDefinition-p.json", body: sdBody},
	})
	d := registryServer(t, "demo.pkg", "1.0.0", archive, sha1hex(archive))
	cache := t.TempDir()

	warnings, err := d.Fetch(context.Background(), cache, "demo.pkg", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}

	// The layout has to be the one PackageDir expects, or the download would
	// succeed and the profiles still not be found.
	sd := filepath.Join(coverage.PackageDir(cache, "demo.pkg", "1.0.0"), "StructureDefinition-p.json")
	if _, err := os.Stat(sd); err != nil {
		t.Fatalf("expected the profile at %s: %v", sd, err)
	}

	reg := coverage.NewRegistry()
	if n, err := reg.LoadPackage(coverage.PackageDir(cache, "demo.pkg", "1.0.0"), "demo.pkg#1.0.0"); err != nil || n != 1 {
		t.Errorf("loading the installed package: n=%d err=%v", n, err)
	}
}

func TestFetchRejectsChecksumMismatch(t *testing.T) {
	archive := makeTarGz(t, []tarEntry{{name: "package/package.json", body: "{}"}})
	d := registryServer(t, "demo.pkg", "1.0.0", archive, sha1hex([]byte("something else")))
	cache := t.TempDir()

	_, err := d.Fetch(context.Background(), cache, "demo.pkg", "1.0.0")
	if err == nil {
		t.Fatal("want an error for a checksum mismatch")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("error should name the mismatch, got: %v", err)
	}
	// A failed verification must leave nothing behind that a later run would
	// mistake for a cached package.
	assertCacheEmpty(t, cache)
}

func TestFetchWarnsWhenChecksumUnavailable(t *testing.T) {
	archive := makeTarGz(t, []tarEntry{
		{name: "package/StructureDefinition-p.json", body: sdBody},
	})
	// Empty shasum makes the packument 404, i.e. no published checksum.
	d := registryServer(t, "demo.pkg", "1.0.0", archive, "")
	cache := t.TempDir()

	warnings, err := d.Fetch(context.Background(), cache, "demo.pkg", "1.0.0")
	if err != nil {
		t.Fatalf("an unobtainable checksum must not be fatal: %v", err)
	}
	// It must never be silent, though (#260).
	if len(warnings) != 1 || !strings.Contains(warnings[0], "checksum") {
		t.Errorf("want a checksum warning, got %v", warnings)
	}
	if _, err := os.Stat(coverage.PackageDir(cache, "demo.pkg", "1.0.0")); err != nil {
		t.Errorf("the package should still be installed: %v", err)
	}
}

func TestFetchRejectsPathTraversal(t *testing.T) {
	archive := makeTarGz(t, []tarEntry{
		{name: "package/ok.json", body: "{}"},
		{name: "../../escaped.json", body: "owned"},
	})
	d := registryServer(t, "demo.pkg", "1.0.0", archive, sha1hex(archive))
	cache := t.TempDir()
	outside := filepath.Dir(cache)

	_, err := d.Fetch(context.Background(), cache, "demo.pkg", "1.0.0")
	if err == nil {
		t.Fatal("want an error for an entry escaping the destination")
	}

	if _, statErr := os.Stat(filepath.Join(outside, "escaped.json")); statErr == nil {
		t.Fatal("the traversal entry was written outside the cache")
	}
	assertCacheEmpty(t, cache)
}

func TestFetchSkipsLinks(t *testing.T) {
	archive := makeTarGz(t, []tarEntry{
		{name: "package/StructureDefinition-p.json", body: sdBody},
		{name: "package/evil", typeflag: tar.TypeSymlink, linkname: "/etc/passwd"},
		{name: "package/evil-hard", typeflag: tar.TypeLink, linkname: "package/StructureDefinition-p.json"},
	})
	d := registryServer(t, "demo.pkg", "1.0.0", archive, sha1hex(archive))
	cache := t.TempDir()

	if _, err := d.Fetch(context.Background(), cache, "demo.pkg", "1.0.0"); err != nil {
		t.Fatal(err)
	}

	// A link is how an archive gets a later write to land somewhere else, and a
	// FHIR package has no use for one. The rest of the package still installs.
	dir := coverage.PackageDir(cache, "demo.pkg", "1.0.0")
	for _, name := range []string{"evil", "evil-hard"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
			t.Errorf("%s should have been skipped", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "StructureDefinition-p.json")); err != nil {
		t.Errorf("the regular file should still be installed: %v", err)
	}
}

func TestFetchReportsMissingPackage(t *testing.T) {
	d := registryServer(t, "other.pkg", "1.0.0", nil, "")
	cache := t.TempDir()

	_, err := d.Fetch(context.Background(), cache, "demo.pkg", "9.9.9")
	if err == nil {
		t.Fatal("want an error for a package the registry does not have")
	}
	if !strings.Contains(err.Error(), "does not exist in the registry") {
		t.Errorf("error should say the package is unknown, got: %v", err)
	}
	assertCacheEmpty(t, cache)
}

func TestFetchRejectsNonArchive(t *testing.T) {
	body := []byte("<html>not a package</html>")
	d := registryServer(t, "demo.pkg", "1.0.0", body, sha1hex(body))
	cache := t.TempDir()

	_, err := d.Fetch(context.Background(), cache, "demo.pkg", "1.0.0")
	if err == nil {
		t.Fatal("want an error for a response that is not a gzip archive")
	}
	assertCacheEmpty(t, cache)
}

// assertCacheEmpty checks that no package directory was left in the cache.
// Failed installs must not leave anything a later run would read as cached.
func assertCacheEmpty(t *testing.T, cache string) {
	t.Helper()
	entries, err := os.ReadDir(cache)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") {
			t.Errorf("cache should be empty, found %q", e.Name())
		}
	}
}
