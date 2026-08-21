package reporter

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/fhirlint/fhirlint/internal/explain"
	"github.com/fhirlint/fhirlint/internal/validator"
	"github.com/muesli/termenv"
)

func DisableColors() {
	lipgloss.DefaultRenderer().SetColorProfile(termenv.Ascii)
}

var (
	fatalStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Bold(true)
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	warningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	infoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	fileStyle    = lipgloss.NewStyle().Bold(true)
)

// issueHeadline is the text shown next to the severity marker.
//
// A redacted finding has had its message replaced by a placeholder, which on
// its own leaves the line with nothing to act on. Its message ID takes over
// that slot instead: it is the handle a reader searches, suppresses and runs
// `fhirlint explain` by. The explain hint further down only appears for IDs
// fhirlint can explain, so it cannot be relied on to surface the ID.
func issueHeadline(issue validator.Issue) string {
	if issue.Redacted && issue.MessageID != "" {
		return issue.Message + " " + issue.MessageID
	}
	return issue.Message
}

func Terminal(result *validator.Result, minSeverity string, showSuppressed, quiet, showSource bool) {
	filtered := filterIssues(result.Issues, minSeverity)

	// Under --quiet, skip files with no active issues (including those with only
	// suppressed issues). The summary line is always printed by TerminalSummary.
	if quiet && len(filtered) == 0 {
		return
	}

	label := result.Label
	if result.Cached {
		label += dimStyle.Render(" ↩ cached")
	}
	fmt.Println(fileStyle.Render("▶ " + label))

	if len(filtered) == 0 && len(result.Suppressed) == 0 {
		fmt.Println(successStyle.Render("  ✓ Valid"))
		fmt.Println()
		return
	}

	hinted := map[string]bool{}
	for _, issue := range filtered {
		prefix, style := severityPrefix(issue.Severity)
		fmt.Println(style.Render(prefix) + issueHeadline(issue))
		// A finding shown as a warning that the validator called an error is
		// confusing without this: the reader has no way to tell a re-levelled
		// finding from one the validator reported that way.
		if issue.OriginalSeverity != "" {
			fmt.Println(dimStyle.Render("           ↕ reported as " + issue.OriginalSeverity))
		}
		if issue.Location != "" {
			fmt.Println(dimStyle.Render("           @ " + issue.Location))
		}
		if showSource {
			// Resolved against SourcePath, not Filename: for preprocessed input
			// the coordinates belong to the temp copy that was validated.
			_, line, col := parseLocationString(issue.Location)
			for _, l := range sourceSnippet(result.SourcePath, line, col) {
				fmt.Println(dimStyle.Render("         " + l))
			}
		}
		// Surface a hint once per message ID when we can explain it.
		if issue.MessageID != "" && !hinted[issue.MessageID] && explain.Known(issue.MessageID) {
			hinted[issue.MessageID] = true
			fmt.Println(dimStyle.Render("           ↳ Run: fhirlint explain " + issue.MessageID))
		}
	}

	if showSuppressed {
		for _, issue := range result.Suppressed {
			fmt.Println(dimStyle.Render("  ↷ SUPP  ") + dimStyle.Render(issueHeadline(issue)))
			if issue.Location != "" {
				fmt.Println(dimStyle.Render("           @ " + issue.Location))
			}
			if issue.SuppressReason != "" {
				fmt.Println(dimStyle.Render("           ↳ " + issue.SuppressReason))
			}
		}
	}

	if len(filtered) == 0 {
		fmt.Println(successStyle.Render("  ✓ Valid"))
	}
	fmt.Println()
}

func TerminalSummary(results []*validator.Result, minSeverity string) int {
	total, fatalCount, errCount, warnCount, suppCount := 0, 0, 0, 0, 0
	skippedChecks := 0
	for _, r := range results {
		for _, issue := range filterIssues(r.Issues, minSeverity) {
			total++
			switch issue.Severity {
			case "fatal":
				fatalCount++
			case "error":
				errCount++
			case "warning":
				warnCount++
			}
		}
		suppCount += len(r.Suppressed)
		// Counted over every issue, not the severity-filtered ones: the
		// validator reports a skipped check as a hint, so the default filter
		// would hide exactly the message that says a check did not run.
		// Suppressed issues are already out of r.Issues, so a project that
		// deliberately suppressed the hint is not nagged about it.
		for _, issue := range r.Issues {
			if validator.IsSkippedCheck(issue.MessageID) {
				skippedChecks++
			}
		}
	}
	validCount := 0
	for _, r := range results {
		if r.Valid {
			validCount++
		}
	}

	sep := strings.Repeat("─", 40)
	fmt.Println(dimStyle.Render(sep))
	line := fmt.Sprintf("Files: %d  Valid: %s  Errors: %s  Warnings: %s",
		len(results),
		successStyle.Render(fmt.Sprintf("%d", validCount)),
		errorStyle.Render(fmt.Sprintf("%d", errCount)),
		warningStyle.Render(fmt.Sprintf("%d", warnCount)),
	)
	if fatalCount > 0 {
		line += fatalStyle.Render(fmt.Sprintf("  Fatal: %d", fatalCount))
	}
	if suppCount > 0 {
		line += dimStyle.Render(fmt.Sprintf("  Suppressed: %d", suppCount))
	}
	fmt.Println(line)
	if skippedChecks > 0 {
		// A run where a class of checks did not execute is not the same as a
		// clean run, and the hint that says so is easy to filter away.
		fmt.Println(warningStyle.Render(fmt.Sprintf(
			"Checks skipped: %d — too many codes to check against a code system. See --codesystem-size-limit.",
			skippedChecks)))
	}
	return total
}

func filterIssues(issues []validator.Issue, minSeverity string) []validator.Issue {
	order := map[string]int{"information": 0, "warning": 1, "error": 2, "fatal": 3}
	min := order[minSeverity]
	var out []validator.Issue
	for _, i := range issues {
		if order[i.Severity] >= min {
			out = append(out, i)
		}
	}
	return out
}
