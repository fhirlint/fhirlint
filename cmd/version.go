package cmd

import (
	"fmt"
	"runtime/debug"

	"github.com/fhirlint/fhirlint/internal/validator"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show fhirlint and validator JAR versions",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("fhirlint:  %s\n", fhirlintVersion())
		fmt.Printf("validator: %s  (%s)\n",
			validator.ValidatorVersion(),
			validator.JARReleasesURL(),
		)
	},
}

func fhirlintVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	// During local development: show VCS commit if available
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && len(s.Value) >= 7 {
			return "dev-" + s.Value[:7]
		}
	}
	return "dev"
}
