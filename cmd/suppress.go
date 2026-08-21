package cmd

import (
	"fmt"
	"os"

	"github.com/fhirlint/fhirlint/internal/advisor"
	"github.com/fhirlint/fhirlint/internal/suppress"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var suppressCmd = &cobra.Command{
	Use:   "suppress",
	Short: "Work with suppression rules",
}

var suppressExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export the suppression rules as a validator advisor file",
	Long: `Convert the suppression rules in fhirlint.yml into an advisor file for the
HL7 validator's -advisor-file parameter.

Suppression rules are applied by fhirlint, after the JAR has produced its
output, so they only hold for runs that go through fhirlint. An advisor file is
read by the validator itself and by the IG Publisher, so exporting one lets a
raw validator run honour the same accepted findings:

  fhirlint suppress export -o advisor.json
  java -jar validator_cli.jar patient.json -advisor-file advisor.json

The conversion is lossy. The advisor format filters messages by id and element
path and knows nothing else, so rules that match the message text, narrow by
severity, or name a bare constraint key cannot be expressed. Every rule that is
dropped is reported on stderr with the reason; --strict turns that into a
non-zero exit for a CI check that the two stay in step.`,
	Args:         cobra.NoArgs,
	RunE:         runSuppressExport,
	SilenceUsage: true,
}

var (
	flagSuppressExportOut    string
	flagSuppressExportStrict bool
)

func init() {
	suppressExportCmd.Flags().StringVarP(&flagSuppressExportOut, "output", "o", "",
		"Write the advisor file to this path (default: stdout)")
	suppressExportCmd.Flags().BoolVar(&flagSuppressExportStrict, "strict", false,
		"Exit non-zero when any rule could not be exported")

	suppressCmd.AddCommand(suppressExportCmd)
	rootCmd.AddCommand(suppressCmd)
}

func runSuppressExport(cmd *cobra.Command, _ []string) error {
	rules, err := configSuppressRules()
	if err != nil {
		return err
	}

	file, dropped := advisor.Convert(rules)

	// Rules fhirlint holds that the format cannot represent at all, as opposed
	// to individual rules it could not translate. Counted separately because
	// the answer is different: these need a decision about scope, not a rewrite
	// of the rule.
	scoped, err := overrideSuppressCount()
	if err != nil {
		return err
	}
	relevelled, err := severityOverrideCount()
	if err != nil {
		return err
	}

	data, err := advisor.Marshal(file)
	if err != nil {
		return err
	}

	if flagSuppressExportOut == "" {
		if _, err := cmd.OutOrStdout().Write(data); err != nil {
			return err
		}
	} else {
		if err := os.WriteFile(flagSuppressExportOut, data, 0o600); err != nil {
			return fmt.Errorf("writing %s: %w", flagSuppressExportOut, err)
		}
	}

	reportExport(len(rules), len(file.Suppress), dropped, scoped, relevelled)

	if flagSuppressExportStrict && (len(dropped) > 0 || scoped > 0 || relevelled > 0) {
		return &exitErr{code: 1, err: fmt.Errorf(
			"--strict: %d rule(s) could not be exported", len(dropped)+scoped+relevelled)}
	}
	return nil
}

// reportExport writes the summary to stderr, never stdout: stdout may be the
// advisor file itself.
func reportExport(ruleCount, entryCount int, dropped []advisor.Dropped, scoped, relevelled int) {
	exported := ruleCount - len(dropped)
	fmt.Fprintf(os.Stderr, "Exported %d of %d suppression rule(s) as %d advisor entr%s.\n",
		exported, ruleCount, entryCount, plural(entryCount, "y", "ies"))

	if len(dropped) > 0 {
		fmt.Fprintf(os.Stderr, "\nNot exported — no advisor equivalent:\n")
		for _, d := range dropped {
			fmt.Fprintf(os.Stderr, "  %s\n      %s\n", d.Rule.Raw, d.Reason)
		}
	}
	if scoped > 0 {
		fmt.Fprintf(os.Stderr, "\nNot exported — %d rule(s) under `overrides:`: an advisor file applies to the whole run, so per-file scope would be lost.\n", scoped)
	}
	if relevelled > 0 {
		fmt.Fprintf(os.Stderr, "\nNot exported — %d `severity-override:` rule(s): the advisor can only suppress a message, not re-level it.\n", relevelled)
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// configSuppressRules reads the top-level suppress rules from the config file.
// Deliberately not the --suppress flag: an advisor file is a project artefact
// checked in next to fhirlint.yml, and exporting an ad-hoc command-line rule
// into it would put something in the file that no config explains.
func configSuppressRules() ([]suppress.Rule, error) {
	if !viper.IsSet("suppress") {
		return nil, nil
	}
	rules, err := parseSuppressFromConfig(viper.Get("suppress"))
	if err != nil {
		return nil, err
	}
	return rules, nil
}

func overrideSuppressCount() (int, error) {
	overrides, err := loadOverrides()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, ov := range overrides {
		n += len(ov.Suppress)
	}
	return n, nil
}

func severityOverrideCount() (int, error) {
	rules, err := buildSeverityOverrides()
	if err != nil {
		return 0, err
	}
	return len(rules), nil
}
