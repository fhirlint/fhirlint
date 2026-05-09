package validator

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const advisoriesURL = "https://api.github.com/repos/hapifhir/org.hl7.fhir.core/security-advisories"

// AuditReport holds the results of a security audit of the validator JAR.
type AuditReport struct {
	CurrentVersion string
	LatestVersion  string
	IsOutdated     bool
	Advisories     []Advisory
	VersionError   string
	AdvisoryError  string
}

// Advisory represents a GitHub Security Advisory.
type Advisory struct {
	GHSAID      string `json:"ghsa_id"`
	Summary     string `json:"summary"`
	Severity    string `json:"severity"`
	PublishedAt string `json:"published_at"`
	HTMLURL     string `json:"html_url"`
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
