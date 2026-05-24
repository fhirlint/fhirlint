package reporter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fhirlint/fhirlint/internal/validator"
)

// ccDefaultCheck is used when an issue carries no HL7 message ID.
const ccDefaultCheck = "fhir-validation"

// ccIssue is one entry of the CodeClimate / GitLab Code Quality report format.
// See https://docs.gitlab.com/ee/ci/testing/code_quality.html#implement-a-custom-tool
type ccIssue struct {
	Description string     `json:"description"`
	CheckName   string     `json:"check_name"`
	Fingerprint string     `json:"fingerprint"`
	Severity    string     `json:"severity"`
	Location    ccLocation `json:"location"`
}

type ccLocation struct {
	Path  string  `json:"path"`
	Lines ccLines `json:"lines"`
}

type ccLines struct {
	Begin int `json:"begin"`
}

// CodeClimate writes a CodeClimate-format JSON report consumable by GitLab's
// Code Quality widget. When dest is empty the report is printed to stdout.
func CodeClimate(results []*validator.Result, minSeverity, dest string) error {
	report := buildCodeClimateReport(results, minSeverity)
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

func buildCodeClimateReport(results []*validator.Result, minSeverity string) []ccIssue {
	out := make([]ccIssue, 0)
	for _, r := range results {
		path := filepath.ToSlash(r.Filename)
		for _, iss := range filterIssues(r.Issues, minSeverity) {
			_, line, _ := parseLocationString(iss.Location)
			if line <= 0 {
				// GitLab requires a positive begin line; fall back to the top of the file.
				line = 1
			}

			check := iss.MessageID
			if check == "" {
				check = ccDefaultCheck
			}

			out = append(out, ccIssue{
				Description: ccDescription(iss),
				CheckName:   check,
				Fingerprint: ccFingerprint(path, iss),
				Severity:    ccSeverity(iss.Severity),
				Location: ccLocation{
					Path:  path,
					Lines: ccLines{Begin: line},
				},
			})
		}
	}
	return out
}

// ccDescription prefixes the message with its HL7 message ID when present,
// e.g. "dom-6: A resource should have narrative for robust management".
func ccDescription(iss validator.Issue) string {
	if iss.MessageID != "" {
		return iss.MessageID + ": " + iss.Message
	}
	return iss.Message
}

// ccSeverity maps FHIR issue severities onto the CodeClimate severity scale.
func ccSeverity(severity string) string {
	switch severity {
	case "fatal":
		return "critical"
	case "error":
		return "major"
	case "warning":
		return "minor"
	default:
		return "info"
	}
}

// ccFingerprint derives a stable identifier from the parts of an issue that do
// not change between runs, so GitLab can track findings as new or resolved.
func ccFingerprint(path string, iss validator.Issue) string {
	h := sha256.Sum256([]byte(path + "\x00" + iss.MessageID + "\x00" + iss.Location + "\x00" + iss.Message))
	return hex.EncodeToString(h[:])
}
