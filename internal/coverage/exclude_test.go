// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package coverage

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Excludes match on the path relative to the input directory, the way
// validate's do (#642).
func TestLoadResources_ExcludesRelativeToTheRoot(t *testing.T) {
	// The input itself lives under a directory named data.
	root := filepath.Join(t.TempDir(), "data", "fhir")
	patient := `{"resourceType":"Patient","id":"p"}`
	for _, f := range []string{"p.json", "data/q.json", "other/mydata/r.json", "fixtures/s.json", "deep/fixtures/t.json"} {
		path := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(patient), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	loaded := func(exclude ...string) string {
		t.Helper()
		rs, _, err := LoadResources(root, exclude)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, r := range rs {
			rel, _ := filepath.Rel(root, r.Path)
			out = append(out, filepath.ToSlash(rel))
		}
		sort.Strings(out)
		return strings.Join(out, " ")
	}

	for _, tc := range []struct {
		exclude []string
		want    string
	}{
		// data/ is the data directory under the input: not the input's own
		// parent, and not mydata.
		{[]string{"data/"}, "deep/fixtures/t.json fixtures/s.json other/mydata/r.json p.json"},
		// A name without a slash is a basename at any depth, a directory
		// included.
		{[]string{"fixtures"}, "data/q.json other/mydata/r.json p.json"},
		{[]string{"*.json"}, ""},
		{[]string{"other/**"}, "data/q.json deep/fixtures/t.json fixtures/s.json p.json"},
	} {
		if got := loaded(tc.exclude...); got != tc.want {
			t.Errorf("exclude %q:\n got %s\nwant %s", tc.exclude, got, tc.want)
		}
	}

	// A file named directly is read even when a pattern matches it, as
	// validate does with a file argument.
	rs, _, err := LoadResources(filepath.Join(root, "p.json"), []string{"*.json"})
	if err != nil || len(rs) != 1 {
		t.Errorf("a file given as the input: %d resources, %v", len(rs), err)
	}
}
