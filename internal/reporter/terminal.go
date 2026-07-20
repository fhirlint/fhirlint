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
		var prefix string
		var style lipgloss.Style
		switch issue.Severity {
		case "fatal":
			prefix = "  ✗ FATAL  "
			style = fatalStyle
		case "error":
			prefix = "  ✗ ERROR  "
			style = errorStyle
		case "warning":
			prefix = "  ⚠ WARN   "
			style = warningStyle
		default:
			prefix = "  ℹ INFO   "
			style = infoStyle
		}
		fmt.Println(style.Render(prefix) + issue.Message)
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
			fmt.Println(dimStyle.Render("  ↷ SUPP  ") + dimStyle.Render(issue.Message))
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
