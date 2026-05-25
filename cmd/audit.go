package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/fhirlint/fhirlint/internal/validator"
	"github.com/spf13/cobra"
)

var flagAuditFormat string

var auditCmd = &cobra.Command{
	Use:          "audit",
	Short:        "Check the validator JAR for updates and known security advisories",
	RunE:         runAudit,
	SilenceUsage: true,
}

func init() {
	auditCmd.Flags().StringVarP(&flagAuditFormat, "format", "f", "terminal",
		"Output format: terminal, json")
	_ = auditCmd.RegisterFlagCompletionFunc("format", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"terminal", "json"}, cobra.ShellCompDirectiveNoFileComp
	})
}

// auditJSON is the machine-readable shape emitted by `audit --format json`,
// consumed by the JAR security-monitor workflow.
type auditJSON struct {
	CurrentVersion string              `json:"currentVersion"`
	LatestVersion  string              `json:"latestVersion"`
	Outdated       bool                `json:"outdated"`
	AdvisoryCount  int                 `json:"advisoryCount"`
	Affecting      []auditAdvisoryJSON `json:"affecting"`
	VersionError   string              `json:"versionError,omitempty"`
	AdvisoryError  string              `json:"advisoryError,omitempty"`
}

type auditAdvisoryJSON struct {
	GHSAID   string `json:"ghsaId"`
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
	URL      string `json:"url"`
}

func runAudit(_ *cobra.Command, _ []string) error {
	report := validator.Audit()

	if flagAuditFormat == "json" {
		return printAuditJSON(report)
	}

	issues := 0

	fmt.Println("Validator JAR")
	fmt.Printf("  current:  %s\n", report.CurrentVersion)
	if report.VersionError != "" {
		fmt.Fprintf(os.Stderr, "  ! could not fetch latest version: %s\n", report.VersionError)
	} else {
		fmt.Printf("  latest:   %s\n", report.LatestVersion)
		switch {
		case report.CurrentVersion == "unknown":
			fmt.Fprintln(os.Stderr, "  ✗ JAR not installed — run: fhirlint update")
			issues++
		case report.IsOutdated:
			fmt.Fprintln(os.Stderr, "  ✗ outdated — run: fhirlint update")
			issues++
		default:
			fmt.Println("  ✓ up to date")
		}
	}

	fmt.Println()
	fmt.Println("Security advisories (hapifhir/org.hl7.fhir.core)")
	if report.AdvisoryError != "" {
		fmt.Fprintf(os.Stderr, "  ! could not reach advisory API: %s\n", report.AdvisoryError)
	} else if len(report.Advisories) == 0 {
		fmt.Println("  ✓ no published advisories found")
	} else {
		affecting := report.AffectingAdvisories()
		if len(affecting) == 0 {
			fmt.Printf("  ✓ %d advisory/advisories published, none affect your version (%s)\n",
				len(report.Advisories), report.CurrentVersion)
		} else {
			for _, a := range affecting {
				fmt.Fprintf(os.Stderr, "  ✗ [%s] %s (%s)\n     %s\n",
					a.Severity, a.Summary, a.GHSAID, a.HTMLURL)
				issues++
			}
			fmt.Printf("  (%d advisory/advisories total, %d not affecting your version)\n",
				len(report.Advisories), len(report.Advisories)-len(affecting))
		}
	}

	fmt.Println()
	if issues > 0 {
		fmt.Fprintf(os.Stderr, "%d issue(s) found.\n", issues)
		os.Exit(1)
	}
	fmt.Println("No issues found.")
	return nil
}

// printAuditJSON emits the audit report as JSON and always exits 0, so callers
// (e.g. the security-monitor workflow) can parse the result rather than rely on
// an exit code that conflates "outdated" with "security advisory".
func printAuditJSON(report validator.AuditReport) error {
	affecting := make([]auditAdvisoryJSON, 0)
	for _, a := range report.AffectingAdvisories() {
		affecting = append(affecting, auditAdvisoryJSON{
			GHSAID:   a.GHSAID,
			Severity: a.Severity,
			Summary:  a.Summary,
			URL:      a.HTMLURL,
		})
	}

	out := auditJSON{
		CurrentVersion: report.CurrentVersion,
		LatestVersion:  report.LatestVersion,
		Outdated:       report.IsOutdated,
		AdvisoryCount:  len(report.Advisories),
		Affecting:      affecting,
		VersionError:   report.VersionError,
		AdvisoryError:  report.AdvisoryError,
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
