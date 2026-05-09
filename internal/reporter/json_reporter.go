package reporter

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/fhirlint/fhirlint/internal/validator"
)

type JSONReport struct {
	Valid   bool              `json:"valid"`
	Files   []*validator.Result `json:"files"`
	Summary JSONSummary       `json:"summary"`
}

type JSONSummary struct {
	Total    int `json:"total"`
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Info     int `json:"info"`
}

func JSON(results []*validator.Result, minSeverity string, dest string) error {
	report := buildJSONReport(results, minSeverity)
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
