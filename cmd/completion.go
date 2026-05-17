package cmd

import "github.com/spf13/cobra"

// registerFlagCompletions adds tab-completion functions for flag values so that
// shells can offer valid choices when the user presses Tab after a flag.
// Called from validate.go's init() after all flags are registered.
//
// The `completion` subcommand itself (bash/zsh/fish/powershell) is provided
// automatically by cobra — no manual setup needed.
func registerFlagCompletions(cmd *cobra.Command) {
	noFile := cobra.ShellCompDirectiveNoFileComp

	complete := func(flag string, values []string) {
		_ = cmd.RegisterFlagCompletionFunc(flag, func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			return values, noFile
		})
	}

	complete("fhir-version", []string{"4.0.1", "4.3.0", "5.0.0"})
	complete("fail-on", []string{"error", "warning", "information", "never"})
	complete("best-practice", []string{"ignore", "hint", "warning", "error"})
	complete("severity", []string{"information", "warning", "error"})
	complete("format", []string{"terminal", "json", "html", "junit", "sarif"})
	complete("watch", []string{"single", "all"})
}
