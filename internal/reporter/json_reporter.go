package reporter

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/fhirlint/fhirlint/internal/validator"
)

type JSONReport struct {
	Valid bool `json:"valid"`
	// Meta says what produced the report. Additive: readers that know only
	// valid/files/summary are unaffected, and an older report without it
	// still parses (fhirlint diff reads reports back).
	Meta    *RunInfo            `json:"meta,omitempty"`
	Files   []*validator.Result `json:"files"`
	Summary JSONSummary         `json:"summary"`
}

type JSONSummary struct {
	Total    int `json:"total"`
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Info     int `json:"info"`
}

func JSON(results []*validator.Result, minSeverity string, info RunInfo, dest string) error {
	report := buildJSONReport(results, minSeverity)
	report.Meta = metaFor(results, info)
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if dest == "" {
		fmt.Println(string(data))
		return nil
	}
	return os.WriteFile(dest, data, 0600)
}

// metaFor is the meta block for a report, or nil when there is nothing to say
// — so that a report built without provenance has no empty object in it.
func metaFor(results []*validator.Result, info RunInfo) *RunInfo {
	info = info.WithResults(results)
	if info.empty() {
		return nil
	}
	return &info
}

func buildJSONReport(results []*validator.Result, minSeverity string) JSONReport {
	summary := JSONSummary{}
	allValid := true
	filtered := make([]*validator.Result, 0, len(results))

	for _, r := range results {
		issues := filterIssues(r.Issues, minSeverity)
		copy := *r
		copy.Issues = issues
		filtered = append(filtered, &copy)

		if !r.Valid {
			allValid = false
		}
		for _, i := range issues {
			summary.Total++
			switch i.Severity {
			case "error":
				summary.Errors++
			case "warning":
				summary.Warnings++
			default:
				summary.Info++
			}
		}
	}
	return JSONReport{Valid: allValid, Files: filtered, Summary: summary}
}
