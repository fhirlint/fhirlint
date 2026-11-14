// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package pkgindex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

const patientURL = "https://fhir.kbv.de/StructureDefinition/KBV_PR_Base_Patient"

// fakeCache writes packages with an .index.json each under a temp home.
func fakeCache(t *testing.T, pkgs map[string][]string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for id, urls := range pkgs {
		dir := filepath.Join(home, ".fhir", "packages", id, "package")
		if err := os.MkdirAll(dir, 0750); err != nil {
			t.Fatal(err)
		}
		_, version, _ := strings.Cut(id, "#")
		var files []map[string]string
		for _, u := range urls {
			files = append(files, map[string]string{"url": u, "version": version, "resourceType": "StructureDefinition"})
		}
		data, _ := json.Marshal(map[string]any{"index-version": 2, "files": files})
		if err := os.WriteFile(filepath.Join(dir, ".index.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLookup_PrefersHighestFinal(t *testing.T) {
	fakeCache(t, map[string][]string{
		"kbv.basis#1.9.0":         {patientURL},
		"kbv.basis#1.10.0":        {patientURL},
		"kbv.basis#1.11.0-ballot": {patientURL},
		"other#1.0.0":             {"http://other/SD/x"},
	})
	ix := Build()
	want := []string{"kbv.basis#1.10.0", "kbv.basis#1.9.0", "kbv.basis#1.11.0-ballot"}
	if got := ix.Lookup(patientURL); !reflect.DeepEqual(got, want) {
		t.Errorf("Lookup = %v, want %v", got, want)
	}
	// A pinned version that one package has: that one.
	if got := ix.Lookup(patientURL + "|1.9.0"); !reflect.DeepEqual(got, []string{"kbv.basis#1.9.0"}) {
		t.Errorf("versioned Lookup = %v", got)
	}
	// A pinned version nobody has: any version, listed.
	if got := ix.Lookup(patientURL + "|9.9.9"); len(got) != 3 {
		t.Errorf("unknown version Lookup = %v", got)
	}
	if got := ix.Lookup("http://nowhere/SD/y"); len(got) != 0 {
		t.Errorf("unknown canonical = %v", got)
	}
}

func TestAnnotate(t *testing.T) {
	fakeCache(t, map[string][]string{
		"kbv.basis#1.10.0": {patientURL},
		"kbv.basis#1.9.0":  {patientURL},
	})
	results := []*validator.Result{{Issues: []validator.Issue{
		{MessageID: "VALIDATION_VAL_PROFILE_UNKNOWN_ERROR", Message: "Profile reference '" + patientURL + "|1.10.0' has not been checked because it could not be found, and fetching it resulted in the error x"},
		{MessageID: "Validation_VAL_Profile_Unknown", Message: "Profile reference 'http://nowhere/SD/y' has not been checked because it could not be found"},
		{MessageID: "Extension_EXT_Unknown_NotHere", Message: "The extension http://nowhere/SD/ext could not be found so is not allowed here", Redacted: true},
		{MessageID: "dom-6", Message: "about " + patientURL},
	}}}
	Annotate(results)
	iss := results[0].Issues

	h := iss[0].Hint
	if h == nil || h.Package != "kbv.basis#1.10.0" || h.Canonical != patientURL+"|1.10.0" || h.Alias != "kbv-basis" {
		t.Errorf("found: %+v", h)
	}
	if h := iss[1].Hint; h == nil || h.Package != "" || h.Canonical != "http://nowhere/SD/y" {
		t.Errorf("not found: %+v", h)
	}
	if iss[2].Hint != nil {
		t.Error("a redacted issue got a hint")
	}
	if iss[3].Hint != nil {
		t.Error("an unrelated message got a hint")
	}
}

// No issue that needs a hint: the cache is never read.
func TestAnnotate_NoIndexWithoutNeed(t *testing.T) {
	var a Annotator
	a.Annotate([]*validator.Result{{Issues: []validator.Issue{{MessageID: "dom-6"}}}})
	if a.ix != nil {
		t.Error("index built without need")
	}
}

func TestParseIndex(t *testing.T) {
	got := parseIndex([]byte(`{"files":[{"url":"http://a","version":"1"},{"url":""},{"url":"http://b"}]}`))
	if !reflect.DeepEqual(got, []string{"http://a", "http://a|1", "http://b"}) {
		t.Errorf("parseIndex = %v", got)
	}
	if parseIndex([]byte("not json")) != nil {
		t.Error("bad JSON gave keys")
	}
}

// FuzzParseIndex: .index.json is written by other tools; any content parses
// without panicking, and every key is a non-empty url.
func FuzzParseIndex(f *testing.F) {
	f.Add([]byte(`{"files":[{"url":"http://a","version":"1"}]}`))
	f.Add([]byte(`{"files":[{"url":5}]}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		for _, k := range parseIndex(data) {
			if k == "" {
				t.Fatal("empty key")
			}
		}
	})
}
