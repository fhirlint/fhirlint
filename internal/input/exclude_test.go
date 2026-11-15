// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package input

import "testing"

func TestMatchesExclude_TrailingSlash(t *testing.T) {
	cases := []struct {
		relPath string
		pattern string
		want    bool
	}{
		{"vendor/foo.json", "vendor/", true},
		{"vendor", "vendor/", true},
		{"src/vendor/foo.json", "vendor/", false},
		{"tests/fixtures/legacy/a.json", "tests/fixtures/legacy/**", true},
		{"tests/fixtures/other/a.json", "tests/fixtures/legacy/**", false},
		{"src/generated/foo.json", "src/generated/*.json", true},
		{"src/generated/sub/foo.json", "src/generated/*.json", false},
		{"foo.json", "*.json", true},
		{"src/foo.json", "*.json", true},
		{"src/foo.xml", "*.json", false},
		{"src/exact.json", "src/exact.json", true},
		{"src/other.json", "src/exact.json", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got := MatchesExclude(tc.relPath, tc.pattern)
		if got != tc.want {
			t.Errorf("MatchesExclude(%q, %q) = %v, want %v", tc.relPath, tc.pattern, got, tc.want)
		}
	}
}
