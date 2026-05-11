package validator

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

const advisoriesURL = "https://api.github.com/repos/hapifhir/org.hl7.fhir.core/security-advisories"

// AuditReport holds the results of a security audit of the validator JAR.
type AuditReport struct {
	CurrentVersion string
	LatestVersion  string
	IsOutdated     bool
	Advisories     []Advisory // all published advisories
	VersionError   string
	AdvisoryError  string
}

// AffectingAdvisories returns advisories that affect the current version.
func (r AuditReport) AffectingAdvisories() []Advisory {
	var out []Advisory
	for _, a := range r.Advisories {
		if a.AffectsVersion(r.CurrentVersion) {
			out = append(out, a)
		}
	}
	return out
}

// Advisory represents a GitHub Security Advisory.
type Advisory struct {
	GHSAID          string          `json:"ghsa_id"`
	Summary         string          `json:"summary"`
	Severity        string          `json:"severity"`
	PublishedAt     string          `json:"published_at"`
	HTMLURL         string          `json:"html_url"`
	Vulnerabilities []Vulnerability `json:"vulnerabilities"`
}

// Vulnerability holds one package/version-range pair within an advisory.
type Vulnerability struct {
	Package struct {
		Name string `json:"name"`
	} `json:"package"`
	VulnerableVersionRange string `json:"vulnerable_version_range"`
}

// AffectsVersion reports whether version falls within any of the advisory's
// vulnerable ranges. Returns true when version is "unknown" (can't rule it out).
func (a Advisory) AffectsVersion(version string) bool {
	if version == "unknown" {
		return true
	}
	for _, v := range a.Vulnerabilities {
		if rangeAffects(v.VulnerableVersionRange, version) {
			return true
		}
	}
	return false
}

// rangeAffects checks whether version falls within a simple "< X.Y.Z" range
// as used in GitHub advisory data for the validator JAR.
func rangeAffects(rangeStr, version string) bool {
	r := strings.TrimSpace(rangeStr)
	if strings.HasPrefix(r, "< ") {
		upper := strings.TrimSpace(strings.TrimPrefix(r, "< "))
		return semverLess(version, upper)
	}
	return false
}

// semverLess returns true if a < b for "X.Y.Z"-style version strings.
func semverLess(a, b string) bool {
	ap := strings.Split(a, ".")
	bp := strings.Split(b, ".")
	for i := range 3 {
		av, bv := 0, 0
		if i < len(ap) {
			av, _ = strconv.Atoi(ap[i])
		}
		if i < len(bp) {
			bv, _ = strconv.Atoi(bp[i])
		}
		if av < bv {
			return true
		}
		if av > bv {
			return false
		}
	}
	return false
}

// Audit checks the validator JAR version and queries the GitHub Security Advisory database.
func Audit() AuditReport {
	report := AuditReport{
		CurrentVersion: ValidatorVersion(),
	}

	latest, err := FetchLatestVersion()
	if err != nil {
		report.VersionError = err.Error()
	} else {
		report.LatestVersion = latest
		if report.CurrentVersion != "unknown" {
			report.IsOutdated = latest != report.CurrentVersion
		}
	}

	advisories, err := fetchAdvisories()
	if err != nil {
		report.AdvisoryError = err.Error()
	} else {
		report.Advisories = advisories
	}

	return report
}

func fetchAdvisories() ([]Advisory, error) {
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, advisoriesURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var advisories []Advisory
	return advisories, json.NewDecoder(resp.Body).Decode(&advisories)
}
