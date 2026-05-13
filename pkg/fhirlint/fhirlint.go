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
	"strings"
	"time"

	"github.com/fhirlint/fhirlint/internal/input"
	"github.com/fhirlint/fhirlint/internal/profiles"
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

	// JARPath overrides the auto-downloaded validator JAR with a local copy.
	// Can also be set via the FHIRLINT_JAR environment variable.
	JARPath string

	// Timeout limits how long the Java validator process may run.
	// Zero means no timeout.
	Timeout time.Duration
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
	return toPublicResult(r), nil
}

// ValidateFile validates a single FHIR resource from a file path.
func ValidateFile(path string, opts Options) (*Result, error) {
	r, err := validator.Run(path, toInternalOpts(opts))
	if err != nil {
		return nil, err
	}
	result := toPublicResult(r)
	result.Label = path
	return result, nil
}

// ValidateDir validates all .json and .xml files in a directory
// in a single JVM invocation.
func ValidateDir(dir string, opts Options) ([]*Result, error) {
	var paths []string
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

	results := make([]*Result, len(internal))
	for i, r := range internal {
		results[i] = toPublicResult(r)
		results[i].Label = r.Filename
	}
	return results, nil
}

// ValidateURL fetches a FHIR resource from an HTTP endpoint and validates it.
func ValidateURL(rawURL string, opts Options) (*Result, error) {
	in, err := input.Resolve("", rawURL)
	if err != nil {
		return nil, err
	}
	defer in.Cleanup()

	r, err := validator.Run(in.Path, toInternalOpts(opts))
	if err != nil {
		return nil, err
	}
	result := toPublicResult(r)
	result.Label = rawURL
	return result, nil
}

func toInternalOpts(opts Options) validator.Options {
	fhirVersion := opts.FHIRVersion
	if fhirVersion == "" {
		fhirVersion = "4.0.1"
	}

	resolvedProfiles := make([]string, 0, len(opts.Profiles))
	for _, p := range opts.Profiles {
		resolvedProfiles = append(resolvedProfiles, profiles.Resolve(p))
	}

	return validator.Options{
		FHIRVersion:         fhirVersion,
		Profiles:            resolvedProfiles,
		IGs:                 opts.IGs,
		NoTerminologyServer: opts.NoTerminologyServer,
		TerminologyServer:   opts.TerminologyServer,
		BestPractice:        opts.BestPractice,
		TxCache:             opts.TxCache,
		Locale:              opts.Locale,
		AllowExampleURLs:    opts.AllowExampleURLs,
		JARPath:             opts.JARPath,
		Timeout:             opts.Timeout,
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
		}
	}
	return &Result{
		Label:  r.Label,
		Valid:  r.Valid,
		Issues: issues,
	}
}
