package reporter

import (
	"fmt"
	"os"
	"strings"

	"github.com/fhirlint/fhirlint/internal/validator"
)

// Markdown writes a human-readable validation summary suitable for posting as a
// GitHub PR comment. When dest is empty the report is printed to stdout.
func Markdown(results []*validator.Result, minSeverity, dest string) error {
	report := buildMarkdownReport(results, minSeverity)
	if dest == "" {
		fmt.Print(report)
		return nil
	}
	return os.WriteFile(dest, []byte(report), 0600)
}

func buildMarkdownReport(results []*validator.Result, minSeverity string) string {
	var errCount, warnCount, validCount, suppCount int
	for _, r := range results {
		if r.Valid {
			validCount++
		}
		for _, iss := range filterIssues(r.Issues, minSeverity) {
			switch iss.Severity {
			case "error", "fatal":
				errCount++
			case "warning":
				warnCount++
			}
		}
		suppCount += len(r.Suppressed)
	}

	var b strings.Builder
	b.WriteString("## FHIR Validation Report\n\n")
	b.WriteString("| | |\n|---|---|\n")
	fmt.Fprintf(&b, "| Files | %d |\n", len(results))
	fmt.Fprintf(&b, "| ✅ Valid | %d |\n", validCount)
	fmt.Fprintf(&b, "| ❌ Errors | %d |\n", errCount)
	fmt.Fprintf(&b, "| ⚠️ Warnings | %d |\n", warnCount)
	b.WriteString("\n")

	// Per-file detail: only files that have issues at or above the threshold.
	// Fully valid files are omitted to keep the comment concise.
	for _, r := range results {
		issues := filterIssues(r.Issues, minSeverity)
		if len(issues) == 0 {
			continue
		}
		emoji := "⚠️"
		for _, iss := range issues {
			if iss.Severity == "error" || iss.Severity == "fatal" {
				emoji = "❌"
				break
			}
		}
		fmt.Fprintf(&b, "### %s %s\n\n", emoji, mdEscape(label(r)))
		b.WriteString("| Severity | Location | Message |\n|---|---|---|\n")
		for _, iss := range issues {
			fmt.Fprintf(&b, "| %s | %s | %s |\n",
				severityLabel(iss.Severity),
				mdEscape(iss.Location),
				mdEscape(iss.Message),
			)
		}
		b.WriteString("\n")
	}

	if suppCount > 0 {
		fmt.Fprintf(&b, "<details>\n<summary>Suppressed (%d)</summary>\n\n", suppCount)
		b.WriteString("| File | Severity | Location | Message | Reason |\n|---|---|---|---|---|\n")
		for _, r := range results {
			for _, iss := range r.Suppressed {
				fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n",
					mdEscape(label(r)),
					severityLabel(iss.Severity),
					mdEscape(iss.Location),
					mdEscape(iss.Message),
					mdEscape(iss.SuppressReason),
				)
			}
		}
		b.WriteString("\n</details>\n")
	}

	return b.String()
}

// label returns the human-readable source for a result, falling back to the
// filename when no label was set.
func label(r *validator.Result) string {
	if r.Label != "" {
		return r.Label
	}
	return r.Filename
}

func severityLabel(severity string) string {
	switch severity {
	case "error":
		return "ERROR"
	case "fatal":
		return "FATAL"
	case "warning":
		return "WARNING"
	default:
		return "INFO"
	}
}

// mdEscape makes a string safe to embed in a single Markdown table cell:
// pipes are escaped and newlines are flattened so they don't break the row.
func mdEscape(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.TrimSpace(s)
}
