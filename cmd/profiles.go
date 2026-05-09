package cmd

import (
	"github.com/fhirlint/fhirlint/internal/profiles"
	"github.com/spf13/cobra"
)

var profilesCmd = &cobra.Command{
	Use:   "profiles",
	Short: "List built-in profile aliases",
	Run: func(cmd *cobra.Command, args []string) {
		profiles.List()
	},
}
