package validator

import (
	"testing"
)

func TestSemverLess(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"6.9.3", "6.9.4", true},
		{"6.9.4", "6.9.4", false},
		{"6.9.7", "6.9.4", false},
		{"5.6.91", "5.6.92", true},
		{"6.0.0", "6.0.1", true},
		{"6.1.0", "6.0.9", false},
		{"7.0.0", "6.9.9", false},
	}
	for _, tc := range tests {
		if got := semverLess(tc.a, tc.b); got != tc.want {
			t.Errorf("semverLess(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestRangeAffects(t *testing.T) {
	tests := []struct {
		rangeStr string
		version  string
		want     bool
	}{
		{"< 6.9.4", "6.9.3", true},
		{"< 6.9.4", "6.9.4", false},
		{"< 6.9.4", "6.9.7", false},
		{"< 5.6.92", "5.6.91", true},
		{"< 5.6.92", "5.6.92", false},
		{"unsupported", "6.9.7", false},
		{"", "6.9.7", false},
	}
	for _, tc := range tests {
		if got := rangeAffects(tc.rangeStr, tc.version); got != tc.want {
			t.Errorf("rangeAffects(%q, %q) = %v, want %v", tc.rangeStr, tc.version, got, tc.want)
		}
	}
}

func TestAdvisoryAffectsVersion(t *testing.T) {
	adv := Advisory{
		GHSAID:   "GHSA-test-0000-0000",
		Severity: "critical",
		Vulnerabilities: []Vulnerability{
			{VulnerableVersionRange: "< 6.9.4"},
		},
	}

	if !adv.AffectsVersion("6.9.3") {
		t.Error("expected 6.9.3 to be affected")
	}
	if adv.AffectsVersion("6.9.4") {
		t.Error("expected 6.9.4 to not be affected")
	}
	if adv.AffectsVersion("6.9.7") {
		t.Error("expected 6.9.7 to not be affected")
	}
	if !adv.AffectsVersion("unknown") {
		t.Error("expected unknown version to be considered affected")
	}
}

func TestAuditReportAffectingAdvisories(t *testing.T) {
	report := AuditReport{
		CurrentVersion: "6.9.7",
		Advisories: []Advisory{
			{
				GHSAID:          "GHSA-old",
				Vulnerabilities: []Vulnerability{{VulnerableVersionRange: "< 5.6.92"}},
			},
			{
				GHSAID:          "GHSA-new",
				Vulnerabilities: []Vulnerability{{VulnerableVersionRange: "< 6.9.4"}},
			},
		},
	}

	affecting := report.AffectingAdvisories()
	if len(affecting) != 0 {
		t.Errorf("expected 0 affecting advisories for 6.9.7, got %d", len(affecting))
	}

	report.CurrentVersion = "6.9.3"
	affecting = report.AffectingAdvisories()
	if len(affecting) != 1 {
		t.Errorf("expected 1 affecting advisory for 6.9.3, got %d", len(affecting))
	}
	if affecting[0].GHSAID != "GHSA-new" {
		t.Errorf("expected GHSA-new, got %s", affecting[0].GHSAID)
	}
}
