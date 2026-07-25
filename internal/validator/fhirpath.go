package validator

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// FHIRPathOptions configures a FHIRPath evaluation.
type FHIRPathOptions struct {
	FHIRVersion      string        // FHIR context version (e.g. "4.0.1")
	JARPath          string        // override auto-downloaded JAR (--jar / FHIRLINT_JAR)
	ValidatorVersion string        // pin the auto-downloaded JAR to an upstream release (--validator-version)
	Timeout          time.Duration // 0 means no timeout
}

// FHIRPathResult holds the outcome of evaluating one FHIRPath expression.
type FHIRPathResult struct {
	Expression string   `json:"expression"`
	Items      []string `json:"result"` // one entry per result item; empty slice means no match
}

// Empty reports whether the expression produced no result items.
func (r *FHIRPathResult) Empty() bool { return len(r.Items) == 0 }

// fhirpathMarker precedes the result on the JAR's stdout: " ...evaluating <expr>".
const fhirpathMarker = "...evaluating"

// fhirpathErrorLine is printed by the JAR when evaluation or parsing fails.
const fhirpathErrorLine = "Error evaluating FHIRPath expression."

// RunFHIRPath evaluates expr against the resource at inputPath using the validator
// JAR's `fhirpath` engine mode. Terminology is disabled (-tx n/a): FHIRPath
// inspection rarely needs a terminology server, and this keeps the command fast
// and offline.
//
// A nil error with a (possibly empty) result means the expression evaluated
// successfully — an empty or false result is not an error. A non-nil error means
// the expression was malformed, the resource was unparseable, or the JAR failed.
func RunFHIRPath(expr, inputPath string, opts FHIRPathOptions) (*FHIRPathResult, error) {
	if strings.TrimSpace(expr) == "" {
		return nil, fmt.Errorf("empty FHIRPath expression")
	}
	if err := validateFHIRVersion(opts.FHIRVersion); err != nil {
		return nil, err
	}

	jarPath, err := EnsureJAR(opts.JARPath, opts.ValidatorVersion)
	if err != nil {
		return nil, err
	}

	args := []string{"-jar", jarPath, "fhirpath", expr, inputPath, "-version", opts.FHIRVersion, "-tx", "n/a"}

	ctx := context.Background()
	var cancel context.CancelFunc = func() {}
	if opts.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
	}
	defer cancel()

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd := exec.CommandContext(ctx, "java", args...) //nolint:gosec // intentional: runs java with user-controlled input paths
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	runErr := cmd.Run()

	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("FHIRPath evaluation timed out after %s — use --timeout to increase the limit", formatDuration(opts.Timeout))
	}

	if oomErr := oomError(stderrBuf.String()); oomErr != nil {
		return nil, oomErr
	}

	return parseFHIRPathOutput(stdoutBuf.String(), expr, runErr)
}

// parseFHIRPathOutput extracts the result block the JAR prints between the
// " ...evaluating <expr>" marker and the trailing "Done." line. The JAR prints
// everything (logs, result, and errors) to stdout, so the block must be located
// rather than read wholesale. runErr is the process exit error, used as a
// fallback signal when the markers are absent.
func parseFHIRPathOutput(stdout, expr string, runErr error) (*FHIRPathResult, error) {
	lines := strings.Split(stdout, "\n")

	start, end := -1, -1
	for i, line := range lines {
		if start == -1 && strings.Contains(line, fhirpathMarker) {
			start = i
			continue
		}
		if start != -1 && strings.HasPrefix(strings.TrimSpace(line), "Done.") {
			end = i
			break
		}
	}

	if start == -1 {
		// The JAR never reached evaluation — a CLI/parse failure before the engine ran.
		return nil, fhirpathFailure(stdout, runErr)
	}
	if end == -1 {
		end = len(lines)
	}

	block := make([]string, 0, end-start-1)
	for _, line := range lines[start+1 : end] {
		block = append(block, strings.TrimRight(line, "\r"))
	}

	// Evaluation/parse error: the engine ran but the expression or resource was bad.
	for i, line := range block {
		if strings.TrimSpace(line) == fhirpathErrorLine {
			return nil, fmt.Errorf("%s", fhirpathErrorMessage(block[i+1:]))
		}
	}

	raw := strings.Trim(strings.Join(block, "\n"), "\n")
	if strings.TrimSpace(raw) == "" {
		return &FHIRPathResult{Expression: expr, Items: []string{}}, nil
	}
	return &FHIRPathResult{Expression: expr, Items: strings.Split(raw, ",")}, nil
}

// fhirpathErrorMessage turns the JAR's exception lines into a concise message.
// The first line after the error banner looks like
// "<exception.class>: <message>"; the class prefix is stripped.
func fhirpathErrorMessage(lines []string) string {
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if _, msg, found := strings.Cut(t, ": "); found {
			return msg
		}
		return t
	}
	return "FHIRPath evaluation failed"
}

// fhirpathFailure builds an error for the case where the JAR exited before
// evaluating (missing/invalid arguments, JAR crash). It surfaces the most
// useful line from stdout, falling back to the raw exit error.
func fhirpathFailure(stdout string, runErr error) error {
	for _, line := range strings.Split(stdout, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "Unable to parse command line arguments:") ||
			strings.HasPrefix(t, "Error:") {
			return fmt.Errorf("%s", t)
		}
	}
	if runErr != nil {
		return fmt.Errorf("FHIRPath evaluation failed: %w", runErr)
	}
	return fmt.Errorf("FHIRPath evaluation produced no result — the validator JAR may have crashed")
}
