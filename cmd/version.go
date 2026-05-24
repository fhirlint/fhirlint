package cmd

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/fhirlint/fhirlint/internal/validator"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
		if viper.IsSet("fhir-version") {
			fmt.Printf("fhir:      %s\n", fhirVersionName(viper.GetString("fhir-version")))
		} else {
			fmt.Printf("fhir:      %s (default)\n", fhirVersionName(defaultFHIRVersion))
		}
		if newer := validator.CheckForUpdate(); newer != "" {
			fmt.Fprintf(os.Stderr, "\nA new validator version (%s) is available. Run: fhirlint update\n", newer)
		}
	},
}

func fhirVersionName(v string) string {
	switch v {
	case "4.0.1":
		return "R4"
	case "4.3.0":
		return "R4B"
	case "5.0.0":
		return "R5"
	default:
		return v
	}
}

// version is injected at release build time via GoReleaser ldflags
// (-X github.com/fhirlint/fhirlint/cmd.version={{ .Tag }}). It is empty for
// plain `go build`/`go install` builds, which fall back to debug.ReadBuildInfo.
var version string

func fhirlintVersion() string {
	if version != "" {
		return version
	}
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
