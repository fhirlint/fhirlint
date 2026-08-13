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
		fmt.Printf("validator: %s%s  (%s)\n",
			validator.ValidatorVersion(),
			checksumSuffix(),
			validator.JARReleasesURL(),
		)
		if viper.IsSet("fhir-version") {
			fmt.Printf("fhir:      %s\n", validator.FHIRVersionName(viper.GetString("fhir-version")))
		} else {
			fmt.Printf("fhir:      %s (default)\n", validator.FHIRVersionName(defaultFHIRVersion))
		}
		if newer := validator.CheckForUpdate(); newer != "" {
			fmt.Fprintf(os.Stderr, "\nA new validator version (%s) is available. Run: fhirlint update\n", newer)
		}
	},
}

// checksumSuffix annotates the validator version with how its checksum
// verification went. Silence would leave users unable to tell a verified JAR
// from one installed when the checksum could not be fetched (#260).
func checksumSuffix() string {
	verified, known := validator.JARChecksumVerified()
	switch {
	case !known:
		return "" // nothing recorded (older cache, or no JAR yet)
	case verified:
		return " (checksum verified)"
	default:
		return " (checksum NOT verified)"
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
