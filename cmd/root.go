package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "fhirlint",
	Short: "Lightweight FHIR validator CLI",
	Long: `fhirlint validates FHIR resources against profiles and implementation guides.

It wraps the official HL7 FHIR Validator with a better developer experience:
  - Auto-downloads the validator JAR on first use
  - Accepts files, directories, stdin, and HTTP URLs
  - Outputs to terminal, JSON, or HTML
  - Ships with German profile aliases (KBV, MII, DiGA)

HL7® FHIR® is a registered trademark of Health Level Seven International.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(auditCmd)
	rootCmd.AddCommand(profilesCmd)
	rootCmd.AddCommand(versionCmd)
}
