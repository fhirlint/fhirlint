package cmd

import (
	"bufio"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fhirlint/fhirlint/internal/baseline"
	"github.com/fhirlint/fhirlint/internal/cache"
	"github.com/fhirlint/fhirlint/internal/coverage"
	"github.com/fhirlint/fhirlint/internal/iglock"
	"github.com/fhirlint/fhirlint/internal/input"
	"github.com/fhirlint/fhirlint/internal/lint"
	"github.com/fhirlint/fhirlint/internal/localig"
	"github.com/fhirlint/fhirlint/internal/ndjson"
	"github.com/fhirlint/fhirlint/internal/profiles"
	"github.com/fhirlint/fhirlint/internal/redact"
	"github.com/fhirlint/fhirlint/internal/refcheck"
	"github.com/fhirlint/fhirlint/internal/reporter"
	"github.com/fhirlint/fhirlint/internal/resultcache"
	"github.com/fhirlint/fhirlint/internal/rules"
	"github.com/fhirlint/fhirlint/internal/suppress"
	"github.com/fhirlint/fhirlint/internal/txreplay"
	"github.com/fhirlint/fhirlint/internal/validator"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tidwall/gjson"
	"go.yaml.in/yaml/v3"
)

const defaultFHIRVersion = validator.DefaultFHIRVersion

// errValidationFailed is returned by checkExitCode when issues exceed the
// --fail-on threshold. Using a sentinel instead of os.Exit allows deferred
// temp-file cleanup to run before the process exits.
var errValidationFailed = errors.New("validation failed")

// configOverride is a single entry in the overrides: config key.
// Matching overrides are merged on top of the global config for each file.
type configOverride struct {
	Files        []string        // gitignore-style glob patterns
	IGs          []string        // appended to global IGs for matching files
	Profiles     []string        // appended to global profiles for matching files
	BestPractice string          // replaces global best-practice for matching files
	Severity     string          // filters issues to this level and above for matching files
	FailOn       string          // "never" = matching files don't contribute to the exit code
	Suppress     []suppress.Rule // additional suppress rules for matching files
}

var (
	flagProfile                  []string
	flagIG                       []string
	flagFHIRVersion              string
	flagFormat                   []string
	flagOutput                   string
	flagSeverity                 string
	flagFailOn                   string
	flagURLs                     []string
	flagExtract                  string
	flagIgnore                   []string
	flagNoTerminologyServer      bool
	flagTerminologyServer        string
	flagBestPractice             string
	flagTxCache                  string
	flagLocale                   string
	flagAllowExampleURLs         bool
	flagAllowInsecureTx          bool
	flagExclude                  []string
	flagTxLog                    string
	flagJurisdiction             string
	flagDisplayIssuesAreWarnings bool
	flagPO                       []string
	flagMaxWarnings              int
	flagWatch                    string
	flagWatchInterval            int
	flagSuppress                 []string
	flagShowSuppressed           bool
	flagExtractEach              string
	flagCodeSystem               []string
	flagValueSet                 []string
	flagCache                    bool
	flagCacheDir                 string
	flagTimeout                  string
	flagValidationTimeout        string
	flagProxy                    string
	flagHTTPSProxy               string
	flagMaxMessages              int
	flagCodeSystemSizeLimit      int
	flagOffline                  bool
	flagURLTimeout               string
	flagLock                     bool
	flagGenerateBaseline         string
	flagBaseline                 string
	flagBundleEntries            bool
	flagSkipNonFHIR              bool
	flagValidatorArg             []string
	flagRequireSuppressReason    bool
	flagShowSource               bool
	flagRedact                   bool
	flagGroup                    bool
	flagQuiet                    bool
	flagNoColor                  bool
	flagRulesFile                string
	flagCheckReferences          bool
	flagSince                    string
	flagTxOffline                bool
	flagTxDir                    string
	flagServer                   string
)

var validateCmd = &cobra.Command{
	Use:   "validate [file-or-dir...]",
	Short: "Validate FHIR resource(s)",
	Long: `Validate one or more FHIR resources against profiles and implementation guides.

Input sources (pick one):
  validate patient.json                              file
  validate a.json b.json ./fhir/                     several files and/or directories
  validate ./fhir/                                   directory (all .json/.xml files)
  cat patient.json | validate                        stdin
  validate --url https://...                         HTTP endpoint (repeatable)
  validate api.json --extract-each "$.medications"   each element of a JSON array`,
	Args:         cobra.ArbitraryArgs,
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
		"FHIR version ("+validator.FHIRVersionList()+")")
	validateCmd.Flags().StringArrayVarP(&flagFormat, "format", "f", []string{"terminal"},
		"Output format: terminal, json, html, junit, sarif, markdown, codeclimate, github (repeatable)")
	validateCmd.Flags().StringVarP(&flagOutput, "output", "o", "",
		"Output file for json/html (stdout if omitted)")
	validateCmd.Flags().StringVarP(&flagSeverity, "severity", "s", "information",
		"Minimum severity to show: information, warning, error")
	validateCmd.Flags().StringVar(&flagFailOn, "fail-on", "error",
		"Exit non-zero when issues at this level or above are found: error, warning, information, never")
	validateCmd.Flags().IntVar(&flagMaxWarnings, "max-warnings", -1,
		"Exit non-zero when warning count exceeds N (ratchet pattern; -1 disables)")
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
	validateCmd.Flags().StringArrayVar(&flagExclude, "exclude", nil,
		"Exclude files or directories matching pattern (repeatable, gitignore-style)")
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
	validateCmd.Flags().StringVar(&flagJurisdiction, "jurisdiction", "",
		"Jurisdiction for country-specific bindings, e.g. urn:iso:std:iso:3166#DE (default: derived from locale)")
	validateCmd.Flags().BoolVar(&flagDisplayIssuesAreWarnings, "display-issues-are-warnings", false,
		"Downgrade coded-display mismatch errors to warnings")
	validateCmd.Flags().StringArrayVar(&flagPO, "po", nil,
		"Load message translations from a .po file at runtime, e.g. validator-messages-de.po (repeatable)")
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
	// Distinct from --timeout: that one kills the JVM and yields nothing, this
	// one asks the validator to stop and hand back what it has.
	validateCmd.Flags().StringVar(&flagValidationTimeout, "validation-timeout", "",
		"Stop validating after this long and report partial results (e.g. 90s, 2m). Unset = unbounded.")
	validateCmd.Flags().IntVar(&flagMaxMessages, "max-messages", 0,
		"Stop after this many validation messages and report partial results. 0 = unbounded.")
	validateCmd.Flags().IntVar(&flagCodeSystemSizeLimit, "codesystem-size-limit",
		validator.UnsetCodeSystemSizeLimit,
		"Max codes checked per ValueSet include, ConceptMap group or CodeSystem supplement. "+
			"0 = no limit; -1 leaves the validator's own default (1000) in place.")
	validateCmd.Flags().BoolVar(&flagOffline, "offline", false,
		"Forbid all network access: use the cached JAR and cached IG packages, and block the validator's own HTTP")
	_ = viper.BindPFlag("offline", validateCmd.Flags().Lookup("offline"))
	_ = viper.BindPFlag("codesystem-size-limit", validateCmd.Flags().Lookup("codesystem-size-limit"))
	_ = viper.BindPFlag("validation-timeout", validateCmd.Flags().Lookup("validation-timeout"))
	_ = viper.BindPFlag("max-messages", validateCmd.Flags().Lookup("max-messages"))
	// Credentials deliberately have no flag and no config key — see
	// validator.ProxyAuthEnvVar.
	validateCmd.Flags().StringVar(&flagProxy, "proxy", "",
		"HTTP proxy for the validator's terminology calls, host:port (default: $HTTP_PROXY)")
	validateCmd.Flags().StringVar(&flagHTTPSProxy, "https-proxy", "",
		"HTTPS proxy for the validator's terminology calls, host:port (default: $HTTPS_PROXY)")
	_ = viper.BindPFlag("proxy", validateCmd.Flags().Lookup("proxy"))
	_ = viper.BindPFlag("https-proxy", validateCmd.Flags().Lookup("https-proxy"))
	validateCmd.Flags().StringVar(&flagURLTimeout, "url-timeout", "30s",
		"Timeout for HTTP fetches via --url (e.g. 10s, 1m). Set to 0 to disable.")
	validateCmd.Flags().BoolVar(&flagLock, "lock", false,
		"Write or update fhirlint.lock with SHA256 hashes of all resolved IG packages")
	validateCmd.Flags().StringVar(&flagGenerateBaseline, "generate-baseline", "",
		"Generate a baseline file from current issues (build always succeeds; re-run to update)")
	validateCmd.Flags().StringVar(&flagBaseline, "baseline", "",
		"Baseline file — issues recorded here are suppressed; only new issues fail the build")
	_ = viper.BindPFlag("baseline", validateCmd.Flags().Lookup("baseline"))
	validateCmd.Flags().BoolVar(&flagBundleEntries, "bundle-entries", false,
		"Also validate each entry.resource inside a FHIR Bundle as a standalone resource")
	_ = viper.BindPFlag("bundle-entries", validateCmd.Flags().Lookup("bundle-entries"))
	validateCmd.Flags().BoolVar(&flagSkipNonFHIR, "skip-non-fhir", false,
		"Skip files that are not FHIR resources instead of reporting them as errors")
	_ = viper.BindPFlag("skip-non-fhir", validateCmd.Flags().Lookup("skip-non-fhir"))
	validateCmd.Flags().StringArrayVar(&flagValidatorArg, "validator-arg", nil,
		"Extra argument passed straight to the validator JAR (repeatable, unvalidated)")
	_ = viper.BindPFlag("validator-arg", validateCmd.Flags().Lookup("validator-arg"))
	validateCmd.Flags().BoolVar(&flagRequireSuppressReason, "require-suppress-reason", false,
		"Fail when a suppression rule has no reason")
	_ = viper.BindPFlag("require-suppress-reason", validateCmd.Flags().Lookup("require-suppress-reason"))
	validateCmd.Flags().BoolVar(&flagShowSource, "show-source", false,
		"Show the offending source line under each finding (terminal output only)")
	_ = viper.BindPFlag("show-source", validateCmd.Flags().Lookup("show-source"))
	validateCmd.Flags().BoolVar(&flagRedact, "redact", false,
		"Remove message text and source lines from all reports, keeping severity, location and message ID (for reports that leave a trusted environment)")
	_ = viper.BindPFlag("redact", validateCmd.Flags().Lookup("redact"))
	validateCmd.Flags().BoolVar(&flagGroup, "group", false,
		"Group repeated findings into one block each with a count (terminal output only)")
	_ = viper.BindPFlag("group", validateCmd.Flags().Lookup("group"))
	validateCmd.Flags().BoolVarP(&flagQuiet, "quiet", "q", false,
		"Suppress per-file output for valid files; only files with issues are printed")
	_ = viper.BindPFlag("quiet", validateCmd.Flags().Lookup("quiet"))
	validateCmd.Flags().BoolVar(&flagNoColor, "no-color", false,
		"Disable ANSI color output")
	_ = viper.BindPFlag("no-color", validateCmd.Flags().Lookup("no-color"))
	validateCmd.Flags().StringVar(&flagRulesFile, "rules-file", "",
		"Load custom FHIRPath lint rules from a YAML file (overrides rules: in config)")
	validateCmd.Flags().BoolVar(&flagCheckReferences, "check-references", false,
		"Check that references resolve within the validated resource set (dangling-reference detection)")
	_ = viper.BindPFlag("check-references", validateCmd.Flags().Lookup("check-references"))
	validateCmd.Flags().StringVar(&flagSince, "since", "",
		"Validate only files changed against this git ref (e.g. main), including uncommitted and untracked ones")
	_ = viper.BindPFlag("since", validateCmd.Flags().Lookup("since"))
	validateCmd.Flags().BoolVar(&flagTxOffline, "tx-offline", false,
		"Replay terminology responses recorded by 'fhirlint tx warm' instead of contacting a server; an unrecorded request is an error")
	_ = viper.BindPFlag("tx-offline", validateCmd.Flags().Lookup("tx-offline"))
	validateCmd.Flags().StringVar(&flagTxDir, "tx-dir", "",
		"Directory holding the terminology recording (default: "+txreplay.DefaultDir+"/)")
	_ = viper.BindPFlag("tx-dir", validateCmd.Flags().Lookup("tx-dir"))
	validateCmd.Flags().StringVar(&flagServer, "server", "",
		"Validate via a running validator server instead of a per-run JVM (e.g. http://localhost:8080; see 'fhirlint serve')")
	_ = viper.BindPFlag("server", validateCmd.Flags().Lookup("server"))

	// Bind all flags to viper so fhirlint.yml values are used as defaults.
	// CLI flags always take precedence over config file values.
	_ = viper.BindPFlag("profile", validateCmd.Flags().Lookup("profile"))
	_ = viper.BindPFlag("ig", validateCmd.Flags().Lookup("ig"))
	_ = viper.BindPFlag("fhir-version", validateCmd.Flags().Lookup("fhir-version"))
	_ = viper.BindPFlag("format", validateCmd.Flags().Lookup("format"))
	_ = viper.BindPFlag("output", validateCmd.Flags().Lookup("output"))
	_ = viper.BindPFlag("severity", validateCmd.Flags().Lookup("severity"))
	_ = viper.BindPFlag("fail-on", validateCmd.Flags().Lookup("fail-on"))
	_ = viper.BindPFlag("max-warnings", validateCmd.Flags().Lookup("max-warnings"))
	_ = viper.BindPFlag("url", validateCmd.Flags().Lookup("url"))
	_ = viper.BindPFlag("extract", validateCmd.Flags().Lookup("extract"))
	_ = viper.BindPFlag("ignore", validateCmd.Flags().Lookup("ignore"))
	_ = viper.BindPFlag("no-terminology-server", validateCmd.Flags().Lookup("no-terminology-server"))
	_ = viper.BindPFlag("terminology-server", validateCmd.Flags().Lookup("terminology-server"))
	_ = viper.BindPFlag("allow-insecure-tx", validateCmd.Flags().Lookup("allow-insecure-tx"))
	_ = viper.BindPFlag("exclude", validateCmd.Flags().Lookup("exclude"))
	_ = viper.BindPFlag("best-practice", validateCmd.Flags().Lookup("best-practice"))
	_ = viper.BindPFlag("tx-cache", validateCmd.Flags().Lookup("tx-cache"))
	_ = viper.BindPFlag("tx-log", validateCmd.Flags().Lookup("tx-log"))
	_ = viper.BindPFlag("locale", validateCmd.Flags().Lookup("locale"))
	_ = viper.BindPFlag("allow-example-urls", validateCmd.Flags().Lookup("allow-example-urls"))
	_ = viper.BindPFlag("jurisdiction", validateCmd.Flags().Lookup("jurisdiction"))
	_ = viper.BindPFlag("display-issues-are-warnings", validateCmd.Flags().Lookup("display-issues-are-warnings"))
	_ = viper.BindPFlag("po", validateCmd.Flags().Lookup("po"))
	_ = viper.BindPFlag("watch", validateCmd.Flags().Lookup("watch"))
	_ = viper.BindPFlag("watch-interval", validateCmd.Flags().Lookup("watch-interval"))
	_ = viper.BindPFlag("timeout", validateCmd.Flags().Lookup("timeout"))
	_ = viper.BindPFlag("url-timeout", validateCmd.Flags().Lookup("url-timeout"))

	registerFlagCompletions(validateCmd)
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
	if !cmd.Flags().Changed("max-warnings") && viper.IsSet("max-warnings") {
		flagMaxWarnings = viper.GetInt("max-warnings")
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
	if !cmd.Flags().Changed("exclude") && viper.IsSet("exclude") {
		flagExclude = viper.GetStringSlice("exclude")
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
	if !cmd.Flags().Changed("jurisdiction") && viper.IsSet("jurisdiction") {
		flagJurisdiction = viper.GetString("jurisdiction")
	}
	if !cmd.Flags().Changed("display-issues-are-warnings") && viper.IsSet("display-issues-are-warnings") {
		flagDisplayIssuesAreWarnings = viper.GetBool("display-issues-are-warnings")
	}
	if !cmd.Flags().Changed("po") && viper.IsSet("po") {
		flagPO = viper.GetStringSlice("po")
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
	if !cmd.Flags().Changed("validation-timeout") && viper.IsSet("validation-timeout") {
		flagValidationTimeout = viper.GetString("validation-timeout")
	}
	if !cmd.Flags().Changed("max-messages") && viper.IsSet("max-messages") {
		flagMaxMessages = viper.GetInt("max-messages")
	}
	if !cmd.Flags().Changed("codesystem-size-limit") && viper.IsSet("codesystem-size-limit") {
		flagCodeSystemSizeLimit = viper.GetInt("codesystem-size-limit")
	}
	if !cmd.Flags().Changed("offline") && viper.IsSet("offline") {
		flagOffline = viper.GetBool("offline")
	}
	if !cmd.Flags().Changed("proxy") && viper.IsSet("proxy") {
		flagProxy = viper.GetString("proxy")
	}
	if !cmd.Flags().Changed("https-proxy") && viper.IsSet("https-proxy") {
		flagHTTPSProxy = viper.GetString("https-proxy")
	}
	if !cmd.Flags().Changed("baseline") && viper.IsSet("baseline") {
		flagBaseline = viper.GetString("baseline")
	}
	if !cmd.Flags().Changed("bundle-entries") && viper.IsSet("bundle-entries") {
		flagBundleEntries = viper.GetBool("bundle-entries")
	}
	if !cmd.Flags().Changed("skip-non-fhir") && viper.IsSet("skip-non-fhir") {
		flagSkipNonFHIR = viper.GetBool("skip-non-fhir")
	}
	if !cmd.Flags().Changed("validator-arg") && viper.IsSet("validator-arg") {
		flagValidatorArg = viper.GetStringSlice("validator-arg")
	}
	if !cmd.Flags().Changed("require-suppress-reason") && viper.IsSet("require-suppress-reason") {
		flagRequireSuppressReason = viper.GetBool("require-suppress-reason")
	}
	if !cmd.Flags().Changed("show-source") && viper.IsSet("show-source") {
		flagShowSource = viper.GetBool("show-source")
	}
	if !cmd.Flags().Changed("redact") && viper.IsSet("redact") {
		flagRedact = viper.GetBool("redact")
	}
	if !cmd.Flags().Changed("group") && viper.IsSet("group") {
		flagGroup = viper.GetBool("group")
	}
	if !cmd.Flags().Changed("check-references") && viper.IsSet("check-references") {
		flagCheckReferences = viper.GetBool("check-references")
	}
	if !cmd.Flags().Changed("since") && viper.IsSet("since") {
		flagSince = viper.GetString("since")
	}
	if !cmd.Flags().Changed("tx-offline") && viper.IsSet("tx-offline") {
		flagTxOffline = viper.GetBool("tx-offline")
	}
	if !cmd.Flags().Changed("tx-dir") && viper.IsSet("tx-dir") {
		flagTxDir = viper.GetString("tx-dir")
	}
	if !cmd.Flags().Changed("server") && viper.IsSet("server") {
		flagServer = viper.GetString("server")
	}
	if !cmd.Flags().Changed("quiet") && viper.IsSet("quiet") {
		flagQuiet = viper.GetBool("quiet")
	}
	if !cmd.Flags().Changed("no-color") && viper.IsSet("no-color") {
		flagNoColor = viper.GetBool("no-color")
	}

	if flagNoColor {
		reporter.DisableColors()
	}

	// Merge .fhirlintignore patterns into the exclude list.
	excludePatterns := append([]string{}, flagExclude...)
	if ignorePatterns, err := loadIgnoreFile(".fhirlintignore"); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: reading .fhirlintignore: %v\n", err)
	} else {
		excludePatterns = append(excludePatterns, ignorePatterns...)
	}
	excludePatterns = append(excludePatterns, txRecordingExcludes(flagTxDir)...)

	profileMap := loadProfileMap()
	overrides, oerr := loadOverrides()
	if oerr != nil {
		return oerr
	}
	// Overrides carry their own suppress rules, so the policy has to reach them
	// too — otherwise it is sidestepped by nesting the rule under `overrides:`.
	for _, ov := range overrides {
		if err := checkSuppressReasons(ov.Suppress, "overrides suppress"); err != nil {
			return err
		}
	}

	var validatorTimeout time.Duration
	if flagTimeout != "0" {
		var parseErr error
		validatorTimeout, parseErr = time.ParseDuration(flagTimeout)
		if parseErr != nil {
			return fmt.Errorf("invalid --timeout value %q (examples: 5m, 30s, 1h): %w", flagTimeout, parseErr)
		}
	}

	var validationTimeout time.Duration
	if flagValidationTimeout != "" && flagValidationTimeout != "0" {
		var parseErr error
		validationTimeout, parseErr = time.ParseDuration(flagValidationTimeout)
		if parseErr != nil {
			return fmt.Errorf("invalid --validation-timeout value %q (examples: 90s, 2m): %w", flagValidationTimeout, parseErr)
		}
		if validationTimeout <= 0 {
			return fmt.Errorf("--validation-timeout must be positive, got %q", flagValidationTimeout)
		}
	}
	if flagMaxMessages < 0 {
		return fmt.Errorf("--max-messages must be zero or positive, got %d", flagMaxMessages)
	}
	if flagOffline && cmd.Flags().Changed("terminology-server") {
		return fmt.Errorf("--offline forbids network access, so it cannot be combined with --terminology-server — " +
			"drop one, or replay a recording with --tx-offline")
	}
	if flagCodeSystemSizeLimit < validator.UnsetCodeSystemSizeLimit {
		return fmt.Errorf("--codesystem-size-limit must be -1 (the validator's default), 0 (no limit) or positive, got %d",
			flagCodeSystemSizeLimit)
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

	// Modes that consume exactly one input document cannot be combined with a
	// list of paths.
	if len(args) > 1 {
		switch {
		case len(flagURLs) > 0:
			return fmt.Errorf("cannot combine file arguments with --url; pick one input source")
		case flagExtractEach != "":
			return fmt.Errorf("--extract-each requires a single input; pass one file at a time")
		case flagWatch != "":
			return fmt.Errorf("--watch requires a single file or directory")
		}
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

	// Resolve profile aliases. An alias may stand for several packages, so the
	// resolved list can be longer than what the user typed.
	resolvedProfiles := profiles.ResolveAll(flagProfile)

	opts := validator.Options{
		FHIRVersion:              flagFHIRVersion,
		Profiles:                 resolvedProfiles,
		IGs:                      flagIG,
		NoTerminologyServer:      flagNoTerminologyServer,
		TerminologyServer:        flagTerminologyServer,
		BestPractice:             flagBestPractice,
		TxCache:                  flagTxCache,
		Locale:                   flagLocale,
		AllowExampleURLs:         flagAllowExampleURLs,
		AllowInsecureTx:          flagAllowInsecureTx,
		TxLog:                    flagTxLog,
		Jurisdiction:             flagJurisdiction,
		DisplayIssuesAreWarnings: flagDisplayIssuesAreWarnings,
		POFiles:                  flagPO,
		JARPath:                  viper.GetString("jar"),
		ValidatorVersion:         viper.GetString("validator-version"),
		ExtraArgs:                flagValidatorArg,
		ValidationTimeout:        validationTimeout,
		MaxMessages:              flagMaxMessages,
		CodeSystemSizeLimit:      codeSystemSizeLimitOpt(flagCodeSystemSizeLimit),
		Offline:                  flagOffline,
		Proxy:                    validator.ProxyConfig{Proxy: flagProxy, HTTPSProxy: flagHTTPSProxy},
		Timeout:                  validatorTimeout,
	}

	// Offline terminology: stand in for the terminology server with recorded
	// responses. The JAR cannot do this itself — its own -txCache still needs a
	// reachable server for the capability statement it fetches at startup.
	var txPlayer *txreplay.Server
	if flagTxOffline {
		switch {
		case flagNoTerminologyServer:
			return fmt.Errorf("--tx-offline and --no-terminology-server are mutually exclusive: one replays terminology, the other skips it")
		case flagServer != "":
			return fmt.Errorf("--tx-offline is not compatible with --server; the validator server's terminology is fixed at its startup")
		case cmd.Flags().Changed("terminology-server"):
			return fmt.Errorf("--tx-offline replaces the terminology server; drop --terminology-server or record against it with 'fhirlint tx warm'")
		}
		dir := txRecordingDir(flagTxDir)
		store, serr := txreplay.Open(dir)
		if serr != nil {
			return serr
		}
		if store.Len() == 0 {
			return fmt.Errorf("no terminology recording in %s/ — record one first with: fhirlint tx warm %s", dir, arg)
		}
		if err := ensureNoUserFHIRSettings(flagValidatorArg); err != nil {
			return err
		}
		txPlayer = txreplay.NewPlayer(store)
		baseURL, perr := txPlayer.Start()
		if perr != nil {
			return perr
		}
		defer func() { _ = txPlayer.Stop() }()
		// Validator 6.10.0+ refuses plain-HTTP destinations. Exempt only our own
		// loopback replay server; disabling SSRF protection for the whole run
		// would be a far bigger hammer than the problem.
		settingsPath, cleanupSettings, serr := txreplay.WriteJARSettings(baseURL)
		if serr != nil {
			return serr
		}
		defer cleanupSettings()
		opts.FHIRSettings = settingsPath
		opts.TerminologyServer = baseURL
		// Loopback HTTP to our own process; the insecure-transport warning is
		// about sending data unencrypted to a remote server.
		opts.AllowInsecureTx = true
		// Disable the JAR's own terminology cache so every request reaches the
		// replay server. Left on, it would answer from ~/.fhir and hide an
		// incomplete recording — green here, failing on a clean CI runner.
		opts.TxCache = "n/a"
		fmt.Fprintf(os.Stderr, "Replaying %d recorded terminology interaction(s) from %s/\n", store.Len(), dir)
		warnValidatorVersionDrift(os.Stderr, store.ReadManifest(), validator.EffectiveValidatorVersion(viper.GetString("validator-version")))
	}

	if flagOffline {
		if err := applyOffline(&opts, os.Stderr); err != nil {
			return err
		}
	}

	// Watch mode: pass -watch-mode to the JAR and block until Ctrl-C.
	if flagServer != "" {
		if flagWatch != "" {
			return fmt.Errorf("--server is not compatible with --watch")
		}
		if cmd.Flags().Changed("fhir-version") || cmd.Flags().Changed("ig") {
			fmt.Fprintln(os.Stderr, "note: --server uses the running server's FHIR version and IGs; --fhir-version/--ig are ignored")
		}
	}

	if flagSince != "" {
		switch {
		case len(flagURLs) > 0:
			return fmt.Errorf("--since is not compatible with --url; it selects files from a git working tree")
		case arg == "" || arg == "-":
			return fmt.Errorf("--since needs a file or directory argument; it cannot select from stdin")
		case flagWatch != "":
			return fmt.Errorf("--since is not compatible with --watch")
		}
		// Resolve the repository from the input path, not the working directory:
		// validating a checkout from somewhere else is ordinary usage and should
		// not fail with "needs a git repository".
		workdir := arg
		if info, statErr := os.Stat(arg); statErr == nil && !info.IsDir() {
			workdir = filepath.Dir(arg)
		}
		changed, serr := input.ChangedSince(flagSince, workdir)
		if serr != nil {
			return serr
		}
		sinceChanged = make(map[string]bool, len(changed))
		for _, p := range changed {
			sinceChanged[p] = true
		}
	}

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
		paths, werr := collectFHIRPaths(in, excludePatterns)
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
	} else if len(args) > 1 {
		// Several files and/or directories: expand them into one path list so
		// everything is validated in as few JVM invocations as possible.
		paths, perr := collectPathsFromArgs(args, excludePatterns)
		if perr != nil {
			return perr
		}
		results, err = validatePaths(paths, opts, profileMap, overrides)
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
			results, err = validateDir(in.Path, opts, excludePatterns, profileMap, overrides)
		} else if flagExtractEach != "" {
			// Apply --ignore to outer document before extracting elements
			if len(flagIgnore) > 0 {
				if err := preprocessJSON(in); err != nil {
					return err
				}
			}
			results, err = extractEachAndValidate(in, opts)
		} else if ndjson.IsNDJSON(in.Path) {
			if flagExtractEach != "" {
				return fmt.Errorf("--extract-each is not supported for NDJSON input")
			}
			results, err = validateNDJSON(in.Path, opts, profileMap, overrides)
		} else if len(filterSincePaths(filterFHIRPaths([]string{in.Path}))) == 0 {
			// Either --skip-non-fhir and this file is not a FHIR resource, or
			// --since and it did not change. Applies to an explicitly named file
			// too: tooling that shells out to fhirlint (pre-commit) passes
			// concrete paths, so filtering only directories would miss the case
			// these flags exist for.
			results, err = nil, nil
		} else {
			// Apply --extract and --ignore on JSON input
			if flagExtract != "" || len(flagIgnore) > 0 {
				if err := preprocessJSON(in); err != nil {
					return err
				}
			}
			singleOpts := optsWithProfileMap(opts, in.Path, profileMap)
			singleOpts = mergeOverrideOpts(singleOpts, matchingOverrides(in.Path, overrides))
			rs, rerr2 := runWithCache([]string{in.Path}, singleOpts)
			if rerr2 != nil {
				return rerr2
			}
			rs[0].Label = in.Label
			results = rs

			// If --bundle-entries, also validate each entry.resource as a standalone resource.
			if flagBundleEntries {
				entryIns, berr := expandBundleEntries(in.Path)
				if berr != nil {
					return berr
				}
				if len(entryIns) > 0 {
					defer func() {
						for _, t := range entryIns {
							t.Cleanup()
						}
					}()
					entryResults, eerr := validateEntryInputs(entryIns, in.Path, opts, profileMap, overrides)
					if eerr != nil {
						return eerr
					}
					results = append(results, entryResults...)
				}
			}
		}
	}
	if err != nil {
		return err
	}

	// A replay miss must fail the run. The JAR treats some terminology failures
	// as warnings and carries on, so a silently incomplete recording would
	// otherwise produce a green result that skipped real terminology checks.
	if txPlayer != nil {
		if err := txMissError(txPlayer.Misses(), txRecordingDir(flagTxDir), arg); err != nil {
			return err
		}
	}

	// Say so explicitly: an empty run and a run that found nothing wrong both
	// exit 0, and only the message distinguishes them.
	if flagSince != "" && len(results) == 0 {
		fmt.Fprintf(os.Stderr, "No changed files to validate (--since %s)\n", flagSince)
	}

	// Apply custom FHIRPath rules and built-in style/naming lint rules before
	// suppression so their findings flow through suppression, baseline, severity
	// filtering and every reporter.
	if cerr := applyCustomChecks(cmd, results); cerr != nil {
		return cerr
	}

	// Check referential integrity across the whole validated set (dangling refs).
	if flagCheckReferences {
		applyReferenceCheck(results, sinceExcluded)
	}

	// Re-level findings before anything reads their severity: suppression, the
	// severity filter, baseline, --fail-on and the exit code all have to agree
	// on what a finding is, and they only do if the override lands first (#311).
	severityRules, oerr := buildSeverityOverrides()
	if oerr != nil {
		return oerr
	}
	if len(severityRules) > 0 {
		for i, o := range suppress.ApplySeverity(results, severityRules) {
			reportRuleOutcome(o, "severity-override", severityRules[i].Rule,
				fmt.Sprintf("%s → %s", severityRules[i].Raw, severityRules[i].To))
		}
	}

	// Apply suppression rules (CLI flags + config file).
	suppressRules, serr := buildSuppressRules(cmd)
	if serr != nil {
		return serr
	}
	if len(suppressRules) > 0 {
		outcomes := suppress.Apply(results, suppressRules)
		for i, o := range outcomes {
			reportRuleOutcome(o, "suppress", suppressRules[i], suppressRules[i].Raw)
		}
	}

	// Apply per-file override post-processing: suppress, severity filter, fail-on tracking.
	// Must run after global suppress so that override suppress appends rather than overwrites.
	neverFailPaths := applyOverridePostProcessing(results, overrides)

	// Apply baseline suppression: issues recorded in the baseline are moved to
	// r.Suppressed and do not contribute to the exit code.
	var staleBaselineEntries int
	if flagBaseline != "" {
		bf, berr := baseline.Read(flagBaseline)
		if berr != nil {
			return fmt.Errorf("reading baseline %s: %w", flagBaseline, berr)
		}
		if bf != nil {
			staleBaselineEntries = baseline.Apply(results, bf)
		}
	}

	// Generate (or update) the baseline from the current active issues.
	if flagGenerateBaseline != "" {
		bf := baseline.Generate(results)
		if werr := baseline.Write(flagGenerateBaseline, bf); werr != nil {
			return fmt.Errorf("writing baseline %s: %w", flagGenerateBaseline, werr)
		}
		fmt.Fprintf(os.Stderr, "Baseline generated: %d entries written to %s\n", len(bf.Entries), flagGenerateBaseline)
	}

	// Strip resource-derived content before anything renders it. This sits
	// after the baseline and cache stages on purpose: both work off the real
	// findings, and only what leaves the process is redacted.
	if flagRedact {
		redact.Apply(results)
		// --show-source is not an error alongside --redact, it simply loses.
		// Failing here would turn a config that is merely over-specified into a
		// broken pipeline, and the safe reading is never in doubt.
		flagShowSource = false
	}

	// Render output(s)
	for _, format := range flagFormat {
		switch strings.ToLower(format) {
		case "terminal":
			// Grouped output replaces the per-file blocks rather than adding to
			// them: printing both would be strictly more noise than today.
			if flagGroup {
				reporter.TerminalGrouped(results, flagSeverity, flagShowSuppressed, flagShowSource)
			} else {
				for _, r := range results {
					reporter.Terminal(r, flagSeverity, flagShowSuppressed, flagQuiet, flagShowSource)
				}
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
		case "markdown", "md":
			outFile := outputFile("md")
			if err := reporter.Markdown(results, flagSeverity, outFile); err != nil {
				return fmt.Errorf("markdown report: %w", err)
			}
		case "github":
			// Annotations must land on the job's stdout for the runner to parse
			// them, so --output does not apply here.
			if err := reporter.GitHub(results, flagSeverity); err != nil {
				return err
			}
		case "codeclimate":
			outFile := outputFile("json")
			if err := reporter.CodeClimate(results, flagSeverity, outFile); err != nil {
				return fmt.Errorf("codeclimate report: %w", err)
			}
		default:
			return fmt.Errorf("unknown format %q — use: terminal, json, html, junit, sarif, markdown, codeclimate, github", format)
		}
	}

	// Say it once, on stderr, after every format has been written. A reader who
	// opens an archived report should never have to wonder whether the terse
	// findings are all the validator had to say.
	if flagRedact {
		fmt.Fprintln(os.Stderr,
			"note: --redact applied — message text and source lines were removed from all reports")
	}

	// Handle fhirlint.lock: verify existing lock, or write/update when --lock is set.
	allIGs := collectAllIGs(opts.IGs, overrides)
	if flagLock {
		if lerr := runLockWrite(allIGs); lerr != nil {
			return lerr
		}
	} else if lerr := runLockVerify(allIGs); lerr != nil {
		return lerr
	}

	printUpdateNotice()

	if staleBaselineEntries > 0 {
		fmt.Fprintf(os.Stderr, "warn: %d baseline occurrence(s) no longer found — run --generate-baseline to update\n", staleBaselineEntries)
	}

	// When generating a baseline the build always succeeds: the point is to
	// record the current state, not to enforce it.
	if flagGenerateBaseline != "" {
		return nil
	}

	if err := checkRunBounds(results); err != nil {
		return err
	}
	if err := checkMaxWarnings(results, neverFailPaths); err != nil {
		return err
	}
	return checkExitCode(results, neverFailPaths)
}

// boundMarkers are the CLI option names the validator quotes when it stops early
// because --validation-timeout or --max-messages was reached. Matching on the
// option name rather than the surrounding sentence keeps this working under
// --locale, where the prose around it is translated but the flag name is not.
var boundMarkers = map[string]string{
	"-validation-timeout":      "--validation-timeout",
	"-max-validation-messages": "--max-messages",
}

// checkRunBounds fails the run when a bound cut validation short.
//
// This matters more than it sounds: when the validator stops early it returns
// only the messages gathered so far, so files with real errors come back with
// none and are counted as valid. A capped run over broken input otherwise
// reports "Valid: 2, Errors: 0" and exits 0 — a green pipeline over data that
// does not validate. Partial results must not be indistinguishable from clean
// ones, so hitting a bound is reported as inconclusive rather than passing.
//
// --fail-on never still wins: it is the explicit "do not fail this run" switch.
func checkRunBounds(results []*validator.Result) error {
	if flagFailOn == "never" {
		return nil
	}
	for _, r := range results {
		for _, issue := range r.Issues {
			for marker, flag := range boundMarkers {
				if strings.Contains(issue.Message, marker) {
					return fmt.Errorf(
						"validation stopped early because %s was reached, so the results are partial "+
							"and files with errors may be reported as valid — raise the bound, or set "+
							"--fail-on never to accept partial results", flag)
				}
			}
		}
	}
	return nil
}

// collectAllIGs gathers IGs from the base options and any validator-level overrides.
func collectAllIGs(base []string, overrides []configOverride) []string {
	seen := make(map[string]struct{}, len(base))
	out := append([]string{}, base...)
	for _, ig := range base {
		seen[ig] = struct{}{}
	}
	for _, ov := range overrides {
		for _, ig := range ov.IGs {
			if _, ok := seen[ig]; !ok {
				seen[ig] = struct{}{}
				out = append(out, ig)
			}
		}
	}
	return out
}

// runLockWrite writes or updates fhirlint.lock with current IG hashes and the
// validator version in use.
func runLockWrite(igs []string) error {
	lf, err := iglock.Read(iglock.LockFileName)
	if err != nil {
		return fmt.Errorf("reading %s: %w", iglock.LockFileName, err)
	}
	if lf == nil {
		lf = &iglock.LockFile{Packages: make(map[string]iglock.Entry)}
	}
	n, err := iglock.Update(lf, igs)
	if err != nil {
		return err
	}
	// Record the validator too, so the lock covers every input that can change
	// the result. This is also why an unchanged package set no longer short-
	// circuits: the validator may still need recording.
	running := validator.ValidatorVersion()
	validatorChanged := running != "" && lf.Validator != running
	if validatorChanged {
		lf.Validator = running
	}
	if n == 0 && !validatorChanged {
		return nil
	}
	if err := iglock.Write(iglock.LockFileName, lf); err != nil {
		return fmt.Errorf("writing %s: %w", iglock.LockFileName, err)
	}
	fmt.Fprintf(os.Stderr, "Lock file updated: %s (%d package(s), validator %s)\n",
		iglock.LockFileName, n, lf.Validator)
	return nil
}

// runLockVerify verifies IGs and the validator version against fhirlint.lock
// when the file exists.
func runLockVerify(igs []string) error {
	lf, err := iglock.Read(iglock.LockFileName)
	if err != nil {
		return fmt.Errorf("reading %s: %w", iglock.LockFileName, err)
	}
	if lf == nil {
		return nil
	}
	if err := iglock.VerifyValidator(lf, validator.ValidatorVersion(), os.Stderr); err != nil {
		return err
	}
	return iglock.Verify(lf, igs, os.Stderr)
}

// collectFHIRPaths returns all .json/.xml file paths for the given input,
// skipping any paths that match an exclude pattern.
func collectFHIRPaths(in *input.Input, excludePatterns []string) ([]string, error) {
	if in.Source != input.SourceDir {
		return []string{in.Path}, nil
	}
	var paths []string
	err := filepath.WalkDir(in.Path, func(fpath string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(in.Path, fpath)
		if relErr != nil {
			rel = fpath
		}
		relSlash := filepath.ToSlash(rel)
		for _, pat := range excludePatterns {
			if matchesExclude(relSlash, pat) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(fpath))
		if ext != ".json" && ext != ".xml" && ext != ".ndjson" {
			return nil
		}
		paths = append(paths, fpath)
		return nil
	})
	return paths, err
}

// collectPathsFromArgs expands multiple positional arguments — any mix of files
// and directories — into a single de-duplicated list of FHIR file paths, keeping
// the order in which they were given. Stdin ("-") is rejected: it cannot be
// combined with other inputs.
func collectPathsFromArgs(args []string, excludePatterns []string) ([]string, error) {
	var paths []string
	seen := make(map[string]bool)
	for _, a := range args {
		if a == "-" {
			return nil, fmt.Errorf("stdin (\"-\") cannot be combined with other inputs; pass it on its own")
		}
		in, err := input.Resolve(a, "", 0)
		if err != nil {
			return nil, err
		}
		found, err := collectFHIRPaths(in, excludePatterns)
		if err != nil {
			return nil, err
		}
		for _, p := range found {
			if seen[p] {
				continue
			}
			seen[p] = true
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// matchesExclude reports whether relPath (forward-slash, relative to walk root)
// matches the given gitignore-style pattern.
func matchesExclude(relPath, pattern string) bool {
	pattern = filepath.ToSlash(strings.TrimSpace(pattern))
	if pattern == "" {
		return false
	}
	// Trailing "/" or "/**" → directory prefix match
	if strings.HasSuffix(pattern, "/**") {
		dir := strings.TrimSuffix(pattern, "/**")
		return relPath == dir || strings.HasPrefix(relPath, dir+"/")
	}
	if strings.HasSuffix(pattern, "/") {
		dir := strings.TrimSuffix(pattern, "/")
		return relPath == dir || strings.HasPrefix(relPath, dir+"/")
	}
	// Try matching the full relative path
	if matched, _ := path.Match(pattern, relPath); matched {
		return true
	}
	// Patterns without "/" are matched against the basename only
	if !strings.Contains(pattern, "/") {
		if matched, _ := path.Match(pattern, path.Base(relPath)); matched {
			return true
		}
	}
	return false
}

// loadProfileMap parses the profile-map config key into a map[resourceTypeOrGlob][]profileURL.
func loadProfileMap() map[string][]string {
	raw := viper.Get("profile-map")
	if raw == nil {
		return nil
	}
	m, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	result := make(map[string][]string, len(m))
	for k, v := range m {
		switch tv := v.(type) {
		case string:
			result[k] = []string{tv}
		case []interface{}:
			ps := make([]string, 0, len(tv))
			for _, p := range tv {
				if s, ok := p.(string); ok {
					ps = append(ps, s)
				}
			}
			result[k] = ps
		}
	}
	return result
}

// loadOverrides parses the overrides: config key into a slice of configOverride.
//
// Parse failures are returned rather than skipped: a malformed rule used to be
// dropped in silence, so a typo simply made the override stop working with no
// indication why (#254).
func loadOverrides() ([]configOverride, error) {
	raw := viper.Get("overrides")
	if raw == nil {
		return nil, nil
	}
	items, ok := raw.([]interface{})
	if !ok {
		return nil, nil
	}
	var overrides []configOverride
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		ov := configOverride{
			Files:    toStringSlice(m["files"]),
			IGs:      toStringSlice(m["ig"]),
			Profiles: toStringSlice(m["profile"]),
		}
		if v, ok := m["best-practice"].(string); ok {
			ov.BestPractice = v
		}
		if v, ok := m["severity"].(string); ok {
			ov.Severity = v
		}
		if v, ok := m["fail-on"].(string); ok {
			ov.FailOn = v
		}
		if v, ok := m["suppress"]; ok {
			rules, err := parseSuppressFromConfig(v)
			if err != nil {
				return nil, fmt.Errorf("overrides: %w", err)
			}
			ov.Suppress = rules
		}
		if len(ov.Files) > 0 {
			overrides = append(overrides, ov)
		}
	}
	return overrides, nil
}

// toStringSlice converts a config value to []string.
func toStringSlice(v interface{}) []string {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case string:
		return []string{t}
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// matchingOverrides returns overrides whose file globs match filePath.
// Overrides are returned in declaration order.
func matchingOverrides(filePath string, overrides []configOverride) []configOverride {
	if len(overrides) == 0 {
		return nil
	}
	relSlash := filepath.ToSlash(filePath)
	var matched []configOverride
	for _, ov := range overrides {
		for _, pattern := range ov.Files {
			if matchesExclude(relSlash, pattern) {
				matched = append(matched, ov)
				break
			}
		}
	}
	return matched
}

// mergeOverrideOpts applies the JVM-level fields from matched overrides to opts.
// Later overrides win on scalar conflicts; slice fields are appended.
func mergeOverrideOpts(opts validator.Options, matched []configOverride) validator.Options {
	for _, ov := range matched {
		if len(ov.IGs) > 0 {
			opts.IGs = append(append([]string{}, opts.IGs...), ov.IGs...)
		}
		if len(ov.Profiles) > 0 {
			opts.Profiles = append(append([]string{}, opts.Profiles...), ov.Profiles...)
		}
		if ov.BestPractice != "" {
			opts.BestPractice = ov.BestPractice
		}
	}
	return opts
}

// optsGroupKey returns a stable string that uniquely identifies the JVM-level
// options so that files with identical options can share a JVM invocation.
func optsGroupKey(opts validator.Options) string {
	return strings.Join([]string{
		strings.Join(opts.Profiles, "\x01"),
		strings.Join(opts.IGs, "\x01"),
		opts.BestPractice,
	}, "\x00")
}

// hasValidatorOverrides reports whether any override has fields that affect the JVM invocation.
func hasValidatorOverrides(overrides []configOverride) bool {
	for _, ov := range overrides {
		if len(ov.IGs) > 0 || len(ov.Profiles) > 0 || ov.BestPractice != "" {
			return true
		}
	}
	return false
}

// applyOverridePostProcessing applies per-file severity filtering and suppress
// rules from matching overrides. It returns the set of file paths whose issues
// should not contribute to the exit code (fail-on: never).
// Must be called after global suppress.Apply so that override suppress appends
// to r.Suppressed rather than overwriting it.
func applyOverridePostProcessing(results []*validator.Result, overrides []configOverride) map[string]struct{} {
	if len(overrides) == 0 {
		return nil
	}
	var neverFail map[string]struct{}
	for _, r := range results {
		matched := matchingOverrides(r.Filename, overrides)
		if len(matched) == 0 {
			continue
		}
		for _, ov := range matched {
			if len(ov.Suppress) > 0 {
				applySuppressToResult(r, ov.Suppress)
			}
			if ov.Severity != "" {
				r.Issues = filterIssuesBySeverity(r.Issues, ov.Severity)
				r.Valid = issuesValid(r.Issues)
			}
			if ov.FailOn == "never" {
				if neverFail == nil {
					neverFail = make(map[string]struct{})
				}
				neverFail[r.Filename] = struct{}{}
			}
		}
	}
	return neverFail
}

// applySuppressToResult applies rules to a single result, appending to r.Suppressed
// (instead of replacing) to preserve suppression from earlier passes.
//
// Expiry is checked here as well as in suppress.ApplyAt: override rules do not
// go through that function, and an expired rule must not keep suppressing just
// because it was written under `overrides:` (#252).
func applySuppressToResult(r *validator.Result, rules []suppress.Rule) {
	now := time.Now()
	var active []validator.Issue
	for _, issue := range r.Issues {
		matched := false
		for _, rule := range rules {
			if rule.ExpiredAt(now) {
				continue
			}
			if rule.Matches(issue) {
				issue.SuppressReason = rule.Reason
				r.Suppressed = append(r.Suppressed, issue)
				matched = true
				break
			}
		}
		if !matched {
			active = append(active, issue)
		}
	}
	r.Issues = active
	r.Valid = issuesValid(r.Issues)
}

var overrideSeverityLevels = map[string]int{
	"information": 0,
	"warning":     1,
	"error":       2,
	"fatal":       3,
}

// filterIssuesBySeverity keeps only issues at or above minSeverity.
func filterIssuesBySeverity(issues []validator.Issue, minSeverity string) []validator.Issue {
	min := overrideSeverityLevels[strings.ToLower(minSeverity)]
	var out []validator.Issue
	for _, iss := range issues {
		if overrideSeverityLevels[iss.Severity] >= min {
			out = append(out, iss)
		}
	}
	return out
}

// issuesValid returns true when none of the issues are error or fatal severity.
func issuesValid(issues []validator.Issue) bool {
	for _, iss := range issues {
		if iss.Severity == "error" || iss.Severity == "fatal" {
			return false
		}
	}
	return true
}

// isFilenameGlob returns true when the profile-map key looks like a filename
// glob (contains /, *, ?, .) rather than a FHIR resource type name.
func isFilenameGlob(pattern string) bool {
	return strings.ContainsAny(pattern, "*/?.\\")
}

// peekResourceType reads the resourceType field from a JSON file without
// parsing the entire document.
func peekResourceType(filePath string) string {
	f, err := os.Open(filePath) //nolint:gosec // user-supplied path validated upstream
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	var v struct {
		ResourceType string `json:"resourceType"`
	}
	_ = json.NewDecoder(io.LimitReader(f, 4096)).Decode(&v)
	return v.ResourceType
}

// fhirPeekBytes is how much of a file looksLikeFHIR inspects. FHIR resources
// carry their marker in the first object/element, so a small prefix is enough.
const fhirPeekBytes = 4096

// fhirXMLNamespace is the namespace every FHIR XML resource is served in.
const fhirXMLNamespace = "http://hl7.org/fhir"

// looksLikeFHIR reports whether path is plausibly a FHIR resource. It backs
// --skip-non-fhir, which drops unrelated files (package.json, tsconfig.json,
// ...) before validation.
//
// It deliberately errs towards true: only a file that parses cleanly and
// demonstrably lacks the FHIR marker is rejected. Anything malformed,
// truncated or unreadable is kept, so a genuinely broken resource still
// reaches the validator and fails loudly instead of vanishing from the report.
func looksLikeFHIR(filePath string) bool {
	f, err := os.Open(filePath) //nolint:gosec // path already resolved by the caller
	if err != nil {
		return true // unreadable — let the validator report it
	}
	defer func() { _ = f.Close() }()

	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".json":
		var v any
		if err := json.NewDecoder(io.LimitReader(f, fhirPeekBytes)).Decode(&v); err != nil {
			// Malformed, or valid but larger than the peek window: not our call.
			return true
		}
		obj, ok := v.(map[string]any)
		if !ok {
			// Valid JSON that is not an object (array, scalar, null). A FHIR
			// resource is always an object, so this is definitively not one.
			return false
		}
		rt, _ := obj["resourceType"].(string)
		return rt != ""
	case ".xml":
		dec := xml.NewDecoder(io.LimitReader(f, fhirPeekBytes))
		for {
			tok, err := dec.Token()
			if err != nil {
				return true
			}
			if se, ok := tok.(xml.StartElement); ok {
				return se.Name.Space == fhirXMLNamespace
			}
		}
	default:
		// NDJSON (a bulk export is FHIR by definition) and anything else.
		return true
	}
}

// filterFHIRPaths drops non-FHIR files when --skip-non-fhir is set and reports
// on stderr how many were dropped — skipping inputs silently would be the very
// blind spot this flag is meant to avoid.
// maxReportedMisses bounds the replay misses listed in an error. A recording
// that is badly out of date can miss on hundreds of codes, and a wall of them
// helps nobody decide what to do.
const maxReportedMisses = 10

// ensureNoUserFHIRSettings rejects a passthrough -fhir-settings while replaying.
//
// --tx-offline has to generate its own settings file to exempt the loopback
// replay server from the validator's SSRF protection, and the JAR takes a single
// -fhir-settings. Silently overriding the user's file would be worse than
// saying so.
func ensureNoUserFHIRSettings(extra []string) error {
	for _, a := range extra {
		name := a
		if i := strings.IndexByte(name, '='); i >= 0 {
			name = name[:i]
		}
		if strings.EqualFold(strings.TrimLeft(name, "-"), "fhir-settings") {
			return fmt.Errorf("--tx-offline generates its own -fhir-settings to reach the local replay server, so it cannot be combined with --validator-arg %q", a)
		}
	}
	return nil
}

// warnValidatorVersionDrift points out that a recording was made with a
// different validator. Which terminology requests get made is a property of the
// validator — 6.10.0 changed how code systems are resolved — so a mismatch is
// the likeliest explanation for misses that otherwise look inexplicable.
func warnValidatorVersionDrift(w io.Writer, m *txreplay.Manifest, current string) {
	if m == nil || m.ValidatorVersion == "" || current == "" || m.ValidatorVersion == current {
		return
	}
	_, _ = fmt.Fprintf(w, "warn: recording was made with validator %s, this run uses %s — re-record if requests come up missing\n",
		m.ValidatorVersion, current)
}

// txMissError turns unreplayable terminology requests into an actionable error.
// It returns nil when there were none.
func txMissError(misses []txreplay.Miss, dir, arg string) error {
	if len(misses) == 0 {
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d terminology request(s) were not in the recording in %s/:\n", len(misses), dir)
	for i, m := range misses {
		if i == maxReportedMisses {
			fmt.Fprintf(&b, "  … and %d more\n", len(misses)-maxReportedMisses)
			break
		}
		fmt.Fprintf(&b, "  %s\n", m)
	}
	fmt.Fprintf(&b, "Re-record with: fhirlint tx warm %s", arg)
	return errors.New(b.String())
}

// sinceChanged holds the absolute paths --since resolved from git, or nil when
// the flag is not in use. sinceExcluded collects the FHIR files that were
// dropped because they did not change, so --check-references can still index
// them (see filterSincePaths).
var (
	sinceChanged  map[string]bool
	sinceExcluded []string
)

// filterSincePaths drops paths that are unchanged against --since.
//
// Dropped paths are remembered rather than forgotten: the reference graph spans
// the whole dataset, so --check-references over a changed subset alone would
// report every reference into the unchanged remainder as unresolved. The
// excluded files are indexed for reference resolution without being validated.
func filterSincePaths(paths []string) []string {
	if sinceChanged == nil {
		return paths
	}
	kept := make([]string, 0, len(paths))
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			// Only fails if the working directory is gone. Validate rather than
			// silently skip — too much work is recoverable, a missed error is not.
			kept = append(kept, p)
			continue
		}
		if sinceChanged[abs] {
			kept = append(kept, p)
			continue
		}
		sinceExcluded = append(sinceExcluded, p)
	}
	if n := len(paths) - len(kept); n > 0 {
		fmt.Fprintf(os.Stderr, "Skipped %d unchanged file(s) (--since %s)\n", n, flagSince)
	}
	return kept
}

func filterFHIRPaths(paths []string) []string {
	if !flagSkipNonFHIR {
		return paths
	}
	kept := make([]string, 0, len(paths))
	for _, p := range paths {
		if looksLikeFHIR(p) {
			kept = append(kept, p)
		}
	}
	if n := len(paths) - len(kept); n > 0 {
		fmt.Fprintf(os.Stderr, "Skipped %d non-FHIR file(s) (--skip-non-fhir)\n", n)
	}
	return kept
}

// resolveProfilesForPath returns the extra profiles that should be applied to
// filePath based on the profile-map. Keys are matched as FHIR resource type
// names first, then as filename globs.
func resolveProfilesForPath(filePath string, profileMap map[string][]string) []string {
	if len(profileMap) == 0 {
		return nil
	}
	relSlash := filepath.ToSlash(filePath)
	var extra []string
	// Resource type match (non-glob keys).
	rt := peekResourceType(filePath)
	if rt != "" {
		if ps, ok := profileMap[rt]; ok {
			extra = append(extra, ps...)
		}
	}
	// Filename glob match.
	for pattern, ps := range profileMap {
		if !isFilenameGlob(pattern) {
			continue
		}
		if matchesExclude(relSlash, pattern) {
			extra = append(extra, ps...)
		}
	}
	return extra
}

// optsWithProfileMap returns opts with extra profiles from profileMap appended.
func optsWithProfileMap(opts validator.Options, filePath string, profileMap map[string][]string) validator.Options {
	extra := resolveProfilesForPath(filePath, profileMap)
	if len(extra) == 0 {
		return opts
	}
	opts.Profiles = append(append([]string{}, opts.Profiles...), extra...)
	return opts
}

// loadIgnoreFile reads a gitignore-style ignore file and returns its patterns.
func loadIgnoreFile(filename string) ([]string, error) {
	f, err := os.Open(filename) //nolint:gosec // filename is a constant (".fhirlintignore")
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var patterns []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns, sc.Err()
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

	results, err := runValidation(paths, opts)
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

	results, err := runValidation(paths, opts)
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

// onceWarner prints at most one warning, however many times it is called.
//
// A broken result cache fails identically for every file in the run, so
// reporting each one would bury the validation output under hundreds of copies
// of the same line — which is its own kind of unreadable.
type onceWarner struct {
	w    io.Writer
	sent bool
}

func (o *onceWarner) warn(format string, args ...interface{}) {
	if o.sent {
		return
	}
	o.sent = true
	_, _ = fmt.Fprintf(o.w, format, args...)
}

// runWithCache validates the given paths, consulting the result cache when --cache is enabled.
// Cached results are used as-is; uncached paths are passed to the validator.
// Fresh results are written back to the cache for future runs.
func runWithCache(paths []string, opts validator.Options) ([]*validator.Result, error) {
	if !flagCache || len(paths) == 0 {
		return runValidation(paths, opts)
	}

	// --cache must never fail a run: it is an optimisation, and a validation
	// result does not depend on it. It must not fail *quietly* either — a cache
	// that cannot be written looks exactly like a cache that keeps missing, and
	// the user goes on paying full validation cost for a flag they asked for
	// (#316). One warning per run, not one per file.
	warn := onceWarner{w: os.Stderr}

	cacheDir := flagCacheDir
	if cacheDir == "" {
		dir, err := cache.ResultCacheDir()
		if err != nil {
			warn.warn("warn: --cache is set but the cache directory could not be resolved (%v) — running without it\n", err)
			return runValidation(paths, opts)
		}
		cacheDir = dir
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
			// A missing entry is the normal case and says nothing. Anything else
			// — no permission, an unreadable directory — is worth one line.
			if !os.IsNotExist(err) {
				warn.warn("warn: --cache is set but the cache could not be read (%v) — validating without it\n", err)
			}
			uncachedPaths = append(uncachedPaths, p)
			uncachedIdx = append(uncachedIdx, i)
		}
	}

	var fresh []*validator.Result
	if len(uncachedPaths) > 0 {
		var err error
		fresh, err = runValidation(uncachedPaths, opts)
		if err != nil {
			return nil, err
		}
		for j, r := range fresh {
			i := uncachedIdx[j]
			if keys[i] != "" {
				if err := resultcache.Put(cacheDir, keys[i], resultcache.Entry{
					CachedAt:        timeNow(),
					FhirlintVersion: keyOpts.FhirlintVersion,
					Result:          *r,
				}); err != nil {
					warn.warn("warn: --cache is set but results could not be written to %s (%v) — the next run will revalidate\n",
						cacheDir, err)
				}
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

// bundleEntryJSON holds just enough of an entry to extract the resource.
type bundleEntryJSON struct {
	Resource json.RawMessage `json:"resource"`
}

type bundleJSON struct {
	ResourceType string            `json:"resourceType"`
	Entry        []bundleEntryJSON `json:"entry"`
}

// expandBundleEntries reads path, and if it is a FHIR Bundle, writes each
// entry.resource to a temp file. Returns nil when the file is not a Bundle or
// has no entries. Labels use "basename → entry[N] (ResourceType/id)" format.
// The caller must call Cleanup() on every returned Input.
func expandBundleEntries(path string) ([]*input.Input, error) {
	data, err := os.ReadFile(path) //nolint:gosec // validated upstream
	if err != nil {
		return nil, err
	}
	var b bundleJSON
	if err := json.Unmarshal(data, &b); err != nil || b.ResourceType != "Bundle" {
		return nil, nil
	}
	base := filepath.Base(path)
	var ins []*input.Input
	for i, entry := range b.Entry {
		if len(entry.Resource) == 0 || string(entry.Resource) == "null" {
			continue
		}
		var res struct {
			ResourceType string `json:"resourceType"`
			ID           string `json:"id"`
		}
		_ = json.Unmarshal(entry.Resource, &res)

		label := fmt.Sprintf("%s → entry[%d]", base, i)
		if res.ResourceType != "" {
			if res.ID != "" {
				label = fmt.Sprintf("%s → entry[%d] (%s/%s)", base, i, res.ResourceType, res.ID)
			} else {
				label = fmt.Sprintf("%s → entry[%d] (%s)", base, i, res.ResourceType)
			}
		}

		f, ferr := os.CreateTemp("", "fhirlint-entry-*.json")
		if ferr != nil {
			for _, t := range ins {
				t.Cleanup()
			}
			return nil, fmt.Errorf("entry %d: %w", i, ferr)
		}
		_, writeErr := f.Write(entry.Resource)
		closeErr := f.Close()
		if writeErr != nil || closeErr != nil {
			_ = os.Remove(f.Name())
			for _, t := range ins {
				t.Cleanup()
			}
			if writeErr != nil {
				return nil, writeErr
			}
			return nil, closeErr
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

// preprocessedInput applies --extract and --ignore to path by writing the result
// to a temp file. Returns an Input pointing to the temp file on success.
// If no preprocessing flags are set, returns an Input wrapping the original path.
// The caller must call Cleanup() on the returned Input.
func preprocessedInput(path string) (*input.Input, error) {
	if flagExtract == "" && len(flagIgnore) == 0 {
		return &input.Input{Source: input.SourceFile, Path: path, Label: path}, nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // validated upstream
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(strings.TrimSpace(string(data)), "<") {
		return preprocessedXMLInput(path, data)
	}
	raw := string(data)
	if flagExtract != "" {
		p := gjsonPath(flagExtract)
		extracted := gjson.Get(raw, p)
		if !extracted.Exists() {
			return nil, fmt.Errorf("--extract: path %q not found in input", flagExtract)
		}
		raw = extracted.Raw
	}
	for _, ign := range flagIgnore {
		raw = deleteJSONPath(raw, gjsonPath(ign))
	}
	f, ferr := os.CreateTemp("", "fhirlint-dir-*.json")
	if ferr != nil {
		return nil, fmt.Errorf("creating temp file: %w", ferr)
	}
	_, writeErr := f.WriteString(raw)
	closeErr := f.Close()
	if writeErr != nil {
		_ = os.Remove(f.Name())
		return nil, writeErr
	}
	if closeErr != nil {
		_ = os.Remove(f.Name())
		return nil, closeErr
	}
	return &input.Input{
		Source:   input.SourceFile,
		Path:     f.Name(),
		TempFile: f.Name(),
		Label:    path,
	}, nil
}

// preprocessedXMLInput applies --extract and/or --ignore to XML data and writes
// the result to a temp .xml file. The caller must call Cleanup() on the returned Input.
func preprocessedXMLInput(origPath string, data []byte) (*input.Input, error) {
	result := data
	var xmlErr error
	if flagExtract != "" {
		result, xmlErr = xmlExtract(result, flagExtract)
		if xmlErr != nil {
			return nil, xmlErr
		}
	}
	if len(flagIgnore) > 0 {
		paths := make([][]string, len(flagIgnore))
		for i, ign := range flagIgnore {
			paths[i] = xmlPathSegments(ign)
		}
		result, xmlErr = xmlDeletePaths(result, paths)
		if xmlErr != nil {
			return nil, fmt.Errorf("--ignore on XML: %w", xmlErr)
		}
	}
	f, ferr := os.CreateTemp("", "fhirlint-dir-*.xml")
	if ferr != nil {
		return nil, fmt.Errorf("creating temp file: %w", ferr)
	}
	_, writeErr := f.Write(result)
	closeErr := f.Close()
	if writeErr != nil {
		_ = os.Remove(f.Name())
		return nil, writeErr
	}
	if closeErr != nil {
		_ = os.Remove(f.Name())
		return nil, closeErr
	}
	return &input.Input{
		Source:   input.SourceFile,
		Path:     f.Name(),
		TempFile: f.Name(),
		Label:    origPath,
	}, nil
}

// validateEntryInputs validates a set of bundle entry temp files, applying
// profile-map per resource type and overrides via the bundle's source path.
// Results get the entry label and bundlePath as Filename.
func validateEntryInputs(ins []*input.Input, bundlePath string, opts validator.Options, profileMap map[string][]string, overrides []configOverride) ([]*validator.Result, error) {
	type group struct {
		paths  []string
		labels []string
		opts   validator.Options
	}
	groupMap := map[string]*group{}
	for _, in := range ins {
		g := optsWithProfileMap(opts, in.Path, profileMap) // resource-type based profile lookup
		g = mergeOverrideOpts(g, matchingOverrides(bundlePath, overrides))
		key := optsGroupKey(g)
		if _, ok := groupMap[key]; !ok {
			groupMap[key] = &group{opts: g}
		}
		groupMap[key].paths = append(groupMap[key].paths, in.Path)
		groupMap[key].labels = append(groupMap[key].labels, in.Label)
	}

	keys := make([]string, 0, len(groupMap))
	for k := range groupMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var results []*validator.Result
	for _, k := range keys {
		g := groupMap[k]
		rs, err := runWithCache(g.paths, g.opts)
		if err != nil {
			return nil, err
		}
		for i, r := range rs {
			r.Filename = bundlePath
			r.Label = g.labels[i]
		}
		results = append(results, rs...)
	}
	return results, nil
}

// validateNDJSON splits an NDJSON file into per-line temp files, optionally
// applies --extract / --ignore preprocessing, and validates them in a single JVM invocation.
func validateNDJSON(path string, opts validator.Options, profileMap map[string][]string, overrides []configOverride) ([]*validator.Result, error) {
	ins, err := ndjson.Split(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		for _, t := range ins {
			t.Cleanup()
		}
	}()

	if len(ins) == 0 {
		return nil, nil
	}

	if flagExtract != "" || len(flagIgnore) > 0 {
		for _, t := range ins {
			if err := preprocessJSON(t); err != nil {
				return nil, err
			}
		}
	}

	paths := make([]string, len(ins))
	for i, t := range ins {
		paths[i] = t.Path
	}

	fileOpts := optsWithProfileMap(opts, path, profileMap)
	fileOpts = mergeOverrideOpts(fileOpts, matchingOverrides(path, overrides))
	rs, err := runWithCache(paths, fileOpts)
	if err != nil {
		return nil, err
	}
	for i, r := range rs {
		r.Label = ins[i].Label
		r.Filename = path
	}
	return rs, nil
}

// validateDir finds all .json/.xml/.ndjson files and validates them.
// Files are grouped by their merged validator options (profile-map + overrides)
// and validated in one JVM invocation per group.
// NDJSON files are expanded into per-line resources before grouping.
func validateDir(dir string, opts validator.Options, excludePatterns []string, profileMap map[string][]string, overrides []configOverride) ([]*validator.Result, error) {
	paths, err := collectFHIRPaths(&input.Input{Source: input.SourceDir, Path: dir}, excludePatterns)
	if err != nil {
		return nil, err
	}
	return validatePaths(paths, opts, profileMap, overrides)
}

// validatePaths validates an already-collected set of FHIR file paths, grouping
// them into as few JVM invocations as possible. It is shared by directory input
// and by multiple positional file arguments.
func validatePaths(paths []string, opts validator.Options, profileMap map[string][]string, overrides []configOverride) ([]*validator.Result, error) {
	paths = filterSincePaths(filterFHIRPaths(paths))
	if len(paths) == 0 {
		return nil, nil
	}

	// Separate NDJSON files from regular files — they need per-line expansion.
	var regularPaths, ndjsonPaths []string
	for _, p := range paths {
		if ndjson.IsNDJSON(p) {
			ndjsonPaths = append(ndjsonPaths, p)
		} else {
			regularPaths = append(regularPaths, p)
		}
	}

	var allResults []*validator.Result

	// Validate NDJSON files.
	for _, p := range ndjsonPaths {
		rs, err := validateNDJSON(p, opts, profileMap, overrides)
		if err != nil {
			return nil, err
		}
		allResults = append(allResults, rs...)
	}

	if len(regularPaths) == 0 {
		return allResults, nil
	}

	// Apply --extract / --ignore: preprocess each file into a temp copy.
	// Files that fail (e.g. extract path not found) produce a synthetic error
	// result and are skipped from validation.
	tempPathToOrig := make(map[string]string)
	if flagExtract != "" || len(flagIgnore) > 0 {
		var preprocessedIns []*input.Input
		for _, p := range regularPaths {
			in, perr := preprocessedInput(p)
			if perr != nil {
				allResults = append(allResults, &validator.Result{
					Filename: p,
					Label:    p,
					Valid:    false,
					Issues: []validator.Issue{{
						Severity:  "error",
						Message:   perr.Error(),
						MessageID: "fhirlint-preprocess",
					}},
				})
				continue
			}
			preprocessedIns = append(preprocessedIns, in)
			if in.TempFile != "" {
				tempPathToOrig[in.Path] = p
			}
		}
		defer func() {
			for _, in := range preprocessedIns {
				in.Cleanup()
			}
		}()
		regularPaths = make([]string, len(preprocessedIns))
		for i, in := range preprocessedIns {
			regularPaths[i] = in.Path
		}
	}

	// origPath maps a (possibly temp) validation path back to the original file path.
	origPath := func(p string) string {
		if orig, ok := tempPathToOrig[p]; ok {
			return orig
		}
		return p
	}

	if len(regularPaths) == 0 {
		return allResults, nil
	}

	// Expand bundle entries when --bundle-entries is set.
	// Entry temp files are added to regularPaths so they go through the normal
	// grouping. entryMetaMap tracks their bundle path and display label.
	type entryMeta struct {
		bundlePath string
		label      string
	}
	entryMetaMap := make(map[string]entryMeta)
	if flagBundleEntries {
		var entryInputs []*input.Input
		for _, p := range regularPaths {
			ins, berr := expandBundleEntries(origPath(p))
			if berr != nil {
				return nil, berr
			}
			for _, in := range ins {
				entryMetaMap[in.Path] = entryMeta{bundlePath: origPath(p), label: in.Label}
				entryInputs = append(entryInputs, in)
			}
		}
		if len(entryInputs) > 0 {
			defer func() {
				for _, in := range entryInputs {
					in.Cleanup()
				}
			}()
			for _, in := range entryInputs {
				regularPaths = append(regularPaths, in.Path)
			}
		}
	}

	// remapResult fills r.Filename and r.Label from entryMetaMap or origPath.
	remapResult := func(r *validator.Result) {
		if meta, ok := entryMetaMap[r.Filename]; ok {
			r.Filename = meta.bundlePath
			r.Label = meta.label
		} else {
			r.Filename = origPath(r.Filename)
			r.Label = r.Filename
		}
	}

	// Fast path: no per-file option overrides — single JVM invocation.
	if len(profileMap) == 0 && !hasValidatorOverrides(overrides) && len(entryMetaMap) == 0 {
		results, err := runWithCache(regularPaths, opts)
		if err != nil {
			return nil, err
		}
		for _, r := range results {
			remapResult(r)
		}
		return append(allResults, results...), nil
	}

	// Group files by their merged validator options for minimal JVM invocations.
	// Entry files use their resource type for profile-map lookup; override lookup
	// uses the bundle path. Regular files use the original (pre-preprocessing) path.
	type group struct {
		paths []string
		opts  validator.Options
	}
	groupMap := map[string]*group{}
	for _, p := range regularPaths {
		var profileLookupPath, overrideLookupPath string
		if meta, ok := entryMetaMap[p]; ok {
			profileLookupPath = p // temp file — peekResourceType reads the entry resource
			overrideLookupPath = meta.bundlePath
		} else {
			profileLookupPath = origPath(p)
			overrideLookupPath = origPath(p)
		}
		g := opts
		extra := resolveProfilesForPath(profileLookupPath, profileMap)
		if len(extra) > 0 {
			g.Profiles = append(append([]string{}, g.Profiles...), extra...)
		}
		g = mergeOverrideOpts(g, matchingOverrides(overrideLookupPath, overrides))
		key := optsGroupKey(g)
		if _, ok := groupMap[key]; !ok {
			groupMap[key] = &group{opts: g}
		}
		groupMap[key].paths = append(groupMap[key].paths, p)
	}

	keys := make([]string, 0, len(groupMap))
	for k := range groupMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		g := groupMap[k]
		rs, rerr := runWithCache(g.paths, g.opts)
		if rerr != nil {
			return nil, rerr
		}
		for _, r := range rs {
			remapResult(r)
		}
		allResults = append(allResults, rs...)
	}
	return allResults, nil
}

// preprocessJSON applies --extract and --ignore to the input file in-place.
// writePreprocessed stores preprocessed content for in without destroying a
// user-supplied file.
//
// When in already owns a temp file (stdin, --url) it is rewritten in place —
// nothing of the user's is at stake there. Otherwise a temp copy is created and
// in is repointed at it, leaving the original file untouched. Writing straight
// back to in.Path used to silently overwrite the input (#257).
func writePreprocessed(in *input.Input, data []byte) error {
	if in.TempFile != "" {
		// in.Path is a temp file this process created, not user-controlled.
		return os.WriteFile(in.Path, data, 0600) //nolint:gosec // writing back to our own temp file
	}
	f, err := os.CreateTemp("", "fhirlint-preprocessed-*"+filepath.Ext(in.Path))
	if err != nil {
		return err
	}
	name := f.Name()
	if _, werr := f.Write(data); werr != nil {
		_ = f.Close()
		_ = os.Remove(name)
		return werr
	}
	if cerr := f.Close(); cerr != nil {
		_ = os.Remove(name)
		return cerr
	}
	in.Path = name
	in.TempFile = name
	return nil
}

func preprocessJSON(in *input.Input) error {
	data, err := os.ReadFile(in.Path)
	if err != nil {
		return err
	}

	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "<") {
		result := data
		var xmlErr error
		if flagExtract != "" {
			result, xmlErr = xmlExtract(result, flagExtract)
			if xmlErr != nil {
				return xmlErr
			}
		}
		if len(flagIgnore) > 0 {
			paths := make([][]string, len(flagIgnore))
			for i, ign := range flagIgnore {
				paths[i] = xmlPathSegments(ign)
			}
			result, xmlErr = xmlDeletePaths(result, paths)
			if xmlErr != nil {
				return fmt.Errorf("--ignore on XML: %w", xmlErr)
			}
		}
		if flagExtract != "" || len(flagIgnore) > 0 {
			return writePreprocessed(in, result)
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

	return writePreprocessed(in, []byte(raw))
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
	// The check is an HTTP call to the GitHub API. --offline said no.
	if flagOffline {
		return
	}
	if newer := validator.CheckForUpdate(); newer != "" {
		fmt.Fprintf(os.Stderr, "\nA new validator version (%s) is available. Run: fhirlint update\n", newer)
	}
}

// reportRuleOutcome prints the warnings a selector rule earns: expired, unused,
// or about to lapse. kind names the config section ("suppress",
// "severity-override") and label is how the rule is shown to the user.
func reportRuleOutcome(o suppress.Outcome, kind string, rule suppress.Rule, label string) {
	verb := "suppresses"
	if kind == "severity-override" {
		verb = "changes"
	}
	switch {
	case o.Expired:
		// Say this loudly: findings this rule used to hold down are back at their
		// reported severity, and without the reason a build failing again looks
		// arbitrary.
		fmt.Fprintf(os.Stderr, "warn: %s rule %q expired on %s and no longer %s anything\n",
			kind, label, rule.Expires.Format("2006-01-02"), verb)
	case o.Matches == 0:
		fmt.Fprintf(os.Stderr, "warn: %s rule %q matched 0 issues\n", kind, label)
	}
	if o.ExpiresSoon {
		fmt.Fprintf(os.Stderr, "warn: %s rule %q expires on %s\n",
			kind, label, rule.Expires.Format("2006-01-02"))
	}
}

// buildSeverityOverrides reads the `severity-override:` list from the config.
// There is no CLI flag: a re-levelling carries a reason and usually an expiry,
// which is config, not something you retype on every invocation.
func buildSeverityOverrides() ([]suppress.SeverityRule, error) {
	if !viper.IsSet("severity-override") {
		return nil, nil
	}
	raw, ok := viper.Get("severity-override").([]interface{})
	if !ok {
		return nil, fmt.Errorf("severity-override config must be a list")
	}
	var rules []suppress.SeverityRule
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf(
				"severity-override rule must be a map with a selector and a severity, got %T", item)
		}
		r, err := suppress.ParseSeverityMap(m)
		if err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	// Same policy as suppression, deliberately: a rule that stops an error
	// failing the build needs a recorded why just as much when it downgrades the
	// finding as when it hides it.
	return rules, checkSuppressReasons(suppress.Selectors(rules), "severity-override")
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
		return rules, checkSuppressReasons(rules, "--suppress")
	}
	if viper.IsSet("suppress") {
		parsed, err := parseSuppressFromConfig(viper.Get("suppress"))
		if err != nil {
			return nil, err
		}
		return parsed, checkSuppressReasons(parsed, "suppress")
	}
	return nil, nil
}

// checkSuppressReasons enforces require-suppress-reason. It is called for every
// source of suppression rules — CLI flags, the global config, and per-file
// overrides — because a policy that only covers one of them is trivially
// sidestepped by moving the rule somewhere else.
func checkSuppressReasons(rules []suppress.Rule, source string) error {
	if !flagRequireSuppressReason {
		return nil
	}
	missing := suppress.WithoutReason(rules)
	if len(missing) == 0 {
		return nil
	}
	raws := make([]string, len(missing))
	for i, r := range missing {
		raws[i] = strconv.Quote(r.Raw)
	}
	return fmt.Errorf(
		"require-suppress-reason is set, but %d %s rule(s) have no reason: %s",
		len(missing), source, strings.Join(raws, ", "))
}

// applyCustomChecks runs the custom FHIRPath rule engine and the built-in
// style/naming lint engine against each result's resource, merging findings as
// issues. Each resource is read once and shared by both engines. Both engines
// operate on JSON only; XML resources are skipped with a notice.
func applyCustomChecks(_ *cobra.Command, results []*validator.Result) error {
	ruleEngine, err := buildRuleEngine()
	if err != nil {
		return err
	}
	lintEngine, err := buildLintEngine()
	if err != nil {
		return err
	}
	if ruleEngine == nil && lintEngine == nil {
		return nil
	}
	skippedXML := 0
	for _, res := range results {
		if res.Filename == "" {
			continue
		}
		content, rerr := os.ReadFile(res.Filename) //nolint:gosec // path from resolved input
		if rerr != nil {
			continue // temp file already cleaned up, or unreadable — nothing to check
		}
		if isXMLContent(content) {
			skippedXML++
			continue
		}
		if ruleEngine != nil {
			ruleEngine.EvaluateResult(res, content)
		}
		if lintEngine != nil {
			lintEngine.EvaluateResult(res, content)
		}
	}
	if skippedXML > 0 {
		fmt.Fprintf(os.Stderr, "warn: custom rules/lint skipped %d XML resource(s) — they support JSON input only\n", skippedXML)
	}
	return nil
}

// runValidation validates paths using either a running validator server
// (--server) or a per-run JVM, returning results in input order. Both backends
// share the same signature so every call site dispatches transparently.
func runValidation(paths []string, opts validator.Options) ([]*validator.Result, error) {
	if flagServer != "" {
		return validator.RunMultipleViaServer(flagServer, paths, opts)
	}
	return validator.RunMultiple(paths, opts)
}

// applyReferenceCheck indexes every validated JSON resource and reports literal
// references that do not resolve within the set. It reads each file once, builds
// a shared index, then checks each resource. XML resources are skipped.
// applyReferenceCheck resolves every literal reference in the validated set.
//
// indexOnly names files that are part of the dataset but were not validated —
// currently the ones --since dropped as unchanged. They are added to the
// identity index but never checked themselves, so a changed resource pointing
// into the unchanged remainder resolves instead of being reported as dangling.
func applyReferenceCheck(results []*validator.Result, indexOnly []string) {
	type parsed struct {
		res *validator.Result
		raw []byte
	}
	docs := make([]parsed, 0, len(results))
	index := refcheck.NewIndex()
	skippedXML := 0
	for _, p := range indexOnly {
		content, err := os.ReadFile(p) //nolint:gosec // path from resolved input
		if err != nil || isXMLContent(content) {
			continue
		}
		index.Add(content)
	}
	for _, res := range results {
		if res.Filename == "" {
			continue
		}
		content, err := os.ReadFile(res.Filename) //nolint:gosec // path from resolved input
		if err != nil {
			continue // temp file already cleaned up, or unreadable
		}
		if isXMLContent(content) {
			skippedXML++
			continue
		}
		index.Add(content)
		docs = append(docs, parsed{res: res, raw: content})
	}
	for _, d := range docs {
		found := refcheck.Check(d.raw, index)
		if len(found) == 0 {
			continue
		}
		d.res.Issues = append(d.res.Issues, found...)
		for _, iss := range found {
			if iss.Severity == "error" || iss.Severity == "fatal" {
				d.res.Valid = false
			}
		}
	}
	if skippedXML > 0 {
		fmt.Fprintf(os.Stderr, "warn: reference check skipped %d XML resource(s) — it supports JSON input only\n", skippedXML)
	}
}

// buildRuleEngine loads rules from --rules-file (precedence) or the rules:
// config key and compiles them. Returns nil when no rules are configured.
func buildRuleEngine() (*rules.Engine, error) {
	ruleset, err := loadRules()
	if err != nil {
		return nil, err
	}
	if len(ruleset) == 0 {
		return nil, nil
	}
	return rules.NewEngine(ruleset, rules.NewNativeEvaluator())
}

// loadRules reads rules from --rules-file if given, otherwise from the rules:
// config key.
func loadRules() ([]rules.Rule, error) {
	if flagRulesFile != "" {
		return loadRulesFromFile(flagRulesFile)
	}
	if viper.IsSet("rules") {
		return parseRulesFromConfig(viper.Get("rules"))
	}
	return nil, nil
}

// loadRulesFromFile parses a YAML file holding a rules: list (or a bare list of
// rule maps).
func loadRulesFromFile(pathArg string) ([]rules.Rule, error) {
	data, err := os.ReadFile(pathArg) //nolint:gosec // caller-supplied path
	if err != nil {
		return nil, fmt.Errorf("reading rules file %s: %w", pathArg, err)
	}
	// Accept both a document with a `rules:` key and a bare list of rule maps.
	var doc struct {
		Rules []map[string]interface{} `yaml:"rules"`
	}
	errDoc := yaml.Unmarshal(data, &doc)
	if errDoc == nil && len(doc.Rules) > 0 {
		return buildRuleList(pathArg, doc.Rules)
	}
	var list []map[string]interface{}
	errList := yaml.Unmarshal(data, &list)
	if errList == nil && len(list) > 0 {
		return buildRuleList(pathArg, list)
	}
	if errDoc != nil && errList != nil {
		return nil, fmt.Errorf("parsing rules file %s: %w", pathArg, errList)
	}
	return nil, fmt.Errorf("rules file %s contains no rules (expected a 'rules:' list or a bare list of rules)", pathArg)
}

// buildRuleList parses a slice of rule maps into rules, tagging errors with the
// source path.
func buildRuleList(pathArg string, maps []map[string]interface{}) ([]rules.Rule, error) {
	out := make([]rules.Rule, 0, len(maps))
	for _, m := range maps {
		r, perr := rules.ParseMap(m)
		if perr != nil {
			return nil, fmt.Errorf("%s: %w", pathArg, perr)
		}
		out = append(out, r)
	}
	return out, nil
}

// parseRulesFromConfig parses the rules: list from fhirlint.yml (viper form).
func parseRulesFromConfig(raw interface{}) ([]rules.Rule, error) {
	items, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("rules config must be a list")
	}
	var out []rules.Rule
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("each rule must be a map, got %T", item)
		}
		r, err := rules.ParseMap(m)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// buildLintEngine parses the lint: config key into a lint engine. Returns nil
// when no lint rules are configured.
func buildLintEngine() (*lint.Engine, error) {
	if !viper.IsSet("lint") {
		return nil, nil
	}
	cfg, err := lint.ParseConfig(viper.Get("lint"))
	if err != nil {
		return nil, fmt.Errorf("lint config: %w", err)
	}
	return lint.NewEngine(cfg)
}

// isXMLContent reports whether content looks like an XML document (first
// non-whitespace byte is '<').
func isXMLContent(content []byte) bool {
	for _, b := range content {
		switch b {
		case ' ', '\t', '\r', '\n':
			continue
		case '<':
			return true
		default:
			return false
		}
	}
	return false
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

func checkMaxWarnings(results []*validator.Result, neverFailPaths map[string]struct{}) error {
	if flagMaxWarnings < 0 {
		return nil
	}
	count := 0
	for _, r := range results {
		if _, skip := neverFailPaths[r.Filename]; skip {
			continue
		}
		for _, issue := range r.Issues {
			if issue.Severity == "warning" {
				count++
			}
		}
	}
	if count > flagMaxWarnings {
		fmt.Fprintf(os.Stderr, "warning count %d exceeds --max-warnings %d\n", count, flagMaxWarnings)
		return errValidationFailed
	}
	return nil
}

func checkExitCode(results []*validator.Result, neverFailPaths map[string]struct{}) error {
	if flagFailOn == "never" {
		return nil
	}
	threshold := map[string]int{"fatal": 3, "error": 2, "warning": 1, "information": 0}
	min, ok := threshold[flagFailOn]
	if !ok {
		return fmt.Errorf("unknown --fail-on value %q — use: error, warning, information, never", flagFailOn)
	}
	for _, r := range results {
		if _, skip := neverFailPaths[r.Filename]; skip {
			continue
		}
		for _, issue := range r.Issues {
			if threshold[issue.Severity] >= min {
				return errValidationFailed
			}
		}
	}
	return nil
}

// codeSystemSizeLimitOpt turns the flag's int sentinel into the optional value
// the validator options carry. The flag needs a sentinel because a command line
// cannot express nil; the option needs a pointer because 0 is a real setting.
func codeSystemSizeLimitOpt(v int) *int {
	if v == validator.UnsetCodeSystemSizeLimit {
		return nil
	}
	return &v
}

// applyOffline settles what an offline run is allowed to do, and states the
// guarantee it actually gets.
//
// Terminology is the part that cannot be handled silently. Left alone, the
// validator would go to tx.fhir.org, so an offline run skips terminology unless
// a recording is being replayed. And a replay means a loopback HTTP server,
// which -no-http-access would block along with everything else — the JAR's
// PROHIBITED policy has no loopback exemption. So the two modes come with
// different guarantees, and the weaker one says so rather than implying a block
// that is not in place.
func applyOffline(opts *validator.Options, w io.Writer) error {
	replaying := opts.TerminologyServer != ""

	if !replaying && !opts.NoTerminologyServer {
		opts.NoTerminologyServer = true
		_, _ = fmt.Fprintln(w, "--offline: terminology checks are skipped (no reachable server). "+
			"Record one with 'fhirlint tx warm' and replay it with --tx-offline to keep them.")
	}
	if replaying {
		_, _ = fmt.Fprintln(w, "--offline: terminology is replayed from a local server, so the validator's own "+
			"network block cannot be used. fhirlint downloads nothing; the JAR could still follow a URL "+
			"if content asks it to. For a hard block, drop --tx-offline or isolate the network.")
	}
	return requireCachedIGs(opts.IGs, opts.Profiles)
}

// requireCachedIGs fails an offline run whose IG packages are not in the local
// FHIR package cache. The JAR would otherwise reach for the registry and fail
// with its own message about a package it could not resolve, which reads like
// the package is wrong rather than like the cache is empty.
func requireCachedIGs(igs, profiles []string) error {
	cacheRoot, err := coverage.DefaultCacheRoot()
	if err != nil {
		return fmt.Errorf("--offline: cannot locate the FHIR package cache: %w", err)
	}
	for _, ref := range append(append([]string{}, igs...), profiles...) {
		name, version := iglock.ParseIGID(ref)
		if name == "" || version == "" {
			// Not a package reference: a canonical profile URL, or a local
			// file. Nothing to fetch, nothing to check.
			continue
		}
		dir := coverage.PackageDir(cacheRoot, name, version)
		if _, statErr := os.Stat(dir); statErr != nil {
			return &coverage.ErrPackageNotCached{ID: name + "#" + version, Dir: dir}
		}
	}
	return nil
}
