// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package coverage_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/coverage"
)

// The remaining corners of unpacking a package (#691).

// Directory entries are created, also an empty one, and files below them
// land where they say.
func TestFetch_DirectoryEntries(t *testing.T) {
	archive := makeTarGz(t, []tarEntry{
		{name: "package/", typeflag: tar.TypeDir},
		{name: "package/examples/", typeflag: tar.TypeDir},
		{name: "package/other/", typeflag: tar.TypeDir}, // stays empty
		{name: "package/examples/Patient-a.json", body: `{"resourceType":"Patient"}`},
	})
	d := registryServer(t, "demo.pkg", "1.0.0", archive, sha1hex(archive))
	cache := t.TempDir()
	if _, err := d.Fetch(context.Background(), cache, "demo.pkg", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(coverage.PackageDir(cache, "demo.pkg", "1.0.0"), "examples", "Patient-a.json")); err != nil {
		t.Errorf("the nested file must be installed: %v", err)
	}
	if fi, err := os.Stat(filepath.Join(coverage.PackageDir(cache, "demo.pkg", "1.0.0"), "other")); err != nil || !fi.IsDir() {
		t.Errorf("an empty directory entry must be created: %v", err)
	}
}

func truncatedTarGz(t *testing.T) []byte {
	t.Helper()
	full := makeTarGz(t, []tarEntry{{name: "package/StructureDefinition-p.json", body: strings.Repeat("x", 4096)}})
	zr, err := gzip.NewReader(bytes.NewReader(full))
	if err != nil {
		t.Fatal(err)
	}
	var raw bytes.Buffer
	_, _ = raw.ReadFrom(zr)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write(raw.Bytes()[:700]) // the header and part of the payload
	_ = gz.Close()
	return buf.Bytes()
}

// Archives that can't be unpacked fail the fetch and leave no package behind,
// so the next run doesn't take a half-written one for a cached one.
func TestFetch_ArchivesThatCannotBeUnpacked(t *testing.T) {
	for _, tc := range []struct {
		name    string
		archive []byte
	}{
		{"a truncated tar", truncatedTarGz(t)},
		{"a file where a directory is needed", makeTarGz(t, []tarEntry{
			{name: "package/x", body: "file"},
			{name: "package/x/y.json", body: "{}"},
		})},
		{"an entry with an empty name", makeTarGz(t, []tarEntry{{name: "", body: "x"}})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := registryServer(t, "demo.pkg", "1.0.0", tc.archive, sha1hex(tc.archive))
			cache := t.TempDir()
			_, err := d.Fetch(context.Background(), cache, "demo.pkg", "1.0.0")
			if err == nil || !strings.Contains(err.Error(), "unpacking demo.pkg#1.0.0") {
				t.Fatalf("got %v", err)
			}
			if _, err := os.Stat(coverage.PackageDir(cache, "demo.pkg", "1.0.0")); !os.IsNotExist(err) {
				t.Errorf("nothing may be left in the cache: %v", err)
			}
		})
	}
}

// A cache root that can't be created fails the fetch.
func TestFetch_CacheRootIsAFile(t *testing.T) {
	archive := makeTarGz(t, []tarEntry{{name: "package/StructureDefinition-p.json", body: sdBody}})
	d := registryServer(t, "demo.pkg", "1.0.0", archive, sha1hex(archive))
	root := filepath.Join(t.TempDir(), "cache")
	if err := os.WriteFile(root, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Fetch(context.Background(), filepath.Join(root, "packages"), "demo.pkg", "1.0.0"); err == nil {
		t.Error("a cache below a file must fail")
	}
}
