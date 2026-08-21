// Package fhirlint provides a Go library API for validating FHIR resources.
//
// It wraps the HL7 FHIR Validator JAR and exposes a simple, stable API that
// can be embedded in other Go projects without using the CLI.
//
// Example:
//
//	result, err := fhirlint.Validate(patientJSON, fhirlint.Options{
//	    FHIRVersion:         "4.0.1",
//	    NoTerminologyServer: true,
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(result.Valid)
package fhirlint

import (
	"bytes"
	"os"
	"path/filepath"
	"time"

	"github.com/fhirlint/fhirlint/internal/input"
	"github.com/fhirlint/fhirlint/internal/profiles"
	"github.com/fhirlint/fhirlint/internal/redact"
	"github.com/fhirlint/fhirlint/internal/validator"
)

// Options configures the FHIR validator. All fields are optional.
type Options struct {
	// FHIRVersion is the FHIR version to validate against: "4.0.1", "4.3.0", "5.0.0".
	// Defaults to "4.0.1" when empty.
	FHIRVersion string

	// Profiles is a list of profile URLs or fhirlint aliases (e.g. "kbv-basis").
	Profiles []string

	// IGs is a list of IG packages (e.g. "kbv.basis#1.5.0").
	IGs []string

	// NoTerminologyServer disables the terminology server. No data is sent to tx.fhir.org.
	NoTerminologyServer bool

	// TerminologyServer sets a custom terminology server URL.
	TerminologyServer string

	// BestPractice controls how best-practice constraints (e.g. dom-6) are handled.
	// Valid values: "ignore", "hint", "warning", "error". Empty uses the JAR default.
	BestPractice string

	// TxCache sets the terminology cache directory. Pass "n/a" to disable caching.
	TxCache string

	// Locale sets the locale for validation messages (Java locale format, e.g. "de").
	Locale string

	// AllowExampleURLs suppresses warnings about example.org placeholder URLs.
	AllowExampleURLs bool

	// AllowInsecureTx suppresses the warning when the terminology server URL uses HTTP.
	AllowInsecureTx bool

	// TxLog writes a terminology server request log to the given file path.
	TxLog string

	// Jurisdiction sets the jurisdiction for country-specific bindings,
	// e.g. "urn:iso:std:iso:3166#DE". Empty derives it from the locale.
	Jurisdiction string

	// DisplayIssuesAreWarnings downgrades coded-display mismatch errors to warnings.
	DisplayIssuesAreWarnings bool

	// POFiles lists .po translation files loaded at runtime to override
	// the validator's built-in messages (e.g. "validator-messages-de.po").
	POFiles []string

	// JARPath overrides the auto-downloaded validator JAR with a local copy.
	// Can also be set via the FHIRLINT_JAR environment variable.
	JARPath string

	// ValidationTimeout stops validation after the given duration and returns
	// the issues found so far, rather than erroring out. Zero is unbounded.
	// Distinct from Timeout, which kills the validator process and yields
	// nothing.
	ValidationTimeout time.Duration

	// MaxMessages stops validation after this many messages and returns the
	// issues found so far. Zero is unbounded.
	MaxMessages int

	// Offline forbids network access for the run: the cached JAR is used and
	// never downloaded, IG packages must already be in the local FHIR package
	// cache, and the validator is told to block its own HTTP when nothing in
	// the run needs a loopback server.
	Offline bool

	// CodeSystemSizeLimit caps how many codes the validator checks against a
	// code system for one ValueSet include, ConceptMap group or CodeSystem
	// supplement. Past the cap it checks none of them and issues a hint.
	//
	// A pointer because the value 0 is meaningful — it is upstream's "no
	// limit" — so it cannot also stand for "not specified". nil leaves the
	// validator's own default in place (1000 as of 6.10.2).
	CodeSystemSizeLimit *int

	// Proxy and HTTPSProxy route the validator's terminology calls through a
	// proxy, as "host:port". Empty falls back to $HTTP_PROXY / $HTTPS_PROXY.
	//
	// Note that fhirlint's own HTTP requests already honour those variables via
	// Go's default transport; these settings exist because the validator JAR
	// does not read them.
	Proxy      string
	HTTPSProxy string

	// ProxyAuth is "username:password" for a proxy requiring basic auth. Empty
	// falls back to $FHIRLINT_PROXY_AUTH, or to credentials embedded in the
	// proxy URL.
	//
	// The validator takes this as a command-line argument, so the credential is
	// visible in `ps` to other users on the same host. It is not a secret-safe
	// channel.
	ProxyAuth string

	// ValidatorVersion pins the auto-downloaded JAR to a specific upstream
	// release (e.g. "6.9.12"). Empty tracks the latest release, which means
	// results can change when HL7 publishes a new validator. Ignored when
	// JARPath is set.
	ValidatorVersion string

	// ExtraArgs are raw arguments appended verbatim to the validator JAR
	// invocation, an escape hatch for validator options fhirlint does not
	// model yet. They are not validated; arguments that collide with the
	// flags fhirlint manages itself (-output, -output-style, -jar) are
	// rejected, since the result pipeline depends on them.
	ExtraArgs []string

	// Timeout limits how long the Java validator process may run.
	// Zero means no timeout.
	Timeout time.Duration

	// HTTPTimeout limits how long each HTTP fetch via ValidateURL may take.
	// Zero uses the default of 30 seconds.
	HTTPTimeout time.Duration

	// Redact removes resource-derived content from the returned findings: the
	// validator's message text is replaced by a placeholder and Redacted is set
	// on every issue. Severity, Location and MessageID are kept, which is
	// enough to act on a finding.
	//
	// For callers that forward findings somewhere the resource itself must not
	// go — a log aggregator, an issue tracker, a third-party dashboard. The
	// validator quotes offending values into its message text, so on real
	// patient data that text is PHI.
	Redact bool
}

// Result holds the validation outcome for one resource.
type Result struct {
	// Label is the human-readable source identifier (file path, URL, or "stdin").
	Label string

	// Valid is true when no errors or fatal issues were found.
	Valid bool

	// Issues contains all validation findings at any severity.
	Issues []Issue
}

// Issue represents a single validation finding.
type Issue struct {
	// Severity is one of "fatal", "error", "warning", "information".
	Severity string

	// Message is the human-readable description of the finding.
	Message string

	// Location identifies where in the resource the issue was found,
	// including line and column numbers where available.
	Location string

	// MessageID is the HL7 message identifier for the finding (e.g. "dom-6").
	MessageID string

	// Redacted reports that Options.Redact removed this finding's message text.
	// Carried through so a stripped finding can never be mistaken for one the
	// validator described that tersely.
	Redacted bool
}

// Validate validates a single FHIR resource from raw bytes (JSON or XML).
// The format is detected automatically from the content.
func Validate(content []byte, opts Options) (*Result, error) {
	ext := "json"
	if bytes.HasPrefix(bytes.TrimSpace(content), []byte("<")) {
		ext = "xml"
	}

	f, err := os.CreateTemp("", "fhirlint-*."+ext)
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}

	r, err := validator.Run(f.Name(), toInternalOpts(opts))
	if err != nil {
		return nil, err
	}
	applyRedaction(opts, r)
	return toPublicResult(r), nil
}

// ValidateFile validates a single FHIR resource from a file path.
func ValidateFile(path string, opts Options) (*Result, error) {
	r, err := validator.Run(path, toInternalOpts(opts))
	if err != nil {
		return nil, err
	}
	applyRedaction(opts, r)
	result := toPublicResult(r)
	result.Label = path
	return result, nil
}

// ValidateDir validates every FHIR file in a directory in a single JVM
// invocation. It accepts the same extensions the CLI does, line-delimited
// exports included — before #340 it silently skipped them, so a directory of
// .ndjson returned no results and no error.
func ValidateDir(dir string, opts Options) ([]*Result, error) {
	var paths []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !input.IsFHIRFile(path) {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, nil
	}

	internal, err := validator.RunMultiple(paths, toInternalOpts(opts))
	if err != nil {
		return nil, err
	}
	applyRedaction(opts, internal...)

	results := make([]*Result, len(internal))
	for i, r := range internal {
		results[i] = toPublicResult(r)
		results[i].Label = r.Filename
	}
	return results, nil
}

// ValidateURL fetches a FHIR resource from an HTTP endpoint and validates it.
func ValidateURL(rawURL string, opts Options) (*Result, error) {
	in, err := input.Resolve("", rawURL, opts.HTTPTimeout)
	if err != nil {
		return nil, err
	}
	defer in.Cleanup()

	r, err := validator.Run(in.Path, toInternalOpts(opts))
	if err != nil {
		return nil, err
	}
	applyRedaction(opts, r)
	result := toPublicResult(r)
	result.Label = rawURL
	return result, nil
}

func toInternalOpts(opts Options) validator.Options {
	fhirVersion := opts.FHIRVersion
	if fhirVersion == "" {
		fhirVersion = validator.DefaultFHIRVersion
	}

	resolvedProfiles := profiles.ResolveAll(opts.Profiles)

	return validator.Options{
		FHIRVersion:              fhirVersion,
		Profiles:                 resolvedProfiles,
		IGs:                      opts.IGs,
		NoTerminologyServer:      opts.NoTerminologyServer,
		TerminologyServer:        opts.TerminologyServer,
		BestPractice:             opts.BestPractice,
		TxCache:                  opts.TxCache,
		Locale:                   opts.Locale,
		AllowExampleURLs:         opts.AllowExampleURLs,
		AllowInsecureTx:          opts.AllowInsecureTx,
		TxLog:                    opts.TxLog,
		Jurisdiction:             opts.Jurisdiction,
		DisplayIssuesAreWarnings: opts.DisplayIssuesAreWarnings,
		POFiles:                  opts.POFiles,
		JARPath:                  opts.JARPath,
		ValidatorVersion:         opts.ValidatorVersion,
		ExtraArgs:                opts.ExtraArgs,
		ValidationTimeout:        opts.ValidationTimeout,
		MaxMessages:              opts.MaxMessages,
		CodeSystemSizeLimit:      opts.CodeSystemSizeLimit,
		Offline:                  opts.Offline,
		Proxy: validator.ProxyConfig{
			Proxy:      opts.Proxy,
			HTTPSProxy: opts.HTTPSProxy,
			Auth:       opts.ProxyAuth,
		},
		Timeout: opts.Timeout,
	}
}

// applyRedaction strips resource-derived content from the internal results
// before they are converted, so no public Result is ever built from data the
// caller asked not to receive.
func applyRedaction(opts Options, results ...*validator.Result) {
	if opts.Redact {
		redact.Apply(results)
	}
}

func toPublicResult(r *validator.Result) *Result {
	issues := make([]Issue, len(r.Issues))
	for i, iss := range r.Issues {
		issues[i] = Issue{
			Severity:  iss.Severity,
			Message:   iss.Message,
			Location:  iss.Location,
			MessageID: iss.MessageID,
			Redacted:  iss.Redacted,
		}
	}
	return &Result{
		Label:  r.Label,
		Valid:  r.Valid,
		Issues: issues,
	}
}
