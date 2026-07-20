package cmd

import (
	"fmt"
	"os"

	"github.com/fhirlint/fhirlint/internal/configcheck"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage fhirlint configuration",
}

var configCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Validate fhirlint.yml for unknown keys and invalid values",
	Long: `Validate the fhirlint.yml config file for unknown keys, invalid enum
values, and type errors.

Exits non-zero when any issues are found.`,
	RunE:         runConfigCheck,
	SilenceUsage: true,
}

var configSchemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "Print the JSON Schema for fhirlint.yml",
	Long: `Print the JSON Schema describing fhirlint.yml.

The schema is generated from the same key definitions ` + "`config check`" + ` validates
against, so the two cannot disagree. Point your editor at the published copy
with a modeline at the top of fhirlint.yml:

  # yaml-language-server: $schema=` + configcheck.SchemaID + `
`,
	RunE:         runConfigSchema,
	SilenceUsage: true,
}

func runConfigSchema(cmd *cobra.Command, _ []string) error {
	data, err := configcheck.Schema()
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return err
}

func init() {
	configCmd.AddCommand(configCheckCmd)
	configCmd.AddCommand(configSchemaCmd)
	rootCmd.AddCommand(configCmd)
}

func runConfigCheck(_ *cobra.Command, _ []string) error {
	path := viper.ConfigFileUsed()
	if path == "" {
		// viper didn't find a config file — search explicitly so we can report
		// errors in a file the user may have just created.
		for _, name := range []string{"fhirlint.yml", ".fhirlint.yml"} {
			if _, err := os.Stat(name); err == nil {
				path = name
				break
			}
		}
	}
	if path == "" {
		fmt.Fprintln(os.Stderr, "No fhirlint.yml found in the current directory.")
		fmt.Fprintln(os.Stderr, "Run: fhirlint init")
		return fmt.Errorf("config file not found")
	}

	issues, err := configcheck.Check(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	if len(issues) == 0 {
		fmt.Printf("✓ %s is valid\n", path)
		return nil
	}

	for _, issue := range issues {
		fmt.Fprintf(os.Stderr, "✗ %s:%s\n", path, issue)
	}
	return fmt.Errorf("%s: %d issue(s) found", path, len(issues))
}
