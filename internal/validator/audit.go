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

// rangeAffects reports whether version satisfies a GitHub advisory version
// range. A range is a comma-separated set of constraints that must all hold,
// e.g. "< 6.9.4", "<= 6.9.6", or ">= 6.0.0, < 6.9.4". Unrecognised constraints
// are treated as not matching.
func rangeAffects(rangeStr, version string) bool {
	rangeStr = strings.TrimSpace(rangeStr)
	if rangeStr == "" {
		return false
	}
	for _, part := range strings.Split(rangeStr, ",") {
		if !constraintSatisfied(strings.TrimSpace(part), version) {
			return false
		}
	}
	return true
}

// constraintSatisfied evaluates a single constraint like "< 6.9.4" or "<= 6.9.6".
func constraintSatisfied(constraint, version string) bool {
	// Two-character operators must be checked before their single-char prefixes.
	for _, op := range []string{"<=", ">=", "==", "<", ">", "="} {
		if strings.HasPrefix(constraint, op) {
			cmp := semverCompare(version, strings.TrimSpace(constraint[len(op):]))
			switch op {
			case "<":
				return cmp < 0
			case "<=":
				return cmp <= 0
			case ">":
				return cmp > 0
			case ">=":
				return cmp >= 0
			case "=", "==":
				return cmp == 0
			}
		}
	}
	return false
}

// semverCompare returns -1, 0, or 1 comparing "X.Y.Z"-style version strings.
func semverCompare(a, b string) int {
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
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
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
