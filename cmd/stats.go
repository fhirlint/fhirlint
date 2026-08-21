package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/fhirlint/fhirlint/internal/input"
	"github.com/fhirlint/fhirlint/internal/stats"
	"github.com/fhirlint/fhirlint/internal/validator"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	flagStatsFormat      []string
	flagStatsOutput      string
	flagStatsNoValidate  bool
	flagStatsFHIRVersion string
	flagStatsExclude     []string
)

var statsCmd = &cobra.Command{
	Use:   "stats [path]",
	Short: "Summarise resource types, profiles, and validation status of a dataset",
	Long: `Summarise a FHIR dataset: how many resources of each type exist, which
profiles they declare, and an aggregate validation summary.

Resource-type and profile counts are gathered offline by parsing each file
(.json, .jsonl, .ndjson, .xml). The validation summary runs the validator; skip it with
--no-validate for an instant, offline structural overview.

With no path, the current directory is used.`,
	Args:         cobra.MaximumNArgs(1),
	RunE:         runStats,
	SilenceUsage: true,
}

func init() {
	statsCmd.Flags().StringArrayVarP(&flagStatsFormat, "format", "f", []string{"terminal"},
		"Output format: terminal, json (repeatable)")
	statsCmd.Flags().StringVarP(&flagStatsOutput, "output", "o", "",
		"Output file for json (stdout if omitted)")
	statsCmd.Flags().BoolVar(&flagStatsNoValidate, "no-validate", false,
		"Skip the validation summary (structural counts only, fully offline)")
	statsCmd.Flags().StringVar(&flagStatsFHIRVersion, "fhir-version", defaultFHIRVersion,
		"FHIR version for the validation summary ("+validator.FHIRVersionList()+")")
	statsCmd.Flags().StringArrayVar(&flagStatsExclude, "exclude", nil,
		"Glob pattern to exclude (repeatable; also reads .fhirlintignore)")

	noFile := cobra.ShellCompDirectiveNoFileComp
	_ = statsCmd.RegisterFlagCompletionFunc("format", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"terminal", "json"}, noFile
	})
	_ = statsCmd.RegisterFlagCompletionFunc("fhir-version", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return validator.FHIRVersionIDs(), noFile
	})
}

func runStats(_ *cobra.Command, args []string) error {
	arg := "."
	if len(args) == 1 {
		arg = args[0]
	}

	in, err := input.Resolve(arg, "", 0)
	if err != nil {
		return &exitErr{code: 2, err: err}
	}

	excludePatterns := append([]string{}, flagStatsExclude...)
	if ignore, ierr := loadIgnoreFile(".fhirlintignore"); ierr != nil {
		fmt.Fprintf(os.Stderr, "warning: reading .fhirlintignore: %v\n", ierr)
	} else {
		excludePatterns = append(excludePatterns, ignore...)
	}

	paths, err := collectFHIRPaths(in, excludePatterns)
	if err != nil {
		return &exitErr{code: 2, err: fmt.Errorf("collecting files: %w", err)}
	}
	if len(paths) == 0 {
		return &exitErr{code: 2, err: fmt.Errorf("no FHIR files (%s) found in %q", input.ExtensionList(), arg)}
	}

	var resources []stats.Resource
	// Formats fhirlint cannot read are counted separately and reported. They
	// are valid input for the validator, so they are in `paths` — but counting
	// them as zero resources without saying so would understate a dataset and
	// look like a miscount (#341).
	unreadable := map[string]int{}
	for _, p := range paths {
		if ft, known := input.LookupFileType(p); known && ft.Kind == input.KindValidatorOnly {
			unreadable[ft.Parser]++
			continue
		}
		rs, perr := stats.ParseFile(p)
		if perr != nil {
			return &exitErr{code: 2, err: fmt.Errorf("reading %s: %w", p, perr)}
		}
		resources = append(resources, rs...)
	}
	for parser, n := range unreadable {
		fmt.Fprintf(os.Stderr, "note: %d %s file(s) not counted — the validator reads them, fhirlint does not\n",
			n, parser)
	}

	report := stats.Compute(resources)

	if !flagStatsNoValidate {
		summary, verr := validationSummary(paths)
		if verr != nil {
			return &exitErr{code: 2, err: fmt.Errorf("validation summary: %w", verr)}
		}
		report.Validation = summary
	}

	for _, format := range flagStatsFormat {
		switch strings.ToLower(format) {
		case "terminal":
			fmt.Print(stats.Terminal(report))
		case "json":
			if err := stats.JSON(report, statsOutputFile("json")); err != nil {
				return &exitErr{code: 2, err: fmt.Errorf("json output: %w", err)}
			}
		default:
			return &exitErr{code: 2, err: fmt.Errorf("unknown format %q — use: terminal, json", format)}
		}
	}
	return nil
}

func validationSummary(paths []string) (*stats.ValidationSummary, error) {
	results, err := validator.RunMultiple(paths, validator.Options{
		FHIRVersion:      flagStatsFHIRVersion,
		JARPath:          viper.GetString("jar"),
		ValidatorVersion: viper.GetString("validator-version"),
	})
	if err != nil {
		return nil, err
	}

	s := &stats.ValidationSummary{Files: len(results)}
	for _, r := range results {
		if r.Valid {
			s.Valid++
		}
		for _, iss := range r.Issues {
			switch iss.Severity {
			case "error", "fatal":
				s.Errors++
			case "warning":
				s.Warnings++
			}
		}
	}
	return s, nil
}

func statsOutputFile(ext string) string {
	if flagStatsOutput == "" {
		return ""
	}
	if !strings.HasSuffix(flagStatsOutput, "."+ext) && len(flagStatsFormat) > 1 {
		return flagStatsOutput + "." + ext
	}
	return flagStatsOutput
}
