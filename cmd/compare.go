package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fhirlint/fhirlint/internal/profiles"
	"github.com/fhirlint/fhirlint/internal/validator"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	flagCompareLeft         string
	flagCompareRight        string
	flagCompareLeftProfile  string
	flagCompareRightProfile string
	flagCompareIG           []string
	flagCompareFHIRVersion  string
	flagCompareFormat       string
	flagCompareOutput       string
)

var compareCmd = &cobra.Command{
	Use:   "compare --left <profile> --right <profile>",
	Short: "Diff two FHIR profiles (StructureDefinitions)",
	Long: `Compare two StructureDefinitions (profiles) and report how their constraints
differ — cardinalities, bindings, types, and invariants.

This is profile diffing, distinct from 'fhirlint diff', which compares two
validation reports at the instance level. It surfaces the validator's built-in
ComparisonService.

Each side is a local StructureDefinition file or a profile from an IG package.
For a local file the canonical URL is read from the file; for a package, name
the profile with --left-profile / --right-profile (its canonical URL or id).
Profile-package aliases (e.g. kbv-basis) resolve just like 'validate --profile'.

  # Two versions of the same profile from two package versions
  fhirlint compare \
    --left  kbv.basis#1.4.0 --left-profile  KBV_PR_Base_Patient \
    --right kbv.basis#1.5.0 --right-profile KBV_PR_Base_Patient

  # Local profile file vs. published profile
  fhirlint compare \
    --left  ./profiles/our-patient.json \
    --right kbv.basis#1.5.0 --right-profile KBV_PR_Base_Patient

  # Two local files
  fhirlint compare --left ./a.json --right ./b.json

Exit codes:
  0  no differences
  1  differences found
  2  fhirlint itself failed (unresolved profile, tool error, etc.)`,
	Args:         cobra.NoArgs,
	RunE:         runCompare,
	SilenceUsage: true,
}

func init() {
	compareCmd.Flags().StringVar(&flagCompareLeft, "left", "",
		"Left profile: local StructureDefinition file or IG package (pkg#version / alias)")
	compareCmd.Flags().StringVar(&flagCompareRight, "right", "",
		"Right profile: local StructureDefinition file or IG package (pkg#version / alias)")
	compareCmd.Flags().StringVar(&flagCompareLeftProfile, "left-profile", "",
		"Canonical URL or id of the profile within --left (when it is a package)")
	compareCmd.Flags().StringVar(&flagCompareRightProfile, "right-profile", "",
		"Canonical URL or id of the profile within --right (when it is a package)")
	compareCmd.Flags().StringArrayVar(&flagCompareIG, "ig", nil,
		"Additional IG package or file to load for resolution (repeatable)")
	compareCmd.Flags().StringVar(&flagCompareFHIRVersion, "fhir-version", defaultFHIRVersion,
		"FHIR version context (4.0.1, 4.3.0, 5.0.0)")
	compareCmd.Flags().StringVarP(&flagCompareFormat, "format", "f", "terminal",
		"Output format: terminal, json, html")
	compareCmd.Flags().StringVarP(&flagCompareOutput, "output", "o", "",
		"Output file (json) or directory (html). Defaults: json→stdout, html→./fhirlint-compare")

	noFile := cobra.ShellCompDirectiveNoFileComp
	_ = compareCmd.RegisterFlagCompletionFunc("format", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"terminal", "json", "html"}, noFile
	})
	_ = compareCmd.RegisterFlagCompletionFunc("fhir-version", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"4.0.1", "4.3.0", "5.0.0"}, noFile
	})
}

func runCompare(_ *cobra.Command, _ []string) error {
	switch flagCompareFormat {
	case "terminal", "json", "html":
	default:
		return &exitErr{code: 2, err: fmt.Errorf("unknown format %q — use: terminal, json, html", flagCompareFormat)}
	}
	if flagCompareLeft == "" || flagCompareRight == "" {
		return &exitErr{code: 2, err: fmt.Errorf("both --left and --right are required")}
	}

	leftIG, leftCanonical, err := resolveCompareSide(flagCompareLeft, flagCompareLeftProfile, "left")
	if err != nil {
		return &exitErr{code: 2, err: err}
	}
	rightIG, rightCanonical, err := resolveCompareSide(flagCompareRight, flagCompareRightProfile, "right")
	if err != nil {
		return &exitErr{code: 2, err: err}
	}

	igs := append([]string{leftIG, rightIG}, flagCompareIG...)

	destDir, cleanup, err := compareDestDir()
	if err != nil {
		return &exitErr{code: 2, err: err}
	}
	defer cleanup()

	result, err := validator.RunCompare(leftCanonical, rightCanonical, validator.CompareOptions{
		FHIRVersion:      flagCompareFHIRVersion,
		IGs:              igs,
		DestDir:          destDir,
		JARPath:          viper.GetString("jar"),
		ValidatorVersion: viper.GetString("validator-version"),
		Timeout:          10 * time.Minute,
	})
	if err != nil {
		return &exitErr{code: 2, err: err}
	}

	switch flagCompareFormat {
	case "terminal":
		printCompareTerminal(result)
	case "json":
		if err := printCompareJSON(result); err != nil {
			return &exitErr{code: 2, err: err}
		}
	case "html":
		fmt.Printf("Comparison report written to %s\n", result.HTMLFile)
	}

	if result.Differs() {
		return errValidationFailed
	}
	return nil
}

// resolveCompareSide turns a --left/--right value into the -ig argument that
// loads it and the canonical URL the JAR compares on. A local file is loaded
// directly and its canonical is read from the file (unless overridden by
// profileFlag); a package spec (alias-resolved) is loaded as an IG and requires
// profileFlag to name the profile within it.
func resolveCompareSide(value, profileFlag, side string) (ig, canonical string, err error) {
	if info, statErr := os.Stat(value); statErr == nil && !info.IsDir() {
		canonical = profileFlag
		if canonical == "" {
			canonical, err = readProfileCanonical(value)
			if err != nil {
				return "", "", fmt.Errorf("%s profile %q: %w", side, value, err)
			}
		}
		return value, canonical, nil
	}

	resolved := profiles.Resolve(value)
	if profileFlag == "" {
		return "", "", fmt.Errorf("--%s is the package %q — also pass --%s-profile with the profile's canonical URL or id", side, resolved, side)
	}
	return resolved, profileFlag, nil
}

// readProfileCanonical reads the canonical url from a StructureDefinition file.
func readProfileCanonical(path string) (string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // user-supplied profile path
	if err != nil {
		return "", err
	}
	var sd struct {
		ResourceType string `json:"resourceType"`
		URL          string `json:"url"`
	}
	if err := json.Unmarshal(data, &sd); err != nil {
		return "", fmt.Errorf("not a JSON StructureDefinition (pass the canonical explicitly for non-JSON profiles): %w", err)
	}
	if sd.ResourceType != "StructureDefinition" {
		return "", fmt.Errorf("expected a StructureDefinition, got %q", sd.ResourceType)
	}
	if sd.URL == "" {
		return "", fmt.Errorf("StructureDefinition has no url — pass the canonical explicitly")
	}
	return sd.URL, nil
}

// compareDestDir picks the directory the JAR writes its HTML site to. For html
// output it persists to --output (or ./fhirlint-compare); otherwise it uses a
// temp directory that is cleaned up afterwards.
func compareDestDir() (dir string, cleanup func(), err error) {
	if flagCompareFormat == "html" {
		dir = flagCompareOutput
		if dir == "" {
			dir = "fhirlint-compare"
		}
		return dir, func() {}, nil
	}
	dir, err = os.MkdirTemp("", "fhirlint-compare-*")
	if err != nil {
		return "", func() {}, fmt.Errorf("creating temp dir: %w", err)
	}
	return dir, func() { _ = os.RemoveAll(dir) }, nil
}

// printCompareTerminal renders the difference list as an aligned, severity-marked
// change report. An empty list is reported explicitly so "equivalent" is
// distinguishable from a failure.
func printCompareTerminal(r *validator.CompareResult) {
	fmt.Printf("Comparing %s → %s\n", r.Left, r.Right)
	if !r.Differs() {
		fmt.Println("  no differences — the profiles are equivalent")
		return
	}

	width := 0
	for _, m := range r.Messages {
		if len(m.Path) > width {
			width = len(m.Path)
		}
	}
	fmt.Printf("  %d difference(s)\n\n", len(r.Messages))
	for _, m := range r.Messages {
		fmt.Printf("  %s %-*s  %s\n", compareSeverityMarker(m.Severity), width, m.Path, m.Message)
	}
}

func compareSeverityMarker(severity string) string {
	switch severity {
	case "error":
		return "✗"
	case "warning":
		return "⚠"
	default:
		return "~"
	}
}

func printCompareJSON(r *validator.CompareResult) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if flagCompareOutput == "" {
		fmt.Println(string(data))
		return nil
	}
	if err := os.WriteFile(flagCompareOutput, append(data, '\n'), 0600); err != nil {
		return fmt.Errorf("writing %s: %w", flagCompareOutput, err)
	}
	fmt.Printf("JSON comparison written to %s\n", filepath.Clean(flagCompareOutput))
	return nil
}
