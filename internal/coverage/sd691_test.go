// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package coverage_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/coverage"
)

// Loading definitions from a package directory (#691).

func writePkg(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// Only StructureDefinitions with a URL and a type count; everything else a
// package holds is skipped, not an error. A URL already loaded keeps the
// first package's definition.
func TestLoadPackage_WhatCounts(t *testing.T) {
	first := writePkg(t, map[string]string{
		"StructureDefinition-a.json": `{"resourceType":"StructureDefinition","url":"https://ex.org/a","type":"Patient","derivation":"constraint",
		  "differential":{"element":[{"id":"Patient.name","path":"Patient.name","mustSupport":true}]}}`,
	})
	second := writePkg(t, map[string]string{
		"StructureDefinition-a.JSON":    `{"resourceType":"StructureDefinition","url":"https://ex.org/a","type":"Observation","derivation":"constraint"}`,
		"StructureDefinition-b.json":    `{"resourceType":"StructureDefinition","url":"https://ex.org/b","type":"Patient","derivation":"constraint"}`,
		"StructureDefinition-base.json": `{"resourceType":"StructureDefinition","url":"https://ex.org/base","type":"Patient","derivation":"specialization"}`,
		"no-url.json":                   `{"resourceType":"StructureDefinition","type":"Patient"}`,
		"no-type.json":                  `{"resourceType":"StructureDefinition","url":"https://ex.org/d"}`,
		"broken.json":                   `{"resourceType":"StructureDefinition",`,
		"bad-element.json":              `{"resourceType":"StructureDefinition","url":"https://ex.org/c","type":"Patient","differential":{"element":[{"id":1}]}}`,
		"Patient-example.json":          `{"resourceType":"Patient"}`,
		"notes.txt":                     `"StructureDefinition"`,
	})
	if err := os.Mkdir(filepath.Join(second, "sub.json"), 0o750); err != nil {
		t.Fatal(err)
	}
	r := coverage.NewRegistry()
	if n, err := r.LoadPackage(first, "first#1"); err != nil || n != 1 {
		t.Fatalf("first: %d, %v", n, err)
	}
	if n, err := r.LoadPackage(second, "second#1"); err != nil || n != 2 {
		t.Fatalf("second: want b and base, got %d, %v", n, err)
	}
	a, ok := r.Get("https://ex.org/a|1.0.0")
	if !ok || a.Type != "Patient" || a.Package != "first#1" {
		t.Fatalf("a versioned reference resolves to the first package's definition: %+v, %v", a, ok)
	}
	if _, ok := r.Get("https://ex.org/missing|1.0.0"); ok {
		t.Error("an unknown URL stays unknown with a version too")
	}
	// Without a snapshot the differential is what is measured.
	if els := a.Elements(); len(els) != 1 || els[0].ID != "Patient.name" {
		t.Errorf("Elements() = %+v", els)
	}
	var urls []string
	for _, sd := range r.ProfilesFrom("second#1") {
		urls = append(urls, sd.URL)
	}
	if strings.Join(urls, ",") != "https://ex.org/b" {
		t.Errorf("ProfilesFrom(second) = %v; base definitions and other packages are left out", urls)
	}
	if _, err := r.LoadPackage(filepath.Join(second, "nope"), "x"); err == nil {
		t.Error("a missing package directory is an error")
	}
}

func TestErrPackageNotCached_SaysWhatToDo(t *testing.T) {
	msg := (&coverage.ErrPackageNotCached{ID: "demo.pkg#1.0.0", Dir: "/c"}).Error()
	for _, want := range []string{"demo.pkg#1.0.0", "/c", "--offline"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %q: %s", want, msg)
		}
	}
}
