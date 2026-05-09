package cmd

import (
	"fmt"
	"os"

	"github.com/fhirlint/fhirlint/internal/validator"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update the cached HL7 FHIR validator JAR",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validator.UpdateJAR(); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			return err
		}
		return nil
	},
}
