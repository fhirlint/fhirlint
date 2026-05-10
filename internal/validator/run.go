package validator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Issue is our internal representation, mapped from OperationOutcome.issue.
type Issue struct {
	Severity  string `json:"severity"`  // fatal | error | warning | information
	Message   string `json:"message"`
	Location  string `json:"location"`
	MessageID string `json:"messageId"`
}

// Result holds the outcome for one validated resource.
type Result struct {
	Filename string  `json:"filename"`
	Label    string  `json:"label"` // human-readable source (path, URL, "stdin")
	Valid    bool    `json:"valid"`
	Issues   []Issue `json:"issues"`
}

// operationOutcome mirrors the FHIR OperationOutcome resource the validator emits.
type operationOutcome struct {
	ResourceType string    `json:"resourceType"`
	Extension    []extItem `json:"extension"`
	Issue        []ooIssue `json:"issue"`
}

type ooIssue struct {
	Severity   string    `json:"severity"`
	Code       string    `json:"code"`
	Details    ooDetails `json:"details"`
	Expression []string  `json:"expression"`
	Extension  []extItem `json:"extension"`
}

type ooDetails struct {
	Text string `json:"text"`
}

type extItem struct {
	URL          string `json:"url"`
	ValueString  string `json:"valueString,omitempty"`
	ValueInteger int    `json:"valueInteger,omitempty"`
	ValueCode    string `json:"valueCode,omitempty"`
}

type Options struct {
	FHIRVersion string
	Profiles    []string
	IGs         []string
}

func Run(inputPath string, opts Options) (*Result, error) {
	jarPath, err := EnsureJAR()
	if err != nil {
		return nil, err
	}

	// Write JSON output to a temp file to avoid mixing with validator log output.
	tmpFile, err := os.CreateTemp("", "fhirlint-result-*.json")
	if err != nil {
		return nil, fmt.Errorf("creating temp output file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return nil, fmt.Errorf("closing temp output file: %w", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	args := []string{"-jar", jarPath, inputPath,
		"-version", opts.FHIRVersion,
		"-output-style", "json",
		"-output", tmpFile.Name(),
	}
	for _, ig := range opts.IGs {
		args = append(args, "-ig", ig)
	}
	for _, p := range opts.Profiles {
		args = append(args, "-profile", p)
	}

	var stderrBuf bytes.Buffer
	cmd := exec.Command("java", args...) //nolint:gosec // intentional: runs java with user-controlled input path
	// Discard stdout — results go to the temp file. Capture stderr for diagnostics.
	cmd.Stdout = nil
	cmd.Stderr = &stderrBuf
	// Non-zero exit is expected when validation finds errors — not a tool failure.
	_ = cmd.Run()

	jsonBytes, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		return nil, fmt.Errorf("reading validator output: %w", err)
	}

	if len(jsonBytes) == 0 {
		return nil, fmt.Errorf("validator produced no output for %s — JAR may have crashed\nstderr: %s", inputPath, stderrBuf.String())
	}

	var oo operationOutcome
	if err := json.Unmarshal(jsonBytes, &oo); err != nil {
		return nil, fmt.Errorf("parsing OperationOutcome: %w\nraw: %s\nstderr: %s", err, jsonBytes, stderrBuf.String())
	}

	return toResult(oo, inputPath), nil
}


func toResult(oo operationOutcome, filename string) *Result {
	valid := true
	issues := make([]Issue, 0, len(oo.Issue))

	for _, i := range oo.Issue {
		if i.Severity == "error" || i.Severity == "fatal" {
			valid = false
		}

		location := strings.Join(i.Expression, ", ")
		line, col := 0, 0
		messageID := ""

		for _, ext := range i.Extension {
			switch ext.URL {
			case "http://hl7.org/fhir/StructureDefinition/operationoutcome-issue-line":
				line = ext.ValueInteger
			case "http://hl7.org/fhir/StructureDefinition/operationoutcome-issue-col":
				col = ext.ValueInteger
			case "http://hl7.org/fhir/StructureDefinition/operationoutcome-message-id":
				messageID = ext.ValueCode
			}
		}

		loc := location
		if line > 0 {
			loc = fmt.Sprintf("%s (line %d, col %d)", location, line, col)
		}

		issues = append(issues, Issue{
			Severity:  i.Severity,
			Message:   i.Details.Text,
			Location:  loc,
			MessageID: messageID,
		})
	}

	return &Result{
		Filename: filename,
		Valid:    valid,
		Issues:   issues,
	}
}
