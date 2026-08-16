package validator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Issue is our internal representation, mapped from OperationOutcome.issue.
type Issue struct {
	Severity       string `json:"severity"` // fatal | error | warning | information
	Message        string `json:"message"`
	Location       string `json:"location"`
	MessageID      string `json:"messageId"`
	SuppressReason string `json:"suppressReason,omitempty"` // set when the issue is suppressed

	// OriginalSeverity is what the validator reported before a severity-override
	// re-levelled the issue; empty when nothing changed it. Severity above is
	// always the effective level, so a report carries both and cannot mislead a
	// reader who does not have the config to hand.
	OriginalSeverity string `json:"originalSeverity,omitempty"`

	// Redacted marks a finding whose message text was removed by --redact. It
	// is carried into every report so that a stripped finding can never be read
	// as one the validator described that tersely.
	Redacted bool `json:"redacted,omitempty"`
}

// Result holds the outcome for one validated resource.
type Result struct {
	Filename   string  `json:"filename"`
	Label      string  `json:"label"` // human-readable source (path, URL, "stdin")
	Valid      bool    `json:"valid"`
	Issues     []Issue `json:"issues"`
	Suppressed []Issue `json:"suppressed,omitempty"` // populated after suppress.Apply
	Cached     bool    `json:"cached,omitempty"`     // true when result came from the result cache

	// SourcePath is the file the validator actually read, which is what the
	// line/col in Issue.Location refer to. It is usually the same as Filename,
	// but differs when the input was preprocessed (--extract, --ignore,
	// --bundle-entries, NDJSON): those validate a temp copy, while Filename is
	// remapped back to the user's file for display. Anything resolving those
	// coordinates against a file must use this, not Filename.
	SourcePath string `json:"-"`
}

// operationOutcome mirrors the FHIR OperationOutcome resource the validator emits.
type operationOutcome struct {
	ResourceType string    `json:"resourceType"`
	Extension    []extItem `json:"extension"`
	Issue        []ooIssue `json:"issue"`
}

type ooIssue struct {
	Severity   string    `json:"severity"`
	Code       string    `json:"code"`
	Details    ooDetails `json:"details"`
	Expression []string  `json:"expression"`
	Extension  []extItem `json:"extension"`
}

type ooDetails struct {
	Text string `json:"text"`
}

type extItem struct {
	URL          string `json:"url"`
	ValueString  string `json:"valueString,omitempty"`
	ValueInteger int    `json:"valueInteger,omitempty"`
	ValueCode    string `json:"valueCode,omitempty"`
}

type Options struct {
	FHIRVersion              string
	Profiles                 []string
	IGs                      []string
	NoTerminologyServer      bool
	TerminologyServer        string
	BestPractice             string        // ignore | hint | warning | error (empty = JAR default)
	TxCache                  string        // path to terminology cache dir, or "n/a" to disable
	Locale                   string        // Java locale code, e.g. "de", "fr" (empty = JAR default)
	AllowExampleURLs         bool          // pass -allow-example-urls to suppress example.org warnings
	AllowInsecureTx          bool          // suppress HTTP terminology server warning
	TxLog                    string        // path to write terminology server request log (-txLog)
	Jurisdiction             string        // jurisdiction for country-specific bindings, e.g. "urn:iso:std:iso:3166#DE" (-jurisdiction)
	DisplayIssuesAreWarnings bool          // downgrade coded-display mismatches to warnings (-display-issues-are-warnings)
	POFiles                  []string      // .po translation override files loaded at runtime (-po, repeatable)
	JARPath                  string        // override auto-downloaded JAR (--jar / FHIRLINT_JAR)
	ValidatorVersion         string        // pin the auto-downloaded JAR to an upstream release (--validator-version)
	ExtraArgs                []string      // raw arguments appended verbatim to the JAR invocation (--validator-arg)
	FHIRSettings             string        // path to a fhir-settings.json for the JAR (-fhir-settings)
	Proxy                    ProxyConfig   // http/https proxy for the JAR's terminology calls (-proxy/-https-proxy/-auth)
	ValidationTimeout        time.Duration // stop validating after this long, returning partial results (-validation-timeout); 0 = unbounded
	MaxMessages              int           // stop after this many validation messages, returning partial results (-max-validation-messages); 0 = unbounded
	Timeout                  time.Duration // 0 means no timeout
}

// reservedValidatorArgs are flags fhirlint sets itself and whose values the
// result pipeline depends on. A passthrough argument that sets them again would
// break output parsing, surfacing as a confusing downstream error instead of a
// clear message here.
var reservedValidatorArgs = map[string]string{
	"output":       "fhirlint writes the validator report to a temp file and parses it",
	"output-style": "fhirlint needs -output-style json to read the results",
	"jar":          "use --jar or FHIRLINT_JAR to point at a different validator JAR",
}

// validateExtraArgs rejects passthrough arguments that collide with the flags
// fhirlint manages itself. Everything else is passed through unchecked.
func validateExtraArgs(extra []string) error {
	for _, a := range extra {
		name := a
		if i := strings.IndexByte(name, '='); i >= 0 {
			name = name[:i]
		}
		name = strings.ToLower(strings.TrimLeft(name, "-"))
		if why, ok := reservedValidatorArgs[name]; ok {
			return fmt.Errorf("--validator-arg %q is not allowed: %s", a, why)
		}
	}
	return nil
}

// buildArgs constructs the java -jar argument list for the given inputs and options.
// Separated from Run() so it can be unit-tested without invoking the JAR.
func buildArgs(jarPath string, inputPaths []string, outputPath string, opts Options) []string {
	args := []string{"-jar", jarPath}
	args = append(args, inputPaths...)
	args = append(args, "-version", opts.FHIRVersion)
	// outputPath is empty in watch mode — skip structured output flags.
	if outputPath != "" {
		args = append(args, "-output-style", "json", "-output", outputPath)
	}
	for _, ig := range opts.IGs {
		args = append(args, "-ig", ig)
	}
	for _, p := range opts.Profiles {
		if strings.Contains(p, "#") {
			args = append(args, "-ig", p)
		} else {
			args = append(args, "-profile", p)
		}
	}
	switch {
	case opts.NoTerminologyServer:
		args = append(args, "-tx", "n/a")
	case opts.TerminologyServer != "":
		args = append(args, "-tx", opts.TerminologyServer)
	}
	if opts.BestPractice != "" {
		args = append(args, "-best-practice", opts.BestPractice)
	}
	if opts.TxCache != "" {
		args = append(args, "-txCache", opts.TxCache)
	}
	if opts.FHIRSettings != "" {
		args = append(args, "-fhir-settings", opts.FHIRSettings)
	}
	if opts.Locale != "" {
		args = append(args, "-locale", opts.Locale)
	}
	if opts.AllowExampleURLs {
		args = append(args, "-allow-example-urls")
	}
	if opts.TxLog != "" {
		args = append(args, "-txLog", opts.TxLog)
	}
	if opts.Jurisdiction != "" {
		args = append(args, "-jurisdiction", opts.Jurisdiction)
	}
	if opts.DisplayIssuesAreWarnings {
		args = append(args, "-display-issues-are-warnings")
	}
	for _, po := range opts.POFiles {
		args = append(args, "-po", po)
	}
	// Both bounds make the validator return what it found so far rather than
	// erroring out, which is what you want in a pipeline: partial findings beat
	// a job that hangs or a report nobody can read.
	if opts.ValidationTimeout > 0 {
		args = append(args, "-validation-timeout", strconv.FormatInt(opts.ValidationTimeout.Milliseconds(), 10))
	}
	if opts.MaxMessages > 0 {
		args = append(args, "-max-validation-messages", strconv.Itoa(opts.MaxMessages))
	}
	// fhirlint's own downloads already honour the proxy environment via Go's
	// default transport; the JAR does not, so its terminology calls need these
	// passed explicitly.
	args = append(args, proxyArgs(opts.Proxy)...)
	// Passthrough arguments go last so that, for flags the JAR resolves
	// last-wins, the user's explicit choice takes effect. Reserved flags are
	// rejected up front by validateExtraArgs.
	args = append(args, opts.ExtraArgs...)
	return args
}

// validateFHIRVersion returns an error when version names no release in
// FHIRVersions, which is the list the HL7 validator JAR accepts.
func validateFHIRVersion(version string) error {
	if _, ok := LookupFHIRVersion(version); ok {
		return nil
	}
	return fmt.Errorf("unknown FHIR version %q — allowed: %s", version, FHIRVersionList())
}

// allowedBestPractice lists the values the HL7 validator JAR accepts for -best-practice.
var allowedBestPractice = []string{"ignore", "hint", "warning", "error"}

// validateBestPractice returns an error when value is not in allowedBestPractice.
// An empty value means "use JAR default" and is always valid.
func validateBestPractice(value string) error {
	if value == "" {
		return nil
	}
	for _, v := range allowedBestPractice {
		if value == v {
			return nil
		}
	}
	return fmt.Errorf("unknown --best-practice value %q — allowed: %s", value, strings.Join(allowedBestPractice, ", "))
}

// warnInsecureTerminologyServer writes a warning to w when the terminology server URL
// uses HTTP and the user has not suppressed the warning with AllowInsecureTx.
func warnInsecureTerminologyServer(w io.Writer, opts Options) {
	if opts.AllowInsecureTx || !strings.HasPrefix(opts.TerminologyServer, "http://") {
		return
	}
	_, _ = fmt.Fprintln(w, "warning: terminology server URL uses HTTP — data will be transmitted unencrypted.")
	_, _ = fmt.Fprintln(w, "Use HTTPS or suppress this warning with --allow-insecure-tx.")
}

// RunWatch starts the JAR in watch mode and blocks until the process is killed (Ctrl-C).
// mode must be "single" or "all". intervalMS sets the polling interval in milliseconds (0 = JAR default).
// The JAR prints results directly to stdout/stderr — no structured output is captured.
func RunWatch(inputPaths []string, opts Options, mode string, intervalMS int) error {
	if err := validateExtraArgs(opts.ExtraArgs); err != nil {
		return err
	}
	if err := validateFHIRVersion(opts.FHIRVersion); err != nil {
		return err
	}
	if err := validateBestPractice(opts.BestPractice); err != nil {
		return err
	}

	jarPath, err := EnsureJAR(opts.JARPath, opts.ValidatorVersion)
	if err != nil {
		return err
	}

	warnInsecureTerminologyServer(os.Stderr, opts)
	args := buildArgs(jarPath, inputPaths, "", opts)
	args = append(args, "-watch-mode", mode)
	if intervalMS > 0 {
		args = append(args, "-watch-interval", strconv.Itoa(intervalMS))
	}

	cmd := exec.Command("java", args...) //nolint:gosec // intentional: runs java with user-controlled paths
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Run validates a single file and returns its result.
func Run(inputPath string, opts Options) (*Result, error) {
	results, err := RunMultiple([]string{inputPath}, opts)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("validator produced no result for %s", inputPath)
	}
	return results[0], nil
}

// RunMultiple validates all inputPaths in a single JVM invocation.
// For directories, this avoids repeated JVM startup and terminology load overhead.
func RunMultiple(inputPaths []string, opts Options) ([]*Result, error) {
	if len(inputPaths) == 0 {
		return nil, nil
	}

	if err := validateExtraArgs(opts.ExtraArgs); err != nil {
		return nil, err
	}
	if err := validateFHIRVersion(opts.FHIRVersion); err != nil {
		return nil, err
	}
	if err := validateBestPractice(opts.BestPractice); err != nil {
		return nil, err
	}

	jarPath, err := EnsureJAR(opts.JARPath, opts.ValidatorVersion)
	if err != nil {
		return nil, err
	}

	warnInsecureTerminologyServer(os.Stderr, opts)

	// Write JSON output to a temp file to avoid mixing with validator log output.
	tmpFile, err := os.CreateTemp("", "fhirlint-result-*.json")
	if err != nil {
		return nil, fmt.Errorf("creating temp output file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return nil, fmt.Errorf("closing temp output file: %w", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	args := buildArgs(jarPath, inputPaths, tmpFile.Name(), opts)

	ctx := context.Background()
	var cancel context.CancelFunc = func() {}
	if opts.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
	}
	defer cancel()

	var stderrBuf bytes.Buffer
	cmd := exec.CommandContext(ctx, "java", args...) //nolint:gosec // intentional: runs java with user-controlled input paths
	cmd.Stdout = nil
	cmd.Stderr = &stderrBuf
	// Non-zero exit is expected when validation finds errors — not a tool failure.
	_ = cmd.Run()

	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("validator timed out after %s — use --timeout to increase the limit", formatDuration(opts.Timeout))
	}

	jsonBytes, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		return nil, fmt.Errorf("reading validator output: %w", err)
	}

	if len(jsonBytes) == 0 {
		if err := oomError(stderrBuf.String()); err != nil {
			return nil, err
		}
		if err := txUnreachableError(stderrBuf.String(), opts); err != nil {
			return nil, err
		}
		// Nothing recognised. Say the validator produced no output — which is
		// what we know — rather than asserting a crash, and trim the Java frames
		// so the exception message is not buried under them.
		return nil, fmt.Errorf("validator produced no output\nfiles: %v\nstderr: %s",
			inputPaths, trimJavaFrames(stderrBuf.String()))
	}

	return parseOutput(jsonBytes, inputPaths, stderrBuf.String())
}

// txCapabilityFailure is the sentence the validator emits when it cannot read a
// terminology server's capability statement — the first thing it asks for, so
// this is what an unreachable server looks like whatever the underlying cause.
// It covers a refused connection, an unresolvable host and a proxy that is not
// answering; only the detail after the colon differs.
const txCapabilityFailure = "Error fetching the server's capability statement"

// DefaultTerminologyServer is the server the validator uses when none is given.
// Recording and replay need to name it explicitly, so it cannot stay implicit
// in the JAR's own defaults.
const DefaultTerminologyServer = "https://tx.fhir.org"

// DefaultTerminologyEndpoint returns the versioned base URL the validator would
// use for a FHIR version when no -tx is given.
//
// The distinction matters for recording: the JAR appends a version path to its
// own default (tx.fhir.org/r4/metadata) but uses an explicit -tx URL verbatim.
// Pointing the JAR at a local recorder therefore means reconstructing the path
// it would otherwise have used, or every proxied request 404s.
//
// The path per release comes from FHIRVersions, where R4B maps to /r4 rather
// than the /r4b that tx.fhir.org does not serve. An unrecognised version — one
// that never passed validateFHIRVersion — falls back to /r4, which is what this
// function has always answered for anything it did not recognise.
func DefaultTerminologyEndpoint(fhirVersion string) string {
	if v, ok := LookupFHIRVersion(fhirVersion); ok {
		return DefaultTerminologyServer + v.TxPath
	}
	return DefaultTerminologyServer + "/r4"
}

// txUnreachableError turns an unreachable terminology server into an
// explanation instead of "the JAR may have crashed". The JAR did not crash: it
// could not reach the server it needs for code and value-set checks, and the
// three ways out are not obvious from a Java stack trace.
func txUnreachableError(stderr string, opts Options) error {
	idx := strings.Index(stderr, txCapabilityFailure)
	if idx < 0 {
		return nil
	}

	server := opts.TerminologyServer
	if server == "" {
		server = DefaultTerminologyServer + " (the default)"
	}
	detail := strings.TrimSpace(stderr[idx+len(txCapabilityFailure):])
	detail = strings.TrimSpace(strings.TrimPrefix(detail, ":"))
	if i := strings.IndexAny(detail, "\r\n"); i >= 0 {
		detail = detail[:i]
	}

	fmt.Fprintf(os.Stderr, "Cannot reach the terminology server %s.\n", server)
	// For an unresolvable host the detail is just the hostname, which the line
	// above already named — printing it again reads like a mistake.
	if detail != "" && !strings.Contains(server, detail) {
		fmt.Fprintf(os.Stderr, "  %s\n", detail)
	}
	fmt.Fprintln(os.Stderr, "The validator needs it to check codes and value sets. Either:")
	fmt.Fprintln(os.Stderr, "  --proxy / --https-proxy       if this network requires a proxy")
	fmt.Fprintln(os.Stderr, "  --terminology-server <url>    point at a reachable server")
	fmt.Fprintln(os.Stderr, "  --no-terminology-server       validate without terminology checks")
	return errors.New("terminology server unreachable")
}

// trimJavaFrames keeps the exception message and the first few stack frames,
// replacing the rest with a count. A validator stack trace runs to dozens of
// frames of HAPI internals, which pushes the one line that says what went wrong
// off the top of the terminal.
func trimJavaFrames(stderr string) string {
	const keep = 3
	lines := strings.Split(stderr, "\n")
	var out []string
	frames, dropped := 0, 0
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimRight(l, "\r"), "\tat ") {
			frames++
			if frames > keep {
				dropped++
				continue
			}
		}
		out = append(out, l)
	}
	if dropped > 0 {
		out = append(out, fmt.Sprintf("\t… %d more frames", dropped))
	}
	return strings.Join(out, "\n")
}

func oomError(stderr string) error {
	if strings.Contains(stderr, "OutOfMemoryError") {
		fmt.Fprintln(os.Stderr, "Java ran out of memory while validating. Try increasing the heap size:")
		fmt.Fprintln(os.Stderr, `  JAVA_OPTS="-Xmx2g" fhirlint validate`)
		return errors.New("out of memory")
	}
	return nil
}

// parseOutput handles both single OperationOutcome (one file) and Bundle (multiple files).
func parseOutput(data []byte, inputPaths []string, stderr string) ([]*Result, error) {
	var peek struct {
		ResourceType string `json:"resourceType"`
	}
	if err := json.Unmarshal(data, &peek); err != nil {
		return nil, fmt.Errorf("parsing validator output: %w\nraw: %s\nstderr: %s", err, data, stderr)
	}

	switch peek.ResourceType {
	case "OperationOutcome":
		var oo operationOutcome
		if err := json.Unmarshal(data, &oo); err != nil {
			return nil, fmt.Errorf("parsing OperationOutcome: %w\nraw: %s\nstderr: %s", err, data, stderr)
		}
		filename := ""
		if len(inputPaths) > 0 {
			filename = inputPaths[0]
		}
		return []*Result{toResult(oo, filename)}, nil

	case "Bundle":
		var bundle struct {
			Entry []struct {
				FullURL  string           `json:"fullUrl"`
				Resource operationOutcome `json:"resource"`
			} `json:"entry"`
		}
		if err := json.Unmarshal(data, &bundle); err != nil {
			return nil, fmt.Errorf("parsing Bundle: %w\nraw: %s\nstderr: %s", err, data, stderr)
		}
		results := make([]*Result, len(bundle.Entry))
		for i, entry := range bundle.Entry {
			// Use positional mapping: entry[i] corresponds to inputPaths[i].
			filename := ""
			if i < len(inputPaths) {
				filename = inputPaths[i]
			} else {
				filename = strings.TrimPrefix(entry.FullURL, "file://")
			}
			results[i] = toResult(entry.Resource, filename)
		}
		return results, nil

	default:
		return nil, fmt.Errorf("unexpected resourceType %q in validator output\nstderr: %s", peek.ResourceType, stderr)
	}
}

// formatDuration formats a duration for display, removing redundant zero components
// (e.g. "5m0s" → "5m", "1h0m0s" → "1h").
func formatDuration(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = s[:len(s)-2] // "5m0s" → "5m"
	}
	if strings.HasSuffix(s, "h0m") {
		s = s[:len(s)-2] // "1h0m" → "1h"
	}
	return s
}

func toResult(oo operationOutcome, filename string) *Result {
	valid := true
	issues := make([]Issue, 0, len(oo.Issue))

	for _, i := range oo.Issue {
		if i.Severity == "error" || i.Severity == "fatal" {
			valid = false
		}

		location := strings.Join(i.Expression, ", ")
		line, col := 0, 0
		messageID := ""

		for _, ext := range i.Extension {
			switch ext.URL {
			case "http://hl7.org/fhir/StructureDefinition/operationoutcome-issue-line":
				line = ext.ValueInteger
			case "http://hl7.org/fhir/StructureDefinition/operationoutcome-issue-col":
				col = ext.ValueInteger
			case "http://hl7.org/fhir/StructureDefinition/operationoutcome-message-id":
				messageID = ext.ValueCode
			}
		}

		loc := location
		if line > 0 {
			loc = fmt.Sprintf("%s (line %d, col %d)", location, line, col)
		}

		issues = append(issues, Issue{
			Severity:  i.Severity,
			Message:   i.Details.Text,
			Location:  loc,
			MessageID: messageID,
		})
	}

	return &Result{
		Filename:   filename,
		SourcePath: filename,
		Valid:      valid,
		Issues:     issues,
	}
}
