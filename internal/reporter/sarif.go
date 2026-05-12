package reporter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/fhirlint/fhirlint/internal/validator"
)

const (
	sarifVersion    = "2.1.0"
	sarifSchema     = "https://json.schemastore.org/sarif-2.1.0.json"
	sarifInfoURI    = "https://github.com/fhirlint/fhirlint"
	sarifDefaultRule = "fhir-validation"
)

var locationRE = regexp.MustCompile(`^(.*?)\s*\(line (\d+), col (\d+)\)$`)

type sarifReport struct {
	Version string     `json:"version"`
	Schema  string     `json:"$schema"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules,omitempty"`
}

type sarifRule struct {
	ID               string       `json:"id"`
	ShortDescription sarifMessage `json:"shortDescription"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId,omitempty"`
	Level     string          `json:"level"`
	Message   sarifMessage    `json:"message"`
	Locations []sarifLocation `json:"locations,omitempty"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation  `json:"physicalLocation"`
	LogicalLocations []sarifLogicalLocation `json:"logicalLocations,omitempty"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           *sarifRegion          `json:"region,omitempty"`
}

type sarifArtifactLocation struct {
	URI       string `json:"uri"`
	URIBaseID string `json:"uriBaseId,omitempty"`
}

type sarifRegion struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn,omitempty"`
}

type sarifLogicalLocation struct {
	Name string `json:"name"`
	Kind string `json:"kind,omitempty"`
}

func SARIF(results []*validator.Result, minSeverity, fhirlintVersion, dest string) error {
	report := buildSARIFReport(results, minSeverity, fhirlintVersion)
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if dest == "" {
		fmt.Print(string(data))
		return nil
	}
	return os.WriteFile(dest, data, 0600)
}

func buildSARIFReport(results []*validator.Result, minSeverity, fhirlintVersion string) sarifReport {
	var sarifResults []sarifResult
	rulesSeen := map[string]bool{}
	var rules []sarifRule

	for _, r := range results {
		issues := filterIssues(r.Issues, minSeverity)
		for _, iss := range issues {
			ruleID := iss.MessageID
			if ruleID == "" {
				ruleID = sarifDefaultRule
			}
			if !rulesSeen[ruleID] {
				rulesSeen[ruleID] = true
				rules = append(rules, sarifRule{
					ID:               ruleID,
					ShortDescription: sarifMessage{Text: ruleID},
				})
			}

			sr := sarifResult{
				RuleID:  ruleID,
				Level:   sarifLevel(iss.Severity),
				Message: sarifMessage{Text: iss.Message},
			}

			loc := buildSARIFLocation(r.Filename, iss.Location)
			if loc != nil {
				sr.Locations = []sarifLocation{*loc}
			}

			sarifResults = append(sarifResults, sr)
		}
	}

	return sarifReport{
		Version: sarifVersion,
		Schema:  sarifSchema,
		Runs: []sarifRun{{
			Tool: sarifTool{
				Driver: sarifDriver{
					Name:           "fhirlint",
					Version:        fhirlintVersion,
					InformationURI: sarifInfoURI,
					Rules:          rules,
				},
			},
			Results: sarifResults,
		}},
	}
}

func buildSARIFLocation(filename, location string) *sarifLocation {
	if filename == "" {
		return nil
	}

	uri := filepath.ToSlash(filename)

	expression, line, col := parseLocationString(location)

	loc := &sarifLocation{
		PhysicalLocation: sarifPhysicalLocation{
			ArtifactLocation: sarifArtifactLocation{
				URI:       uri,
				URIBaseID: "%SRCROOT%",
			},
		},
	}

	if line > 0 {
		loc.PhysicalLocation.Region = &sarifRegion{
			StartLine:   line,
			StartColumn: col,
		}
	}

	if expression != "" {
		loc.LogicalLocations = []sarifLogicalLocation{{
			Name: expression,
			Kind: "member",
		}}
	}

	return loc
}

// parseLocationString splits "expression (line N, col M)" into its parts.
func parseLocationString(loc string) (expression string, line, col int) {
	loc = strings.TrimSpace(loc)
	if m := locationRE.FindStringSubmatch(loc); m != nil {
		expression = strings.TrimSpace(m[1])
		line, _ = strconv.Atoi(m[2])
		col, _ = strconv.Atoi(m[3])
		return
	}
	return loc, 0, 0
}

func sarifLevel(severity string) string {
	switch severity {
	case "error", "fatal":
		return "error"
	case "warning":
		return "warning"
	default:
		return "note"
	}
}
