package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/fhirlint/fhirlint/internal/input"
	"github.com/fhirlint/fhirlint/internal/validator"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	flagFHIRPathFormat      string
	flagFHIRPathFHIRVersion string
)

var fhirpathCmd = &cobra.Command{
	Use:   "fhirpath <expression> [file]",
	Short: "Evaluate a FHIRPath expression against a resource",
	Long: `Evaluate a FHIRPath expression against a FHIR resource and print the result.

FHIRPath is the expression language behind FHIR invariants/constraints (dom-6,
ele-1, …), slicing discriminators, and search parameters. When an invariant
fails and it is unclear why, evaluate sub-expressions of the rule against your
real resource to find where it returns false.

The resource is read from a file argument or stdin (JSON and XML auto-detected,
like validate). Evaluation is an inspection aid, not a pass/fail gate: an empty
or false result still exits 0; only a malformed expression, an unparseable
resource, or a tool failure exits 2.

Terminology is disabled for speed and offline use, so expressions that need a
terminology server (e.g. memberOf) are not supported.

Examples:
  fhirlint fhirpath "Patient.name.given" patient.json
  cat patient.json | fhirlint fhirpath "Observation.value.exists()"
  fhirlint fhirpath "identifier.where(system='http://fhir.de/sid/gkv/kvid-10')" patient.json`,
	Args:         cobra.RangeArgs(1, 2),
	RunE:         runFHIRPath,
	SilenceUsage: true,
}

func init() {
	fhirpathCmd.Flags().StringVarP(&flagFHIRPathFormat, "format", "f", "terminal",
		"Output format: terminal, json")
	fhirpathCmd.Flags().StringVar(&flagFHIRPathFHIRVersion, "fhir-version", defaultFHIRVersion,
		"FHIR version context ("+validator.FHIRVersionList()+")")

	noFile := cobra.ShellCompDirectiveNoFileComp
	_ = fhirpathCmd.RegisterFlagCompletionFunc("format", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"terminal", "json"}, noFile
	})
	_ = fhirpathCmd.RegisterFlagCompletionFunc("fhir-version", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return validator.FHIRVersionIDs(), noFile
	})
}

func runFHIRPath(_ *cobra.Command, args []string) error {
	if flagFHIRPathFormat != "terminal" && flagFHIRPathFormat != "json" {
		return &exitErr{code: 2, err: fmt.Errorf("unknown format %q — use: terminal, json", flagFHIRPathFormat)}
	}

	expr := args[0]
	arg := ""
	if rest := args[1:]; len(rest) > 0 {
		arg = rest[0]
	}

	in, err := input.Resolve(arg, "", 0)
	if err != nil {
		return &exitErr{code: 2, err: err}
	}
	defer in.Cleanup()
	if in.Source == input.SourceDir {
		return &exitErr{code: 2, err: fmt.Errorf("fhirpath evaluates a single resource — %q is a directory", in.Label)}
	}

	result, err := validator.RunFHIRPath(expr, in.Path, validator.FHIRPathOptions{
		FHIRVersion:      flagFHIRPathFHIRVersion,
		JARPath:          viper.GetString("jar"),
		ValidatorVersion: viper.GetString("validator-version"),
		Timeout:          5 * time.Minute,
	})
	if err != nil {
		return &exitErr{code: 2, err: err}
	}

	if flagFHIRPathFormat == "json" {
		return printFHIRPathJSON(result)
	}
	printFHIRPathTerminal(result)
	return nil
}

// printFHIRPathTerminal prints a single result item plainly, multiple items
// indexed, and an empty result as an explicit "(empty)" so "no match" is
// distinguishable from an error.
func printFHIRPathTerminal(r *validator.FHIRPathResult) {
	switch {
	case r.Empty():
		fmt.Println("(empty)")
	case len(r.Items) == 1:
		fmt.Println(r.Items[0])
	default:
		for i, item := range r.Items {
			fmt.Printf("[%d] %s\n", i, item)
		}
	}
}

func printFHIRPathJSON(r *validator.FHIRPathResult) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return &exitErr{code: 2, err: err}
	}
	fmt.Println(string(data))
	return nil
}
