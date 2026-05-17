package cmd

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fhirlint/fhirlint/internal/input"
	"github.com/fhirlint/fhirlint/internal/localig"
	"github.com/fhirlint/fhirlint/internal/profiles"
	"github.com/fhirlint/fhirlint/internal/reporter"
	"github.com/fhirlint/fhirlint/internal/resultcache"
	"github.com/fhirlint/fhirlint/internal/suppress"
	"github.com/fhirlint/fhirlint/internal/validator"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tidwall/gjson"
)

const defaultFHIRVersion = "4.0.1"

// errValidationFailed is returned by checkExitCode when issues exceed the
// --fail-on threshold. Using a sentinel instead of os.Exit allows deferred
// temp-file cleanup to run before the process exits.
var errValidationFailed = errors.New("validation failed")

var (
	flagProfile             []string
	flagIG                  []string
	flagFHIRVersion         string
	flagFormat              []string
	flagOutput              string
	flagSeverity            string
	flagFailOn              string
	flagURLs                []string
	flagExtract             string
	flagIgnore              []string
	flagNoTerminologyServer bool
	flagTerminologyServer   string
	flagBestPractice        string
	flagTxCache             string
	flagLocale              string
	flagAllowExampleURLs    bool
	flagAllowInsecureTx     bool
	flagTxLog               string
	flagWatch               string
	flagWatchInterval       int
	flagSuppress            []string
	flagShowSuppressed      bool
	flagExtractEach         string
	flagCodeSystem          []string
	flagValueSet            []string
	flagCache               bool
	flagCacheDir            string
	flagTimeout             string
	flagURLTimeout          string
)

var validateCmd = &cobra.Command{
	Use:   "validate [file-or-dir]",
	Short: "Validate FHIR resource(s)",
	Long: `Validate one or more FHIR resources against profiles and implementation guides.

Input sources (pick one):
  validate patient.json                              file
  validate ./fhir/                                   directory (all .json/.xml files)
  cat patient.json | validate                        stdin
  validate --url https://...                         HTTP endpoint (repeatable)
  validate api.json --extract-each "$.medications"   each element of a JSON array`,
	Args:         cobra.MaximumNArgs(1),
	RunE:         runValidate,
	SilenceUsage: true,
}

func init() {
	validateCmd.Flags().StringArrayVarP(&flagProfile, "profile", "p", nil,
		"Profile alias or URL (repeatable). See: fhirlint profiles")
	validateCmd.Flags().StringArrayVar(&flagIG, "ig", nil,
		"Implementation guide package, e.g. kbv.basis#1.5.0 (repeatable)")
	validateCmd.Flags().StringArrayVar(&flagCodeSystem, "codesystem", nil,
		"Local FHIR CodeSystem JSON file to load without a full IG package (repeatable)")
	validateCmd.Flags().StringArrayVar(&flagValueSet, "valueset", nil,
		"Local FHIR ValueSet JSON file to load without a full IG package (repeatable)")
	validateCmd.Flags().BoolVar(&flagCache, "cache", false,
		"Cache validation results per file hash to skip unchanged files on subsequent runs")
	validateCmd.Flags().StringVar(&flagCacheDir, "cache-dir", "",
		"Directory for result cache (default: ~/.fhirlint/result-cache/)")
	validateCmd.Flags().StringVar(&flagFHIRVersion, "fhir-version", defaultFHIRVersion,
		"FHIR version (4.0.1, 4.3.0, 5.0.0)")
	validateCmd.Flags().StringArrayVarP(&flagFormat, "format", "f", []string{"terminal"},
		"Output format: terminal, json, html, junit, sarif (repeatable)")
	validateCmd.Flags().StringVarP(&flagOutput, "output", "o", "",
		"Output file for json/html (stdout if omitted)")
	validateCmd.Flags().StringVarP(&flagSeverity, "severity", "s", "information",
		"Minimum severity to show: information, warning, error")
	validateCmd.Flags().StringVar(&flagFailOn, "fail-on", "error",
		"Exit non-zero when issues at this level or above are found: error, warning, information, never")
	validateCmd.Flags().StringArrayVar(&flagURLs, "url", nil,
		"Fetch and validate a resource from an HTTP endpoint (repeatable)")
	validateCmd.Flags().StringVar(&flagExtract, "extract", "",
		"JSONPath to extract from the response before validating (e.g. $.entry[0].resource)")
	validateCmd.Flags().StringVar(&flagExtractEach, "extract-each", "",
		"JSONPath to an array — validates each element as a separate FHIR resource (mutually exclusive with --extract)")
	validateCmd.Flags().StringArrayVar(&flagIgnore, "ignore", nil,
		"JSONPath field(s) to remove before validating (repeatable, e.g. $.meta.tag)")
	validateCmd.Flags().BoolVar(&flagNoTerminologyServer, "no-terminology-server", false,
		"Disable terminology server — no data is sent to tx.fhir.org")
	validateCmd.Flags().StringVar(&flagTerminologyServer, "terminology-server", "",
		"Custom terminology server URL (default: https://tx.fhir.org)")
	validateCmd.Flags().BoolVar(&flagAllowInsecureTx, "allow-insecure-tx", false,
		"Suppress warning when terminology server URL uses HTTP instead of HTTPS")
	validateCmd.Flags().StringVar(&flagBestPractice, "best-practice", "",
		"Best-practice constraint handling: ignore, hint, warning, error (default: warning)")
	validateCmd.Flags().StringVar(&flagTxCache, "tx-cache", "",
		"Terminology cache directory (pass n/a to disable, useful with actions/cache in CI)")
	validateCmd.Flags().StringVar(&flagTxLog, "tx-log", "",
		"Write terminology server request log to this file (for debugging and auditing)")
	validateCmd.Flags().StringVar(&flagLocale, "locale", "",
		"Locale for validation messages, e.g. de, fr (default: system locale)")
	validateCmd.Flags().BoolVar(&flagAllowExampleURLs, "allow-example-urls", false,
		"Suppress warnings about example.org and similar placeholder URLs")
	validateCmd.Flags().StringVar(&flagWatch, "watch", "",
		"Watch for file changes and re-validate: single (changed files only) or all (all files on any change)")
	validateCmd.Flags().Lookup("watch").NoOptDefVal = "single"
	validateCmd.Flags().IntVar(&flagWatchInterval, "watch-interval", 0,
		"Polling interval for --watch in milliseconds (default: JAR default)")
	validateCmd.Flags().StringArrayVar(&flagSuppress, "suppress", nil,
		"Silence a known issue: type:value (repeatable). Types: messageId, constraint, expression")
	validateCmd.Flags().BoolVar(&flagShowSuppressed, "show-suppressed", false,
		"Show suppressed issues with a muted label instead of hiding them")
	validateCmd.Flags().StringVar(&flagTimeout, "timeout", "5m",
		"Timeout for the Java validator process (e.g. 30s, 5m, 1h). Set to 0 to disable.")
	validateCmd.Flags().StringVar(&flagURLTimeout, "url-timeout", "30s",
		"Timeout for HTTP fetches via --url (e.g. 10s, 1m). Set to 0 to disable.")

	// Bind all flags to viper so fhirlint.yml values are used as defaults.
	// CLI flags always take precedence over config file values.
	_ = viper.BindPFlag("profile", validateCmd.Flags().Lookup("profile"))
	_ = viper.BindPFlag("ig", validateCmd.Flags().Lookup("ig"))
	_ = viper.BindPFlag("fhir-version", validateCmd.Flags().Lookup("fhir-version"))
	_ = viper.BindPFlag("format", validateCmd.Flags().Lookup("format"))
	_ = viper.BindPFlag("output", validateCmd.Flags().Lookup("output"))
	_ = viper.BindPFlag("severity", validateCmd.Flags().Lookup("severity"))
	_ = viper.BindPFlag("fail-on", validateCmd.Flags().Lookup("fail-on"))
	_ = viper.BindPFlag("url", validateCmd.Flags().Lookup("url"))
	_ = viper.BindPFlag("extract", validateCmd.Flags().Lookup("extract"))
	_ = viper.BindPFlag("ignore", validateCmd.Flags().Lookup("ignore"))
	_ = viper.BindPFlag("no-terminology-server", validateCmd.Flags().Lookup("no-terminology-server"))
	_ = viper.BindPFlag("terminology-server", validateCmd.Flags().Lookup("terminology-server"))
	_ = viper.BindPFlag("allow-insecure-tx", validateCmd.Flags().Lookup("allow-insecure-tx"))
	_ = viper.BindPFlag("best-practice", validateCmd.Flags().Lookup("best-practice"))
	_ = viper.BindPFlag("tx-cache", validateCmd.Flags().Lookup("tx-cache"))
	_ = viper.BindPFlag("tx-log", validateCmd.Flags().Lookup("tx-log"))
	_ = viper.BindPFlag("locale", validateCmd.Flags().Lookup("locale"))
	_ = viper.BindPFlag("allow-example-urls", validateCmd.Flags().Lookup("allow-example-urls"))
	_ = viper.BindPFlag("watch", validateCmd.Flags().Lookup("watch"))
	_ = viper.BindPFlag("watch-interval", validateCmd.Flags().Lookup("watch-interval"))
	_ = viper.BindPFlag("timeout", validateCmd.Flags().Lookup("timeout"))
	_ = viper.BindPFlag("url-timeout", validateCmd.Flags().Lookup("url-timeout"))
}

func runValidate(cmd *cobra.Command, args []string) error {
	if err := validator.CheckJava(); err != nil {
		return err
	}

	// Merge config file values: use CLI flag if explicitly set, otherwise fall
	// back to fhirlint.yml value, then the flag default.
	if !cmd.Flags().Changed("severity") && viper.IsSet("severity") {
		flagSeverity = viper.GetString("severity")
	}
	if !cmd.Flags().Changed("fail-on") && viper.IsSet("fail-on") {
		flagFailOn = viper.GetString("fail-on")
	}
	if !cmd.Flags().Changed("fhir-version") && viper.IsSet("fhir-version") {
		flagFHIRVersion = viper.GetString("fhir-version")
	}
	if !cmd.Flags().Changed("profile") && viper.IsSet("profile") {
		flagProfile = viper.GetStringSlice("profile")
	}
	if !cmd.Flags().Changed("ig") && viper.IsSet("ig") {
		flagIG = viper.GetStringSlice("ig")
	}
	if !cmd.Flags().Changed("format") && viper.IsSet("format") {
		flagFormat = viper.GetStringSlice("format")
	}
	if !cmd.Flags().Changed("output") && viper.IsSet("output") {
		flagOutput = viper.GetString("output")
	}
	if !cmd.Flags().Changed("ignore") && viper.IsSet("ignore") {
		flagIgnore = viper.GetStringSlice("ignore")
	}
	if !cmd.Flags().Changed("extract") && viper.IsSet("extract") {
		flagExtract = viper.GetString("extract")
	}
	if !cmd.Flags().Changed("extract-each") && viper.IsSet("extract-each") {
		flagExtractEach = viper.GetString("extract-each")
	}
	if !cmd.Flags().Changed("url") && viper.IsSet("url") {
		flagURLs = viper.GetStringSlice("url")
	}
	if !cmd.Flags().Changed("no-terminology-server") && viper.IsSet("no-terminology-server") {
		flagNoTerminologyServer = viper.GetBool("no-terminology-server")
	}
	if !cmd.Flags().Changed("terminology-server") && viper.IsSet("terminology-server") {
		flagTerminologyServer = viper.GetString("terminology-server")
	}
	if !cmd.Flags().Changed("allow-insecure-tx") && viper.IsSet("allow-insecure-tx") {
		flagAllowInsecureTx = viper.GetBool("allow-insecure-tx")
	}
	if !cmd.Flags().Changed("best-practice") && viper.IsSet("best-practice") {
		flagBestPractice = viper.GetString("best-practice")
	}
	if !cmd.Flags().Changed("tx-cache") && viper.IsSet("tx-cache") {
		flagTxCache = viper.GetString("tx-cache")
	}
	if !cmd.Flags().Changed("tx-log") && viper.IsSet("tx-log") {
		flagTxLog = viper.GetString("tx-log")
	}
	if !cmd.Flags().Changed("locale") && viper.IsSet("locale") {
		flagLocale = viper.GetString("locale")
	}
	if !cmd.Flags().Changed("allow-example-urls") && viper.IsSet("allow-example-urls") {
		flagAllowExampleURLs = viper.GetBool("allow-example-urls")
	}
	if !cmd.Flags().Changed("watch") && viper.IsSet("watch") {
		flagWatch = viper.GetString("watch")
	}
	if !cmd.Flags().Changed("watch-interval") && viper.IsSet("watch-interval") {
		flagWatchInterval = viper.GetInt("watch-interval")
	}
	if !cmd.Flags().Changed("show-suppressed") && viper.IsSet("show-suppressed") {
		flagShowSuppressed = viper.GetBool("show-suppressed")
	}
	if !cmd.Flags().Changed("cache") && viper.IsSet("cache") {
		flagCache = viper.GetBool("cache")
	}
	if !cmd.Flags().Changed("cache-dir") && viper.IsSet("cache-dir") {
		flagCacheDir = viper.GetString("cache-dir")
	}
	if !cmd.Flags().Changed("timeout") && viper.IsSet("timeout") {
		flagTimeout = viper.GetString("timeout")
	}
	if !cmd.Flags().Changed("url-timeout") && viper.IsSet("url-timeout") {
		flagURLTimeout = viper.GetString("url-timeout")
	}

	var validatorTimeout time.Duration
	if flagTimeout != "0" {
		var parseErr error
		validatorTimeout, parseErr = time.ParseDuration(flagTimeout)
		if parseErr != nil {
			return fmt.Errorf("invalid --timeout value %q (examples: 5m, 30s, 1h): %w", flagTimeout, parseErr)
		}
	}

	var urlTimeout time.Duration
	if flagURLTimeout != "0" {
		var parseErr error
		urlTimeout, parseErr = time.ParseDuration(flagURLTimeout)
		if parseErr != nil {
			return fmt.Errorf("invalid --url-timeout value %q (examples: 10s, 30s, 1m): %w", flagURLTimeout, parseErr)
		}
	}

	if flagExtract != "" && flagExtractEach != "" {
		return fmt.Errorf("--extract and --extract-each are mutually exclusive")
	}

	arg := ""
	if len(args) > 0 {
		arg = args[0]
	}

	// Bundle any --codesystem / --valueset files into a temporary local IG package.
	localFiles := append(flagCodeSystem, flagValueSet...)
	if len(localFiles) > 0 {
		igDir, igCleanup, igErr := localig.PackageDir(localFiles, flagFHIRVersion)
		if igErr != nil {
			return igErr
		}
		defer igCleanup()
		flagIG = append(flagIG, igDir)
	}

	// Resolve profile aliases
	resolvedProfiles := make([]string, 0, len(flagProfile))
	for _, p := range flagProfile {
		resolvedProfiles = append(resolvedProfiles, profiles.Resolve(p))
	}

	opts := validator.Options{
		FHIRVersion:         flagFHIRVersion,
		Profiles:            resolvedProfiles,
		IGs:                 flagIG,
		NoTerminologyServer: flagNoTerminologyServer,
		TerminologyServer:   flagTerminologyServer,
		BestPractice:        flagBestPractice,
		TxCache:             flagTxCache,
		Locale:              flagLocale,
		AllowExampleURLs:    flagAllowExampleURLs,
		AllowInsecureTx:     flagAllowInsecureTx,
		TxLog:               flagTxLog,
		JARPath:             viper.GetString("jar"),
		Timeout:             validatorTimeout,
	}

	// Watch mode: pass -watch-mode to the JAR and block until Ctrl-C.
	if flagWatch != "" {
		if len(flagURLs) > 0 {
			return fmt.Errorf("--watch is not compatible with --url")
		}
		for _, format := range flagFormat {
			if (format == "json" || format == "html") && flagOutput != "" {
				return fmt.Errorf("--watch is not compatible with --format %s --output; use terminal output only", format)
			}
		}
		in, werr := input.Resolve(arg, "", 0)
		if werr != nil {
			return werr
		}
		defer in.Cleanup()
		paths, werr := collectFHIRPaths(in)
		if werr != nil {
			return werr
		}
		fmt.Fprintf(os.Stderr, "Watching %d file(s) for changes (mode: %s). Press Ctrl-C to stop.\n", len(paths), flagWatch)
		return validator.RunWatch(paths, opts, flagWatch, flagWatchInterval)
	}

	var (
		results []*validator.Result
		err     error
	)

	if len(flagURLs) > 0 {
		if arg != "" {
			return fmt.Errorf("cannot combine a file argument with --url; pick one input source")
		}
		if flagExtractEach != "" {
			if len(flagURLs) > 1 {
				return fmt.Errorf("--extract-each can only be used with a single --url")
			}
			in, rerr := input.Resolve("", flagURLs[0], urlTimeout)
			if rerr != nil {
				return rerr
			}
			defer in.Cleanup()
			if len(flagIgnore) > 0 {
				if err := preprocessJSON(in); err != nil {
					return err
				}
			}
			results, err = extractEachAndValidate(in, opts)
		} else {
			results, err = validateURLs(flagURLs, opts, urlTimeout)
		}
	} else {
		in, rerr := input.Resolve(arg, "", 0)
		if rerr != nil {
			return rerr
		}
		defer in.Cleanup()

		if in.Source == input.SourceDir {
			if flagExtractEach != "" {
				return fmt.Errorf("--extract-each cannot be used with a directory")
			}
			// Apply --ignore on directory inputs (--extract not supported for dirs)
			results, err = validateDir(in.Path, opts)
		} else if flagExtractEach != "" {
			// Apply --ignore to outer document before extracting elements
			if len(flagIgnore) > 0 {
				if err := preprocessJSON(in); err != nil {
					return err
				}
			}
			results, err = extractEachAndValidate(in, opts)
		} else {
			// Apply --extract and --ignore on JSON input
			if flagExtract != "" || len(flagIgnore) > 0 {
				if err := preprocessJSON(in); err != nil {
					return err
				}
			}
			rs, rerr2 := runWithCache([]string{in.Path}, opts)
			if rerr2 != nil {
				return rerr2
			}
			rs[0].Label = in.Label
			results = rs
		}
	}
	if err != nil {
		return err
	}

	// Apply suppression rules (CLI flags + config file).
	suppressRules, serr := buildSuppressRules(cmd)
	if serr != nil {
		return serr
	}
	if len(suppressRules) > 0 {
		counts := suppress.Apply(results, suppressRules)
		for i, count := range counts {
			if count == 0 {
				fmt.Fprintf(os.Stderr, "warn: suppress rule %q matched 0 issues\n", suppressRules[i].Raw)
			}
		}
	}

	// Render output(s)
	for _, format := range flagFormat {
		switch strings.ToLower(format) {
		case "terminal":
			for _, r := range results {
				reporter.Terminal(r, flagSeverity, flagShowSuppressed)
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
		case "junit":
			outFile := outputFile("xml")
			if err := reporter.JUnit(results, flagSeverity, outFile); err != nil {
				return fmt.Errorf("junit report: %w", err)
			}
		case "sarif":
			outFile := outputFile("sarif")
			if err := reporter.SARIF(results, flagSeverity, fhirlintVersion(), outFile); err != nil {
				return fmt.Errorf("sarif report: %w", err)
			}
		default:
			return fmt.Errorf("unknown format %q — use: terminal, json, html, junit, sarif", format)
		}
	}

	printUpdateNotice()
	return checkExitCode(results)
}

// collectFHIRPaths returns all .json/.xml file paths for the given input.
func collectFHIRPaths(in *input.Input) ([]string, error) {
	if in.Source != input.SourceDir {
		return []string{in.Path}, nil
	}
	var paths []string
	err := filepath.WalkDir(in.Path, func(path string, d os.DirEntry, err error) error {
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
		paths = append(paths, path)
		return nil
	})
	return paths, err
}

// validateURLs fetches all URLs, applies --extract/--ignore to each response, and validates
// them in a single JVM invocation.
func validateURLs(urls []string, opts validator.Options, httpTimeout time.Duration) ([]*validator.Result, error) {
	ins := make([]*input.Input, 0, len(urls))
	defer func() {
		for _, in := range ins {
			in.Cleanup()
		}
	}()

	for _, u := range urls {
		in, err := input.Resolve("", u, httpTimeout)
		if err != nil {
			return nil, err
		}
		ins = append(ins, in)
	}

	if flagExtract != "" || len(flagIgnore) > 0 {
		for _, in := range ins {
			if err := preprocessJSON(in); err != nil {
				return nil, err
			}
		}
	}

	paths := make([]string, len(ins))
	for i, in := range ins {
		paths[i] = in.Path
	}

	results, err := validator.RunMultiple(paths, opts)
	if err != nil {
		return nil, err
	}

	for i, r := range results {
		if i < len(ins) {
			r.Label = ins[i].Label
		}
	}
	return results, nil
}

// extractEachElements reads the input file, extracts the array at jsonPath, and writes
// each element to its own temp file. Returns one Input per element (caller must Cleanup).
func extractEachElements(in *input.Input, jsonPath string) ([]*input.Input, error) {
	data, err := os.ReadFile(in.Path)
	if err != nil {
		return nil, err
	}

	arr := gjson.Get(string(data), gjsonPath(jsonPath))
	if !arr.Exists() {
		return nil, fmt.Errorf("--extract-each: path %q not found in input", jsonPath)
	}
	if !arr.IsArray() {
		return nil, fmt.Errorf("--extract-each: path %q is not an array", jsonPath)
	}
	elements := arr.Array()
	if len(elements) == 0 {
		return nil, fmt.Errorf("--extract-each: array at %q is empty", jsonPath)
	}

	var ins []*input.Input
	for i, elem := range elements {
		label := fmt.Sprintf("%s[%d]", in.Label, i)
		if rt := elem.Get("resourceType"); rt.Exists() {
			if id := elem.Get("id"); id.Exists() {
				label = fmt.Sprintf("%s[%d] (%s/%s)", in.Label, i, rt.String(), id.String())
			}
		}

		f, ferr := os.CreateTemp("", "fhirlint-extract-*.json")
		if ferr != nil {
			for _, t := range ins {
				t.Cleanup()
			}
			return nil, fmt.Errorf("creating temp file for element %d: %w", i, ferr)
		}
		_, werr := f.WriteString(elem.Raw)
		cerr := f.Close()
		if werr != nil || cerr != nil {
			_ = os.Remove(f.Name())
			for _, t := range ins {
				t.Cleanup()
			}
			if werr != nil {
				return nil, werr
			}
			return nil, cerr
		}

		ins = append(ins, &input.Input{
			Source:   input.SourceFile,
			Path:     f.Name(),
			TempFile: f.Name(),
			Label:    label,
		})
	}
	return ins, nil
}

// extractEachAndValidate extracts array elements from in, validates them in one JVM call,
// and sets each result's label to the element label (filename[i] or filename[i] (Type/id)).
func extractEachAndValidate(in *input.Input, opts validator.Options) ([]*validator.Result, error) {
	ins, err := extractEachElements(in, flagExtractEach)
	if err != nil {
		return nil, err
	}
	defer func() {
		for _, t := range ins {
			t.Cleanup()
		}
	}()

	paths := make([]string, len(ins))
	for i, t := range ins {
		paths[i] = t.Path
	}

	results, err := validator.RunMultiple(paths, opts)
	if err != nil {
		return nil, err
	}
	for i, r := range results {
		if i < len(ins) {
			r.Label = ins[i].Label
		}
	}
	return results, nil
}

// timeNow is a variable so tests can override it.
var timeNow = func() time.Time { return time.Now().UTC() }

// runWithCache validates the given paths, consulting the result cache when --cache is enabled.
// Cached results are used as-is; uncached paths are passed to the validator.
// Fresh results are written back to the cache for future runs.
func runWithCache(paths []string, opts validator.Options) ([]*validator.Result, error) {
	if !flagCache || len(paths) == 0 {
		return validator.RunMultiple(paths, opts)
	}

	cacheDir := flagCacheDir
	if cacheDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return validator.RunMultiple(paths, opts)
		}
		cacheDir = filepath.Join(home, ".fhirlint", "result-cache")
	}

	keyOpts := resultcache.KeyOpts{
		FhirlintVersion: fhirlintVersion(),
		FHIRVersion:     opts.FHIRVersion,
		Profiles:        opts.Profiles,
		IGs:             opts.IGs,
	}

	keys := make([]string, len(paths))
	cachedResults := make([]*validator.Result, len(paths))
	var uncachedPaths []string
	var uncachedIdx []int

	for i, p := range paths {
		key, err := resultcache.Key(p, keyOpts)
		if err != nil {
			uncachedPaths = append(uncachedPaths, p)
			uncachedIdx = append(uncachedIdx, i)
			continue
		}
		keys[i] = key
		entry, err := resultcache.Get(cacheDir, key)
		if err == nil {
			r := entry.Result
			r.Cached = true
			cachedResults[i] = &r
		} else {
			uncachedPaths = append(uncachedPaths, p)
			uncachedIdx = append(uncachedIdx, i)
		}
	}

	var fresh []*validator.Result
	if len(uncachedPaths) > 0 {
		var err error
		fresh, err = validator.RunMultiple(uncachedPaths, opts)
		if err != nil {
			return nil, err
		}
		for j, r := range fresh {
			i := uncachedIdx[j]
			if keys[i] != "" {
				_ = resultcache.Put(cacheDir, keys[i], resultcache.Entry{
					CachedAt:        timeNow(),
					FhirlintVersion: keyOpts.FhirlintVersion,
					Result:          *r,
				})
			}
		}
	}

	results := make([]*validator.Result, len(paths))
	freshIdx := 0
	for i := range paths {
		if cachedResults[i] != nil {
			results[i] = cachedResults[i]
		} else {
			results[i] = fresh[freshIdx]
			freshIdx++
		}
	}
	return results, nil
}

// validateDir finds all .json/.xml files and validates them in a single JVM invocation.
func validateDir(dir string, opts validator.Options) ([]*validator.Result, error) {
	paths, err := collectFHIRPaths(&input.Input{Source: input.SourceDir, Path: dir})
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, nil
	}
	results, err := runWithCache(paths, opts)
	if err != nil {
		return nil, err
	}
	for _, r := range results {
		r.Label = r.Filename
	}
	return results, nil
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

// buildSuppressRules merges --suppress CLI flags with suppress rules from fhirlint.yml.
// CLI flags take precedence: if any --suppress flag was given, config rules are ignored.
func buildSuppressRules(cmd *cobra.Command) ([]suppress.Rule, error) {
	var rules []suppress.Rule
	if cmd.Flags().Changed("suppress") {
		for _, s := range flagSuppress {
			r, err := suppress.ParseCLI(s)
			if err != nil {
				return nil, err
			}
			rules = append(rules, r)
		}
		return rules, nil
	}
	if viper.IsSet("suppress") {
		raw := viper.Get("suppress")
		return parseSuppressFromConfig(raw)
	}
	return nil, nil
}

// parseSuppressFromConfig parses the suppress list from fhirlint.yml.
// Each entry can be a string ("messageId:dom-6") or a map ({messageId: dom-6, severity: warning}).
func parseSuppressFromConfig(raw interface{}) ([]suppress.Rule, error) {
	items, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("suppress config must be a list")
	}
	var rules []suppress.Rule
	for _, item := range items {
		switch v := item.(type) {
		case string:
			r, err := suppress.ParseCLI(v)
			if err != nil {
				return nil, err
			}
			rules = append(rules, r)
		case map[string]interface{}:
			r, err := suppress.ParseMap(v)
			if err != nil {
				return nil, err
			}
			rules = append(rules, r)
		default:
			return nil, fmt.Errorf("suppress rule must be a string or map, got %T", item)
		}
	}
	return rules, nil
}

func checkExitCode(results []*validator.Result) error {
	if flagFailOn == "never" {
		return nil
	}
	threshold := map[string]int{"fatal": 3, "error": 2, "warning": 1, "information": 0}
	min, ok := threshold[flagFailOn]
	if !ok {
		return fmt.Errorf("unknown --fail-on value %q — use: error, warning, information, never", flagFailOn)
	}
	for _, r := range results {
		for _, issue := range r.Issues {
			if threshold[issue.Severity] >= min {
				return errValidationFailed
			}
		}
	}
	return nil
}
