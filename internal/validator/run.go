package validator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
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
	Filename   string  `json:"filename"`
	Label      string  `json:"label"` // human-readable source (path, URL, "stdin")
	Valid       bool    `json:"valid"`
	Issues     []Issue `json:"issues"`
	Suppressed []Issue `json:"suppressed,omitempty"` // populated after suppress.Apply
	Cached     bool    `json:"cached,omitempty"`     // true when result came from the result cache
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
	FHIRVersion         string
	Profiles            []string
	IGs                 []string
	NoTerminologyServer bool
	TerminologyServer   string
	BestPractice       string // ignore | hint | warning | error (empty = JAR default)
	TxCache            string // path to terminology cache dir, or "n/a" to disable
	Locale             string // Java locale code, e.g. "de", "fr" (empty = JAR default)
	AllowExampleURLs   bool   // pass -allow-example-urls to suppress example.org warnings
	JARPath            string // override auto-downloaded JAR (--jar / FHIRLINT_JAR)
}

// buildArgs constructs the java -jar argument list for the given inputs and options.
// Separated from Run() so it can be unit-tested without invoking the JAR.
func buildArgs(jarPath string, inputPaths []string, outputPath string, opts Options) []string {
	args := []string{"-jar", jarPath}
	args = append(args, inputPaths...)
	args = append(args, "-version", opts.FHIRVersion)
	// outputPath is empty in watch mode — skip structured output flags.
	if outputPath != "" {
		args = append(args, "-output-style", "json", "-output", outputPath)
	}
	for _, ig := range opts.IGs {
		args = append(args, "-ig", ig)
	}
	for _, p := range opts.Profiles {
		if strings.Contains(p, "#") {
			args = append(args, "-ig", p)
		} else {
			args = append(args, "-profile", p)
		}
	}
	switch {
	case opts.NoTerminologyServer:
		args = append(args, "-tx", "n/a")
	case opts.TerminologyServer != "":
		args = append(args, "-tx", opts.TerminologyServer)
	}
	if opts.BestPractice != "" {
		args = append(args, "-best-practice", opts.BestPractice)
	}
	if opts.TxCache != "" {
		args = append(args, "-txCache", opts.TxCache)
	}
	if opts.Locale != "" {
		args = append(args, "-locale", opts.Locale)
	}
	if opts.AllowExampleURLs {
		args = append(args, "-allow-example-urls")
	}
	return args
}

// RunWatch starts the JAR in watch mode and blocks until the process is killed (Ctrl-C).
// mode must be "single" or "all". intervalMS sets the polling interval in milliseconds (0 = JAR default).
// The JAR prints results directly to stdout/stderr — no structured output is captured.
func RunWatch(inputPaths []string, opts Options, mode string, intervalMS int) error {
	jarPath, err := EnsureJAR(opts.JARPath)
	if err != nil {
		return err
	}

	args := buildArgs(jarPath, inputPaths, "", opts)
	args = append(args, "-watch-mode", mode)
	if intervalMS > 0 {
		args = append(args, "-watch-interval", strconv.Itoa(intervalMS))
	}

	cmd := exec.Command("java", args...) //nolint:gosec // intentional: runs java with user-controlled paths
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Run validates a single file and returns its result.
func Run(inputPath string, opts Options) (*Result, error) {
	results, err := RunMultiple([]string{inputPath}, opts)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("validator produced no result for %s", inputPath)
	}
	return results[0], nil
}

// RunMultiple validates all inputPaths in a single JVM invocation.
// For directories, this avoids repeated JVM startup and terminology load overhead.
func RunMultiple(inputPaths []string, opts Options) ([]*Result, error) {
	if len(inputPaths) == 0 {
		return nil, nil
	}

	jarPath, err := EnsureJAR(opts.JARPath)
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

	args := buildArgs(jarPath, inputPaths, tmpFile.Name(), opts)

	var stderrBuf bytes.Buffer
	cmd := exec.Command("java", args...) //nolint:gosec // intentional: runs java with user-controlled input paths
	cmd.Stdout = nil
	cmd.Stderr = &stderrBuf
	// Non-zero exit is expected when validation finds errors — not a tool failure.
	_ = cmd.Run()

	jsonBytes, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		return nil, fmt.Errorf("reading validator output: %w", err)
	}

	if len(jsonBytes) == 0 {
		if err := oomError(stderrBuf.String()); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("validator produced no output — JAR may have crashed\nfiles: %v\nstderr: %s", inputPaths, stderrBuf.String())
	}

	return parseOutput(jsonBytes, inputPaths, stderrBuf.String())
}

func oomError(stderr string) error {
	if strings.Contains(stderr, "OutOfMemoryError") {
		fmt.Fprintln(os.Stderr, "Java ran out of memory while validating. Try increasing the heap size:")
		fmt.Fprintln(os.Stderr, `  JAVA_OPTS="-Xmx2g" fhirlint validate`)
		return errors.New("out of memory")
	}
	return nil
}

// parseOutput handles both single OperationOutcome (one file) and Bundle (multiple files).
func parseOutput(data []byte, inputPaths []string, stderr string) ([]*Result, error) {
	var peek struct {
		ResourceType string `json:"resourceType"`
	}
	if err := json.Unmarshal(data, &peek); err != nil {
		return nil, fmt.Errorf("parsing validator output: %w\nraw: %s\nstderr: %s", err, data, stderr)
	}

	switch peek.ResourceType {
	case "OperationOutcome":
		var oo operationOutcome
		if err := json.Unmarshal(data, &oo); err != nil {
			return nil, fmt.Errorf("parsing OperationOutcome: %w\nraw: %s\nstderr: %s", err, data, stderr)
		}
		filename := ""
		if len(inputPaths) > 0 {
			filename = inputPaths[0]
		}
		return []*Result{toResult(oo, filename)}, nil

	case "Bundle":
		var bundle struct {
			Entry []struct {
				FullURL  string           `json:"fullUrl"`
				Resource operationOutcome `json:"resource"`
			} `json:"entry"`
		}
		if err := json.Unmarshal(data, &bundle); err != nil {
			return nil, fmt.Errorf("parsing Bundle: %w\nraw: %s\nstderr: %s", err, data, stderr)
		}
		results := make([]*Result, len(bundle.Entry))
		for i, entry := range bundle.Entry {
			// Use positional mapping: entry[i] corresponds to inputPaths[i].
			filename := ""
			if i < len(inputPaths) {
				filename = inputPaths[i]
			} else {
				filename = strings.TrimPrefix(entry.FullURL, "file://")
			}
			results[i] = toResult(entry.Resource, filename)
		}
		return results, nil

	default:
		return nil, fmt.Errorf("unexpected resourceType %q in validator output\nstderr: %s", peek.ResourceType, stderr)
	}
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
