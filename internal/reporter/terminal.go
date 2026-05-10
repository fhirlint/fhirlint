package reporter

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/fhirlint/fhirlint/internal/validator"
)

var (
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	warningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	infoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	fileStyle    = lipgloss.NewStyle().Bold(true)
)

func Terminal(result *validator.Result, minSeverity string, showSuppressed bool) {
	fmt.Println(fileStyle.Render("▶ " + result.Label))

	filtered := filterIssues(result.Issues, minSeverity)

	if len(filtered) == 0 && len(result.Suppressed) == 0 {
		fmt.Println(successStyle.Render("  ✓ Valid"))
		fmt.Println()
		return
	}

	for _, issue := range filtered {
		var prefix string
		var style lipgloss.Style
		switch issue.Severity {
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
	}

	if showSuppressed {
		for _, issue := range result.Suppressed {
			fmt.Println(dimStyle.Render("  ↷ SUPP  ") + dimStyle.Render(issue.Message))
			if issue.Location != "" {
				fmt.Println(dimStyle.Render("           @ " + issue.Location))
			}
		}
	}

	if len(filtered) == 0 {
		fmt.Println(successStyle.Render("  ✓ Valid"))
	}
	fmt.Println()
}

func TerminalSummary(results []*validator.Result, minSeverity string) int {
	total, errCount, warnCount, suppCount := 0, 0, 0, 0
	for _, r := range results {
		for _, issue := range filterIssues(r.Issues, minSeverity) {
			total++
			switch issue.Severity {
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
	if suppCount > 0 {
		line += dimStyle.Render(fmt.Sprintf("  Suppressed: %d", suppCount))
	}
	fmt.Println(line)
	return total
}

func filterIssues(issues []validator.Issue, minSeverity string) []validator.Issue {
	order := map[string]int{"information": 0, "warning": 1, "error": 2}
	min := order[minSeverity]
	var out []validator.Issue
	for _, i := range issues {
		if order[i.Severity] >= min {
			out = append(out, i)
		}
	}
	return out
}
