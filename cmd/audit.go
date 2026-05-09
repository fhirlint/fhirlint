package cmd

import (
	"fmt"
	"os"

	"github.com/fhirlint/fhirlint/internal/validator"
	"github.com/spf13/cobra"
)

var auditCmd = &cobra.Command{
	Use:          "audit",
	Short:        "Check the validator JAR for updates and known security advisories",
	RunE:         runAudit,
	SilenceUsage: true,
}

func runAudit(_ *cobra.Command, _ []string) error {
	report := validator.Audit()
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
		for _, a := range report.Advisories {
			fmt.Fprintf(os.Stderr, "  ✗ [%s] %s (%s)\n     %s\n", a.Severity, a.Summary, a.GHSAID, a.HTMLURL)
			issues++
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
