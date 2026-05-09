package cmd

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fhirlint/fhirlint/internal/input"
	"github.com/fhirlint/fhirlint/internal/profiles"
	"github.com/fhirlint/fhirlint/internal/reporter"
	"github.com/fhirlint/fhirlint/internal/validator"
	"github.com/spf13/cobra"
	"github.com/tidwall/gjson"
)

var (
	flagProfile     []string
	flagIG          []string
	flagFHIRVersion string
	flagFormat      []string
	flagOutput      string
	flagSeverity    string
	flagFailOn      string
	flagURL         string
	flagExtract     string
	flagIgnore      []string
)

var validateCmd = &cobra.Command{
	Use:   "validate [file-or-dir]",
	Short: "Validate FHIR resource(s)",
	Long: `Validate one or more FHIR resources against profiles and implementation guides.

Input sources (pick one):
  validate patient.json          file
  validate ./fhir/               directory (all .json/.xml files)
  cat patient.json | validate    stdin
  validate --url https://...     HTTP endpoint`,
	Args:         cobra.MaximumNArgs(1),
	RunE:         runValidate,
	SilenceUsage: true,
}

func init() {
	validateCmd.Flags().StringArrayVarP(&flagProfile, "profile", "p", nil,
		"Profile alias or URL (repeatable). See: fhirlint profiles")
	validateCmd.Flags().StringArrayVar(&flagIG, "ig", nil,
		"Implementation guide package, e.g. kbv.basis#1.5.0 (repeatable)")
	validateCmd.Flags().StringVar(&flagFHIRVersion, "fhir-version", "4.0.1",
		"FHIR version (4.0.1, 4.3.0, 5.0.0)")
	validateCmd.Flags().StringArrayVarP(&flagFormat, "format", "f", []string{"terminal"},
		"Output format: terminal, json, html (repeatable)")
	validateCmd.Flags().StringVarP(&flagOutput, "output", "o", "",
		"Output file for json/html (stdout if omitted)")
	validateCmd.Flags().StringVarP(&flagSeverity, "severity", "s", "information",
		"Minimum severity to show: information, warning, error")
	validateCmd.Flags().StringVar(&flagFailOn, "fail-on", "error",
		"Exit non-zero when issues at this level or above are found: error, warning, never")
	validateCmd.Flags().StringVar(&flagURL, "url", "",
		"Fetch and validate a resource from an HTTP endpoint")
	validateCmd.Flags().StringVar(&flagExtract, "extract", "",
		"JSONPath to extract from the response before validating (e.g. $.entry[0].resource)")
	validateCmd.Flags().StringArrayVar(&flagIgnore, "ignore", nil,
		"JSONPath field(s) to remove before validating (repeatable, e.g. $.meta.tag)")
}

func runValidate(cmd *cobra.Command, args []string) error {
	if err := validator.CheckJava(); err != nil {
		return err
	}

	arg := ""
	if len(args) > 0 {
		arg = args[0]
	}

	in, err := input.Resolve(arg, flagURL)
	if err != nil {
		return err
	}
	defer in.Cleanup()

	// Apply --extract and --ignore on JSON input
	if in.Source != input.SourceDir && (flagExtract != "" || len(flagIgnore) > 0) {
		if err := preprocessJSON(in); err != nil {
			return err
		}
	}

	// Resolve profile aliases
	resolvedProfiles := make([]string, 0, len(flagProfile))
	for _, p := range flagProfile {
		resolvedProfiles = append(resolvedProfiles, profiles.Resolve(p))
	}

	opts := validator.Options{
		FHIRVersion: flagFHIRVersion,
		Profiles:    resolvedProfiles,
		IGs:         flagIG,
	}

	var results []*validator.Result

	if in.Source == input.SourceDir {
		results, err = validateDir(in.Path, opts)
	} else {
		result, rerr := validator.Run(in.Path, opts)
		if rerr != nil {
			return rerr
		}
		result.Label = in.Label
		results = []*validator.Result{result}
	}
	if err != nil {
		return err
	}

	// Render output(s)
	for _, format := range flagFormat {
		switch strings.ToLower(format) {
		case "terminal":
			for _, r := range results {
				reporter.Terminal(r, flagSeverity)
			}
			reporter.TerminalSummary(results, flagSeverity)
		case "json":
			outFile := outputFile("json")
			if err := reporter.JSON(results, flagSeverity, outFile); err != nil {
				return fmt.Errorf("json report: %w", err)
			}
		case "html":
			outFile := outputFile("html")
			if err := reporter.HTML(results, flagSeverity, flagFHIRVersion, outFile); err != nil {
				return fmt.Errorf("html report: %w", err)
			}
		default:
			return fmt.Errorf("unknown format %q — use: terminal, json, html", format)
		}
	}

	printUpdateNotice()
	return checkExitCode(results)
}

// validateDir finds all .json/.xml files and validates each individually.
func validateDir(dir string, opts validator.Options) ([]*validator.Result, error) {
	var results []*validator.Result
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".json" && ext != ".xml" {
			return nil
		}
		result, err := validator.Run(path, opts)
		if err != nil {
			return fmt.Errorf("validating %s: %w", path, err)
		}
		result.Label = path
		results = append(results, result)
		return nil
	})
	return results, err
}

// preprocessJSON applies --extract and --ignore to the input file in-place.
func preprocessJSON(in *input.Input) error {
	data, err := os.ReadFile(in.Path)
	if err != nil {
		return err
	}

	// Detect XML — skip JSON preprocessing
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "<") {
		if flagExtract != "" {
			return fmt.Errorf("--extract is only supported for JSON input")
		}
		if len(flagIgnore) > 0 {
			return applyXMLIgnore(in, data)
		}
		return nil
	}

	raw := string(data)

	if flagExtract != "" {
		path := gjsonPath(flagExtract)
		extracted := gjson.Get(raw, path)
		if !extracted.Exists() {
			return fmt.Errorf("--extract: path %q not found in input", flagExtract)
		}
		raw = extracted.Raw
	}

	for _, ignore := range flagIgnore {
		raw = deleteJSONPath(raw, gjsonPath(ignore))
	}

	return os.WriteFile(in.Path, []byte(raw), 0600) //nolint:gosec // intentional: writing back to the user-supplied input path after preprocessing
}

// gjsonPath converts a simple JSONPath ($.foo.bar[0]) to gjson syntax (foo.bar.0).
func gjsonPath(p string) string {
	p = strings.TrimPrefix(p, "$.")
	p = strings.TrimPrefix(p, "$")
	p = strings.ReplaceAll(p, "[", ".")
	p = strings.ReplaceAll(p, "]", "")
	return p
}

// deleteJSONPath removes a key from a JSON document using gjson to locate it.
// This is a best-effort deletion for top-level and nested keys.
func deleteJSONPath(raw, path string) string {
	result := gjson.Get(raw, path)
	if !result.Exists() {
		return raw
	}
	// Use gjson result index to cut the key out of the raw JSON.
	// For complex nested deletion we rely on re-marshaling.
	var obj interface{}
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return raw
	}
	deleteNestedKey(obj, strings.Split(path, "."))
	out, err := json.Marshal(obj)
	if err != nil {
		return raw
	}
	return string(out)
}

func deleteNestedKey(obj interface{}, parts []string) {
	if len(parts) == 0 {
		return
	}
	switch v := obj.(type) {
	case map[string]interface{}:
		if len(parts) == 1 {
			delete(v, parts[0])
			return
		}
		if child, ok := v[parts[0]]; ok {
			deleteNestedKey(child, parts[1:])
		}
	case []interface{}:
		for _, item := range v {
			deleteNestedKey(item, parts)
		}
	}
}

func applyXMLIgnore(in *input.Input, data []byte) error {
	// Minimal XML field removal: unmarshal → delete tag → re-marshal.
	// For full XPath support this can be extended later.
	var doc map[string]interface{}
	if err := xml.Unmarshal(data, (*xmlMap)(&doc)); err != nil {
		return fmt.Errorf("--ignore on XML is not yet supported for this document structure")
	}
	return nil
}

// xmlMap is a placeholder; full XML ignore support is a future enhancement.
type xmlMap map[string]interface{}

func (x *xmlMap) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	return fmt.Errorf("XML field ignoring is not yet supported — use --ignore with JSON input")
}

func outputFile(ext string) string {
	if flagOutput == "" {
		return ""
	}
	// If --output has no extension matching the format, append it.
	if !strings.HasSuffix(flagOutput, "."+ext) && len(flagFormat) > 1 {
		return flagOutput + "." + ext
	}
	return flagOutput
}

func printUpdateNotice() {
	if newer := validator.CheckForUpdate(); newer != "" {
		fmt.Fprintf(os.Stderr, "\nA new validator version (%s) is available. Run: fhirlint update\n", newer)
	}
}

func checkExitCode(results []*validator.Result) error {
	if flagFailOn == "never" {
		return nil
	}
	threshold := map[string]int{"error": 2, "warning": 1, "information": 0}
	min, ok := threshold[flagFailOn]
	if !ok {
		return fmt.Errorf("unknown --fail-on value %q — use: error, warning, never", flagFailOn)
	}
	for _, r := range results {
		for _, issue := range r.Issues {
			if threshold[issue.Severity] >= min {
				os.Exit(1)
			}
		}
	}
	return nil
}
