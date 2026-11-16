// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package fhirpkg

import "testing"

// CompareVersions in the package that owns it (#691); igaudit only re-exports
// it, so its table alone left the pre-release ordering unmeasured here.
func TestCompareVersions(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
		ok   bool
	}{
		{"1.0.0", "1.0.0", 0, true},
		{"1.0", "1.0.0", 0, true}, // a missing segment is zero
		{"1.0.1", "1.0.0", 1, true},
		{"1.2.0", "1.10.0", -1, true}, // numeric, not lexical
		{"2.0.0", "2.0.0-ballot", 1, true},
		{"2.0.0-ballot", "2.0.0", -1, true},
		{"2.0.0-ballot", "2.0.0-ballot", 0, true},
		{"2.0.0-alpha", "2.0.0-beta", -1, true},
		{"2.0.0-beta", "2.0.0-alpha", 1, true},
		{"1.1.0-ballot", "1.0.0", 1, true}, // the core version decides first
		{"1", "1.0.0", 0, false},
		{"1.0.0", "1", 0, false},
		{"1.x.0", "1.0.0", 0, false},
		{"1.0.0", "1.-1.0", 0, false},
		{"current", "1.0.0", 0, false},
	} {
		got, ok := CompareVersions(tc.a, tc.b)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("CompareVersions(%q, %q) = %d, %v; want %d, %v", tc.a, tc.b, got, ok, tc.want, tc.ok)
		}
	}
}
