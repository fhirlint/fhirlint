package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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

Configuration can be stored in fhirlint.yml or .fhirlint.yml in the project root.

HL7® FHIR® is a registered trademark of Health Level Seven International.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().String("config", "", "Config file (default: fhirlint.yml or .fhirlint.yml in project root)")
	rootCmd.PersistentFlags().String("jar", "", "Path to a local validator_cli.jar (overrides auto-download, also: FHIRLINT_JAR env var)")
	_ = viper.BindPFlag("config", rootCmd.PersistentFlags().Lookup("config"))
	_ = viper.BindPFlag("jar", rootCmd.PersistentFlags().Lookup("jar"))
	_ = viper.BindEnv("jar", "FHIRLINT_JAR")

	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(auditCmd)
	rootCmd.AddCommand(profilesCmd)
	rootCmd.AddCommand(versionCmd)
}

func initConfig() {
	if cfgFile, _ := rootCmd.PersistentFlags().GetString("config"); cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("fhirlint")
		viper.AddConfigPath(".")
		// Walk up to parent directories (like golangci-lint)
		viper.AddConfigPath("..")
		viper.AddConfigPath("../..")
	}

	viper.SetConfigType("yaml")
	viper.AutomaticEnv()

	// Silently ignore missing config — it's optional
	_ = viper.ReadInConfig()
}
