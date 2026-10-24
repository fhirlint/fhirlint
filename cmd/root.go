package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fhirlint/fhirlint/internal/input"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var rootCmd = &cobra.Command{
	Use:           "fhirlint",
	Short:         "Lightweight FHIR validator CLI",
	SilenceErrors: true,
	Long: `fhirlint validates FHIR resources against profiles and implementation guides.

It wraps the official HL7 FHIR Validator with a better developer experience:
  - Auto-downloads the validator JAR on first use
  - Accepts files, directories, stdin, and HTTP URLs
  - Outputs to terminal, JSON, or HTML
  - Ships with German profile aliases (KBV, MII, ISiK, DiGA)

Configuration can be stored in fhirlint.yml or .fhirlint.yml in the project root.

HL7® FHIR® is a registered trademark of Health Level Seven International.`,
}

// exitErr lets a command request a specific process exit code from its RunE.
// Commands that don't use it fall back to exit code 1 on any error.
type exitErr struct {
	code int
	err  error
}

func (e *exitErr) Error() string { return e.err.Error() }
func (e *exitErr) Unwrap() error { return e.err }

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		code := 1
		var ee *exitErr
		if errors.As(err, &ee) {
			code = ee.code
		}
		if !errors.Is(err, errValidationFailed) {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		os.Exit(code)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().String("config", "", "Config file (default: fhirlint.yml or .fhirlint.yml in project root)")
	rootCmd.PersistentFlags().String("jar", "", "Path to a local validator_cli.jar (overrides auto-download, also: FHIRLINT_JAR env var)")
	rootCmd.PersistentFlags().String("validator-version", "",
		"Pin the auto-downloaded validator JAR to an upstream release, e.g. 6.9.12 (default: latest)")
	_ = viper.BindPFlag("config", rootCmd.PersistentFlags().Lookup("config"))
	_ = viper.BindPFlag("jar", rootCmd.PersistentFlags().Lookup("jar"))
	_ = viper.BindEnv("jar", "FHIRLINT_JAR")
	_ = viper.BindPFlag("validator-version", rootCmd.PersistentFlags().Lookup("validator-version"))
	_ = viper.BindEnv("validator-version", "FHIRLINT_VALIDATOR_VERSION")

	rootCmd.Version = fhirlintVersion()

	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(auditCmd)
	rootCmd.AddCommand(profilesCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(explainCmd)
	rootCmd.AddCommand(diffCmd)
	rootCmd.AddCommand(qualifyCmd)
	rootCmd.AddCommand(statsCmd)
	rootCmd.AddCommand(coverageCmd)
	rootCmd.AddCommand(fhirpathCmd)
	rootCmd.AddCommand(compareCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(packagesCmd)
}

func initConfig() {
	viper.SetConfigType("yaml")
	viper.AutomaticEnv()

	if cfgFile, _ := rootCmd.PersistentFlags().GetString("config"); cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		found := discoverConfig()
		if found == "" {
			return // no config — it's optional
		}
		viper.SetConfigFile(found)
	}

	// Silently ignore an unreadable config here; `fhirlint config check`
	// reports what is wrong with it.
	_ = viper.ReadInConfig()
}

// discoverConfig finds the fhirlint.yml viper would pick up in the working
// directory or one of the two above it, and refuses one that is a symbolic
// link out of the directory it was found in (#500). A repository could
// otherwise make fhirlint read a file elsewhere on the machine as its config,
// and `config check` quote it. A file passed with --config stays trusted.
//
// It follows viper's own search (searchInPath in viper 1.21: each of
// SupportedExts in order, then the bare name, since a config type is set) but
// only stats, so a link to something like /dev/zero is refused before it is
// read.
func discoverConfig() string {
	// Walk up to parent directories (like golangci-lint)
	for _, dir := range []string{".", "..", "../.."} {
		abs, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		names := make([]string, 0, len(viper.SupportedExts)+1)
		for _, ext := range viper.SupportedExts {
			names = append(names, "fhirlint."+ext)
		}
		for _, name := range append(names, "fhirlint") {
			p := filepath.Join(abs, name)
			if st, err := os.Stat(p); err != nil || st.IsDir() {
				continue
			}
			if !input.Inside(abs, p) {
				fmt.Fprintf(os.Stderr, "warn: ignoring %s — it is a symbolic link that leads outside its directory; pass it with --config to use it anyway\n", p)
				return ""
			}
			return p
		}
	}
	return ""
}
