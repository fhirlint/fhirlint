package fhirpkg

import "testing"

func TestIsPreRelease(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{"1.9.0", false},
		{"2027.0.0", false},
		{"2027.0.0-ballot.rc1", true},
		{"2026.0.2-rc.1", true},
		// KBV publishes these beside the release. Syntactically pre-releases,
		// and a caller asking "is this a version to point someone at?" wants
		// the same answer for both.
		{"1.9.0-Expansions", true},
		{"1.9.0-Resources", true},
		{"1.0.0+build.5", true},
		{"", false},
	}

	for _, tt := range tests {
		if got := IsPreRelease(tt.version); got != tt.want {
			t.Errorf("IsPreRelease(%q) = %v, want %v", tt.version, got, tt.want)
		}
	}
}
