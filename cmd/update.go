package cmd

import (
	"fmt"
	"os"

	"github.com/fhirlint/fhirlint/internal/validator"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update the cached HL7 FHIR validator JAR",
	Long: `Update the cached HL7 FHIR validator JAR.

Downloads the latest upstream release, or the release named by
--validator-version. Use the latter to move a pin deliberately:

  fhirlint update --validator-version 6.9.12`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validator.UpdateJAR(viper.GetString("validator-version")); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			return err
		}
		return nil
	},
}
