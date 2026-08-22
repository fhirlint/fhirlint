package cmd

import (
	"fmt"
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
			verifySuffix(),
			validator.JARReleasesURL(),
		)
		if viper.IsSet("fhir-version") {
			fmt.Printf("fhir:      %s\n", validator.FHIRVersionName(viper.GetString("fhir-version")))
		} else {
			fmt.Printf("fhir:      %s (default)\n", validator.FHIRVersionName(defaultFHIRVersion))
		}
		printUpdateNotice()
	},
}

// verifySuffix annotates the validator version with how the JAR was verified.
// Silence would leave users unable to tell a verified JAR from one installed
// when the check could not be made (#260), and naming the method matters
// because a PGP signature and a release digest are not worth the same (#358).
func verifySuffix() string {
	method, verified, known := validator.JARVerification()
	switch {
	case !known:
		return "" // nothing recorded (older cache, or no JAR yet)
	case verified:
		return " (verified: " + method + ")"
	default:
		return " (NOT verified)"
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
