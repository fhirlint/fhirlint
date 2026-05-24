package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/fhirlint/fhirlint/internal/qualify"
	"github.com/fhirlint/fhirlint/internal/validator"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	flagQualifyFormat      []string
	flagQualifyOutput      string
	flagQualifyTestSuite   string
	flagQualifyFHIRVersion string
	flagQualifyTxServer    string
)

var qualifyCmd = &cobra.Command{
	Use:   "qualify",
	Short: "Run Computer System Validation (Operational Qualification)",
	Long: `Run fhirlint against a built-in set of known-valid and known-invalid FHIR
resources and produce a formal Operational Qualification (OQ) report.

This provides documented evidence — for Computer System Validation under
ISO 13485, IEC 62304, and FDA 21 CFR Part 11 — that the tool correctly accepts
valid resources and rejects invalid ones. The report records the tool version,
validator JAR version and SHA256, FHIR version, and per-case results.

By default validation runs offline (no terminology server) for reproducibility.
Supply --test-suite to add your own cases alongside the built-in ones; each FHIR
file needs a companion <name>.expected.json declaring {"valid": true|false}.

The HTML report can be printed to PDF from any browser for DHF or QMS attachment.`,
	Args:         cobra.NoArgs,
	RunE:         runQualify,
	SilenceUsage: true,
}

func init() {
	qualifyCmd.Flags().StringArrayVarP(&flagQualifyFormat, "format", "f", []string{"terminal"},
		"Output format: terminal, html, json (repeatable)")
	qualifyCmd.Flags().StringVarP(&flagQualifyOutput, "output", "o", "",
		"Output file for html/json (stdout if omitted)")
	qualifyCmd.Flags().StringVar(&flagQualifyTestSuite, "test-suite", "",
		"Directory of additional test cases (each .json with a companion .expected.json)")
	qualifyCmd.Flags().StringVar(&flagQualifyFHIRVersion, "fhir-version", defaultFHIRVersion,
		"FHIR version to qualify against (4.0.1, 4.3.0, 5.0.0)")
	qualifyCmd.Flags().StringVar(&flagQualifyTxServer, "terminology-server", "",
		"Terminology server URL (default: offline, for reproducible results)")

	noFile := cobra.ShellCompDirectiveNoFileComp
	_ = qualifyCmd.RegisterFlagCompletionFunc("format", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"terminal", "html", "json"}, noFile
	})
	_ = qualifyCmd.RegisterFlagCompletionFunc("fhir-version", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"4.0.1", "4.3.0", "5.0.0"}, noFile
	})
}

func runQualify(_ *cobra.Command, _ []string) error {
	tmpDir, err := os.MkdirTemp("", "fhirlint-qualify-*")
	if err != nil {
		return &exitErr{code: 2, err: fmt.Errorf("creating temp dir: %w", err)}
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	cases, err := qualify.BuiltinCases(tmpDir)
	if err != nil {
		return &exitErr{code: 2, err: fmt.Errorf("loading built-in test cases: %w", err)}
	}
	if flagQualifyTestSuite != "" {
		custom, lerr := qualify.LoadSuite(flagQualifyTestSuite)
		if lerr != nil {
			return &exitErr{code: 2, err: fmt.Errorf("loading test suite: %w", lerr)}
		}
		cases = append(cases, custom...)
	}
	if len(cases) == 0 {
		return &exitErr{code: 2, err: fmt.Errorf("no test cases to run")}
	}

	jarPath, err := validator.EnsureJAR(viper.GetString("jar"))
	if err != nil {
		return &exitErr{code: 2, err: fmt.Errorf("preparing validator JAR: %w", err)}
	}

	opts := validator.Options{
		FHIRVersion:         flagQualifyFHIRVersion,
		JARPath:             jarPath,
		NoTerminologyServer: flagQualifyTxServer == "",
		TerminologyServer:   flagQualifyTxServer,
		Timeout:             10 * time.Minute,
	}

	paths := make([]string, len(cases))
	for i, c := range cases {
		paths[i] = c.Path
	}
	results, err := validator.RunMultiple(paths, opts)
	if err != nil {
		return &exitErr{code: 2, err: fmt.Errorf("running validation: %w", err)}
	}

	byPath := make(map[string]*validator.Result, len(results))
	for _, r := range results {
		byPath[r.Filename] = r
	}

	report := &qualify.Report{
		ToolVersion: fhirlintVersion(),
		JARVersion:  validator.ValidatorVersion(),
		JARSHA256:   jarSHA256(jarPath),
		FHIRVersion: flagQualifyFHIRVersion,
		Terminology: terminologyLabel(flagQualifyTxServer),
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Cases:       qualify.Evaluate(cases, byPath),
	}
	report.Passed, report.Failed, report.Qualified = qualify.Summarize(report.Cases)

	for _, format := range flagQualifyFormat {
		switch strings.ToLower(format) {
		case "terminal":
			qualify.Terminal(os.Stdout, report)
		case "html":
			if err := qualify.HTML(report, qualifyOutputFile("html")); err != nil {
				return &exitErr{code: 2, err: fmt.Errorf("html report: %w", err)}
			}
		case "json":
			if err := qualify.JSON(report, qualifyOutputFile("json")); err != nil {
				return &exitErr{code: 2, err: fmt.Errorf("json report: %w", err)}
			}
		default:
			return &exitErr{code: 2, err: fmt.Errorf("unknown format %q — use: terminal, html, json", format)}
		}
	}

	if !report.Qualified {
		return errValidationFailed
	}
	return nil
}

func terminologyLabel(server string) string {
	if server == "" {
		return "offline"
	}
	return server
}

// jarSHA256 returns the hex SHA256 of the JAR, or "unavailable" if it can't be read.
func jarSHA256(path string) string {
	f, err := os.Open(path) //nolint:gosec // path comes from EnsureJAR
	if err != nil {
		return "unavailable"
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "unavailable"
	}
	return hex.EncodeToString(h.Sum(nil))
}

func qualifyOutputFile(ext string) string {
	if flagQualifyOutput == "" {
		return ""
	}
	if !strings.HasSuffix(flagQualifyOutput, "."+ext) && len(flagQualifyFormat) > 1 {
		return flagQualifyOutput + "." + ext
	}
	return flagQualifyOutput
}
