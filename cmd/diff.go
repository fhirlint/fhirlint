package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/fhirlint/fhirlint/internal/diff"
	"github.com/fhirlint/fhirlint/internal/reporter"
	"github.com/spf13/cobra"
)

var (
	flagDiffFormat        []string
	flagDiffOutput        string
	flagDiffSeverity      string
	flagDiffShowUnchanged bool
)

var diffCmd = &cobra.Command{
	Use:   "diff <baseline-report.json> <current-report.json>",
	Short: "Compare two validation runs for change control",
	Long: `Compare two JSON validation reports and categorise every issue as new,
resolved, or unchanged.

Both inputs must be reports produced by 'fhirlint validate --format json'.
This provides documented evidence — for change control under ISO 13485 /
IEC 62304 — that a change introduces no new validation issues.

Exit codes:
  0  no new issues
  1  new issues found
  2  fhirlint itself failed (malformed input, etc.)`,
	Args:         cobra.ExactArgs(2),
	RunE:         runDiff,
	SilenceUsage: true,
}

func init() {
	diffCmd.Flags().StringArrayVarP(&flagDiffFormat, "format", "f", []string{"terminal"},
		"Output format: terminal, json, sarif (repeatable)")
	diffCmd.Flags().StringVarP(&flagDiffOutput, "output", "o", "",
		"Output file for json/sarif (stdout if omitted)")
	diffCmd.Flags().StringVarP(&flagDiffSeverity, "severity", "s", "information",
		"Minimum severity to compare: information, warning, error")
	diffCmd.Flags().BoolVar(&flagDiffShowUnchanged, "show-unchanged", false,
		"List unchanged issues instead of only counting them")

	noFile := cobra.ShellCompDirectiveNoFileComp
	_ = diffCmd.RegisterFlagCompletionFunc("format", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"terminal", "json", "sarif"}, noFile
	})
	_ = diffCmd.RegisterFlagCompletionFunc("severity", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"information", "warning", "error"}, noFile
	})
}

func runDiff(_ *cobra.Command, args []string) error {
	baseline, err := readReport(args[0])
	if err != nil {
		return &exitErr{code: 2, err: fmt.Errorf("reading baseline report %q: %w", args[0], err)}
	}
	current, err := readReport(args[1])
	if err != nil {
		return &exitErr{code: 2, err: fmt.Errorf("reading current report %q: %w", args[1], err)}
	}

	d := diff.Compute(baseline.Files, current.Files, flagDiffSeverity)

	for _, format := range flagDiffFormat {
		switch strings.ToLower(format) {
		case "terminal":
			reporter.DiffTerminal(d, flagDiffShowUnchanged)
		case "json":
			if err := reporter.DiffJSON(d, diffOutputFile("json")); err != nil {
				return &exitErr{code: 2, err: fmt.Errorf("json diff report: %w", err)}
			}
		case "sarif":
			if err := reporter.DiffSARIF(d, fhirlintVersion(), diffOutputFile("sarif")); err != nil {
				return &exitErr{code: 2, err: fmt.Errorf("sarif diff report: %w", err)}
			}
		default:
			return &exitErr{code: 2, err: fmt.Errorf("unknown format %q — use: terminal, json, sarif", format)}
		}
	}

	if len(d.New) > 0 {
		return errValidationFailed
	}
	return nil
}

// readReport parses a JSON report written by `fhirlint validate --format json`.
func readReport(path string) (*reporter.JSONReport, error) {
	data, err := os.ReadFile(path) //nolint:gosec // user-supplied report path
	if err != nil {
		return nil, err
	}
	var report reporter.JSONReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("not a valid fhirlint JSON report: %w", err)
	}
	return &report, nil
}

// diffOutputFile mirrors validate's outputFile: it appends the format extension
// when multiple formats share a single --output target.
func diffOutputFile(ext string) string {
	if flagDiffOutput == "" {
		return ""
	}
	if !strings.HasSuffix(flagDiffOutput, "."+ext) && len(flagDiffFormat) > 1 {
		return flagDiffOutput + "." + ext
	}
	return flagDiffOutput
}
