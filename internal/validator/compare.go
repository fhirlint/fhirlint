package validator

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// CompareOptions configures a profile comparison.
type CompareOptions struct {
	FHIRVersion string        // FHIR context version (e.g. "4.0.1")
	IGs         []string      // implementation guides / files to load (-ig): both sides plus extras
	DestDir     string        // directory the JAR writes its HTML site to; created if needed
	JARPath     string        // override auto-downloaded JAR (--jar / FHIRLINT_JAR)
	Timeout     time.Duration // 0 means no timeout
}

// CompareMessage is one difference the validator's ComparisonService reports
// between the two profiles.
type CompareMessage struct {
	Severity string `json:"severity"` // error | warning | information
	Path     string `json:"path"`     // element or metadata path the difference is on
	Message  string `json:"message"`  // human-readable description of the difference
}

// CompareResult holds the outcome of comparing two StructureDefinitions.
type CompareResult struct {
	Left     string           `json:"left"`     // canonical of the left profile
	Right    string           `json:"right"`    // canonical of the right profile
	Messages []CompareMessage `json:"messages"` // differences; empty means the profiles are equivalent
	HTMLDir  string           `json:"-"`        // directory holding the generated HTML site
	HTMLFile string           `json:"-"`        // entry point of the side-by-side comparison
}

// Differs reports whether the comparison found any differences.
func (r *CompareResult) Differs() bool { return len(r.Messages) > 0 }

// RunCompare compares the left and right StructureDefinitions (given by canonical
// URL) using the validator JAR's `compare` engine. Both profiles must be loadable
// from opts.IGs (package specs or local files). The JAR writes a full HTML
// comparison site to opts.DestDir; the structured message list is parsed back out
// of it. Terminology is disabled (-tx n/a): structural comparison does not need a
// terminology server, and this keeps the command fast and offline.
func RunCompare(left, right string, opts CompareOptions) (*CompareResult, error) {
	if strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
		return nil, fmt.Errorf("both a left and right profile are required")
	}
	if err := validateFHIRVersion(opts.FHIRVersion); err != nil {
		return nil, err
	}
	if opts.DestDir == "" {
		return nil, fmt.Errorf("no destination directory for the comparison output")
	}
	if err := os.MkdirAll(opts.DestDir, 0750); err != nil {
		return nil, fmt.Errorf("creating output directory: %w", err)
	}

	jarPath, err := EnsureJAR(opts.JARPath)
	if err != nil {
		return nil, err
	}

	args := []string{"-jar", jarPath, "compare", "-version", opts.FHIRVersion, "-tx", "n/a"}
	for _, ig := range opts.IGs {
		args = append(args, "-ig", ig)
	}
	args = append(args, "-left", left, "-right", right, "-dest", opts.DestDir)

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
		return nil, fmt.Errorf("comparison timed out after %s — use --timeout to increase the limit", formatDuration(opts.Timeout))
	}
	if oomErr := oomError(stderrBuf.String()); oomErr != nil {
		return nil, oomErr
	}

	mainHTML := findComparisonHTML(opts.DestDir)
	if mainHTML == "" {
		return nil, compareFailure(stdoutBuf.String(), runErr)
	}

	data, err := os.ReadFile(mainHTML) //nolint:gosec // path produced by findComparisonHTML within our dest dir
	if err != nil {
		return nil, fmt.Errorf("reading comparison output: %w", err)
	}

	return &CompareResult{
		Left:     left,
		Right:    right,
		Messages: parseCompareMessages(string(data)),
		HTMLDir:  opts.DestDir,
		HTMLFile: mainHTML,
	}, nil
}

// findComparisonHTML returns the path to the main side-by-side comparison file
// the JAR writes (sd+N-<uuid>-<uuid>.html), excluding the -union/-intersection
// variants. Returns "" when no comparison was produced.
func findComparisonHTML(destDir string) string {
	matches, err := filepath.Glob(filepath.Join(destDir, "sd*.html"))
	if err != nil {
		return ""
	}
	for _, m := range matches {
		base := filepath.Base(m)
		if strings.Contains(base, "-union.") || strings.Contains(base, "-intersection.") {
			continue
		}
		return m
	}
	return ""
}

// compareMessageRow matches one row of the ComparisonService "Messages" table:
// <tr ...><td>Severity</td><td>Path</td><td>Message</td></tr>.
var compareMessageRow = regexp.MustCompile(`(?is)<tr[^>]*>\s*<td>(.*?)</td>\s*<td>(.*?)</td>\s*<td>(.*?)</td>\s*</tr>`)

// stripTags removes HTML tags and unescapes entities from a table cell.
var stripTags = regexp.MustCompile(`(?s)<[^>]+>`)

// parseCompareMessages extracts the structured difference list from the JAR's
// comparison HTML. The "Messages" section is a <table class="grid"> whose rows
// are (severity, path, description). Returns an empty slice when there is no
// Messages table (the profiles are equivalent).
func parseCompareMessages(htmlDoc string) []CompareMessage {
	start := strings.Index(htmlDoc, ">Messages<")
	if start == -1 {
		return []CompareMessage{}
	}
	tableStart := strings.Index(htmlDoc[start:], "<table")
	if tableStart == -1 {
		return []CompareMessage{}
	}
	tableStart += start
	tableEnd := strings.Index(htmlDoc[tableStart:], "</table>")
	if tableEnd == -1 {
		return []CompareMessage{}
	}
	table := htmlDoc[tableStart : tableStart+tableEnd]

	messages := []CompareMessage{}
	for _, row := range compareMessageRow.FindAllStringSubmatch(table, -1) {
		sev := cellText(row[1])
		// Skip the header row, which has no severity in the first cell.
		if sev == "" {
			continue
		}
		messages = append(messages, CompareMessage{
			Severity: strings.ToLower(sev),
			Path:     cellText(row[2]),
			Message:  cellText(row[3]),
		})
	}
	return messages
}

func cellText(s string) string {
	return strings.TrimSpace(html.UnescapeString(stripTags.ReplaceAllString(s, "")))
}

// compareFailure builds an error for the case where the JAR produced no
// comparison HTML, surfacing the most useful line from its output.
func compareFailure(stdout string, runErr error) error {
	for _, line := range strings.Split(stdout, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "Unable to resolve") ||
			strings.HasPrefix(t, "Unable to parse") ||
			strings.HasPrefix(t, "Error:") ||
			strings.Contains(t, "not found") ||
			strings.HasPrefix(t, "no -") {
			return fmt.Errorf("comparison failed: %s", t)
		}
	}
	if runErr != nil {
		return fmt.Errorf("comparison failed: %w", runErr)
	}
	return fmt.Errorf("comparison produced no output — check that both profiles resolve from the loaded IGs")
}
