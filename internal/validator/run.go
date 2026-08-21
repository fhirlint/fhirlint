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

	"github.com/fhirlint/fhirlint/internal/cache"
	"github.com/fhirlint/fhirlint/internal/igaudit"
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

	// CodeSystemSizeLimit caps how many codes the validator checks against a
	// code system for a single ValueSet include, ConceptMap group or CodeSystem
	// supplement (-codesystem-validation-size-limit). Past the cap the
	// validator checks none of them and issues a hint instead, because every
	// code costs a terminology round trip.
	//
	// Offline forbids this run from using the network. fhirlint refuses to
	// download the JAR or fetch IG packages, and — when nothing in the run
	// needs a local loopback server — the JAR is told to block HTTP itself with
	// -no-http-access, which is what turns the promise into an enforced one.
	Offline bool

	// A pointer because upstream's 0 means "no limit", so it cannot double as
	// "not specified". nil passes no argument at all and leaves the validator's
	// own default in place (1000 as of 6.10.2, where the parameter appeared).
	// A plain int would make every existing Options literal request "no limit"
	// by omission — and on a validator older than 6.10.2, fail outright.
	CodeSystemSizeLimit *int
	Timeout             time.Duration // 0 means no timeout
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
	args := jvmArgs(jarPath)
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
	if blocksJARNetwork(opts.Offline, opts.TerminologyServer) {
		args = append(args, "-no-http-access")
	}
	if opts.CodeSystemSizeLimit != nil {
		args = append(args, "-codesystem-validation-size-limit", strconv.Itoa(*opts.CodeSystemSizeLimit))
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

// jvmArgs starts the argument list for every JVM fhirlint launches.
//
// It pins user.home to the home directory fhirlint itself uses. On Linux the
// JVM takes user.home from the OS passwd entry and ignores $HOME, so a CI job
// that exports a writable $HOME — because the runner's real home is not
// writable — gets fhirlint writing to one place and the validator reading from
// another. The validator then dies listing a package cache it cannot reach
// (#351).
//
// The two are the same directory on any ordinary machine, so this is a no-op
// there. When they differ, fhirlint's answer is the right one: it is the same
// $HOME that decided where ~/.fhirlint and the ~/.fhir package cache the
// --offline check inspects live.
//
// A user.home already set through JAVA_TOOL_OPTIONS wins: someone who spelled
// it out deliberately should not have it overruled.
func jvmArgs(jarPath string) []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || strings.Contains(os.Getenv("JAVA_TOOL_OPTIONS"), "-Duser.home") {
		return []string{"-jar", jarPath}
	}
	return []string{"-Duser.home=" + home, "-jar", jarPath}
}

// UnsetCodeSystemSizeLimit is what a command-line int uses to say "not
// specified", since a flag cannot carry nil. It cannot be 0: upstream gives 0
// the meaning "check every code, however many there are".
const UnsetCodeSystemSizeLimit = -1

// SkippedCheckMessageIDs are the messages the validator issues when it declines
// to check a set of codes because there are more of them than
// -codesystem-validation-size-limit allows.
//
// They arrive as hints, which is how a whole class of checks can silently stop
// running: filter hints out, as most projects do, and a run where nothing was
// checked looks exactly like a run where everything passed. Reporters use this
// list to say so regardless of the severity filter (#338).
var SkippedCheckMessageIDs = []string{
	"VALUESET_INC_TOO_MANY_CODES",       // a ValueSet include
	"CONCEPTMAP_VS_TOO_MANY_CODES",      // a ConceptMap group
	"CODESYSTEM_CS_SUPP_TOO_MANY_CODES", // a CodeSystem supplement
}

// IsSkippedCheck reports whether a message id is one of the above.
func IsSkippedCheck(messageID string) bool {
	for _, id := range SkippedCheckMessageIDs {
		if strings.EqualFold(messageID, id) {
			return true
		}
	}
	return false
}

// requireCachedJAR stops an offline run before EnsureJAR reaches for the
// network. Downloading 250 MB is exactly what --offline promises not to do, and
// finding that out from a stalled progress line would be worse than an error.
func requireCachedJAR(offline bool, jarPath string) error {
	if !offline || jarPath != "" {
		return nil
	}
	path, err := cache.JARPath()
	if err != nil {
		return fmt.Errorf("--offline: cannot locate the JAR cache: %w", err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		return fmt.Errorf(
			"--offline: no validator JAR in the cache (%s) — run 'fhirlint update' once with network access, or point --jar at a local copy",
			path)
	}
	return nil
}

// minCodeSystemSizeLimitVersion is the first validator release that understands
// -codesystem-validation-size-limit.
const minCodeSystemSizeLimitVersion = "6.10.2"

// minNoHTTPAccessVersion is the first validator release that understands
// -no-http-access (added with the managed-web-access options in 6.10.0).
const minNoHTTPAccessVersion = "6.10.0"

// blocksJARNetwork reports whether this run can hand the JAR its own network
// block.
//
// -no-http-access sets the validator's access policy to PROHIBITED, and that
// policy refuses every request unconditionally — there is no loopback
// exemption (the localhost list in ManagedWebAccess only governs the
// http→https upgrade). So a run replaying terminology from fhirlint's local
// server cannot use it: the block would cut off the replay server too.
//
// The guarantee therefore differs by mode, and the caller says which one is in
// force rather than leaving the user to guess.
func blocksJARNetwork(offline bool, terminologyServer string) bool {
	return offline && terminologyServer == ""
}

// checkOptionsSupported rejects options the JAR in use is too old to
// understand. An unknown parameter makes the validator exit without producing
// output; the error now carries what it printed (#351), but a picocli stack
// trace is still a poor way to learn that a pinned JAR is the reason.
//
// An empty or unorderable version is not treated as evidence: better to let the
// run proceed and fail upstream than to refuse on a guess.
func checkOptionsSupported(opts Options, jarVersion string) error {
	if jarVersion == "" {
		return nil
	}
	if opts.CodeSystemSizeLimit != nil {
		if err := requireVersion(jarVersion, minCodeSystemSizeLimitVersion, "--codesystem-size-limit"); err != nil {
			return err
		}
	}
	if blocksJARNetwork(opts.Offline, opts.TerminologyServer) {
		if err := requireVersion(jarVersion, minNoHTTPAccessVersion, "--offline"); err != nil {
			return err
		}
	}
	return nil
}

// jarVersion reports the version of the JAR this run actually executes.
//
// ValidatorVersion() answers a different question — what is in the cache — and
// a run started with --jar or FHIRLINT_JAR does not execute that file. Asking
// the cache there produces a version guard that refuses a JAR new enough to
// have the flag, which is the most annoying way for a guard to be wrong.
func jarVersion(jarPath string) string {
	cachedPath, err := cache.JARPath()
	if err == nil && jarPath == cachedPath {
		return ValidatorVersion()
	}
	if v := versionFromJARManifest(jarPath); v != "" {
		return v
	}
	// Unknown rather than the cache's answer: an unreadable manifest must not
	// be reported as some other JAR's version.
	return ""
}

// requireVersion rejects a flag the JAR in use is too old to understand. An
// unorderable version is not treated as evidence.
func requireVersion(jarVersion, minVersion, flag string) error {
	cmp, ok := igaudit.CompareVersions(jarVersion, minVersion)
	if !ok || cmp >= 0 {
		return nil
	}
	return fmt.Errorf(
		"%s needs validator %s or newer, but the JAR in use is %s — run 'fhirlint update', or drop the flag",
		flag, minVersion, jarVersion)
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

	if err := requireCachedJAR(opts.Offline, opts.JARPath); err != nil {
		return err
	}
	jarPath, err := EnsureJAR(opts.JARPath, opts.ValidatorVersion)
	if err != nil {
		return err
	}
	if err := checkOptionsSupported(opts, jarVersion(jarPath)); err != nil {
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

	if err := requireCachedJAR(opts.Offline, opts.JARPath); err != nil {
		return nil, err
	}
	jarPath, err := EnsureJAR(opts.JARPath, opts.ValidatorVersion)
	if err != nil {
		return nil, err
	}
	if err := checkOptionsSupported(opts, jarVersion(jarPath)); err != nil {
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

	// Both streams, because the validator uses stdout for everything — its
	// banner, its progress, and the exception it dies on. Reporting only
	// stderr is why a JAR-side failure used to arrive as "produced no output"
	// with nothing after it (#351).
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd := exec.CommandContext(ctx, "java", args...) //nolint:gosec // intentional: runs java with user-controlled input paths
	cmd.Stdout = &stdoutBuf
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
		// The recognisers look at both streams: which one carries a given
		// message is the JAR's business and has changed between releases.
		combined := stderrBuf.String() + "\n" + stdoutBuf.String()
		if err := oomError(combined); err != nil {
			return nil, err
		}
		if err := txUnreachableError(combined, opts); err != nil {
			return nil, err
		}
		// Nothing recognised. Say the validator produced no result — which is
		// what we know — rather than asserting a crash, and show what it did
		// say instead of leaving the user with an empty stderr.
		return nil, fmt.Errorf("validator produced no result\nfiles: %v\n%s",
			inputPaths, jarDiagnostics(stdoutBuf.String(), stderrBuf.String()))
	}

	return parseOutput(jsonBytes, inputPaths, jarDiagnostics(stdoutBuf.String(), stderrBuf.String()))
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
// jarDiagnosticLines bounds how much of the validator's output an error
// carries. It prints a banner and a line per loaded package before it fails, so
// the whole buffer would bury the message that matters; the failure is at the
// end.
const jarDiagnosticLines = 15

// jarDiagnostics renders what the validator said when it produced no result.
//
// Both streams are shown, labelled, because the split is not intuitive: the
// validator writes even fatal errors to stdout, and stderr is usually empty.
// Omitting an empty stream keeps the common case to one block.
func jarDiagnostics(stdout, stderr string) string {
	var b strings.Builder
	for _, s := range []struct{ name, text string }{{"stdout", stdout}, {"stderr", stderr}} {
		trimmed := strings.TrimSpace(s.text)
		if trimmed == "" {
			continue
		}
		fmt.Fprintf(&b, "%s: %s\n", s.name, tailLines(trimJavaFrames(trimmed), jarDiagnosticLines))
	}
	if b.Len() == 0 {
		return "the validator wrote nothing to stdout or stderr"
	}
	return strings.TrimRight(b.String(), "\n")
}

// tailLines keeps the last n lines, noting what it dropped so the output is not
// silently partial.
func tailLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	kept := lines[len(lines)-n:]
	return fmt.Sprintf("(%d earlier line(s) omitted)\n%s", len(lines)-n, strings.Join(kept, "\n"))
}

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
// diagnostics is what the validator said, rendered by jarDiagnostics: both
// streams, labelled and bounded. It is only ever shown when something failed.
func parseOutput(data []byte, inputPaths []string, diagnostics string) ([]*Result, error) {
	var peek struct {
		ResourceType string `json:"resourceType"`
	}
	if err := json.Unmarshal(data, &peek); err != nil {
		return nil, fmt.Errorf("parsing validator output: %w\nraw: %s\n%s", err, data, diagnostics)
	}

	switch peek.ResourceType {
	case "OperationOutcome":
		var oo operationOutcome
		if err := json.Unmarshal(data, &oo); err != nil {
			return nil, fmt.Errorf("parsing OperationOutcome: %w\nraw: %s\n%s", err, data, diagnostics)
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
			return nil, fmt.Errorf("parsing Bundle: %w\nraw: %s\n%s", err, data, diagnostics)
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
		return nil, fmt.Errorf("unexpected resourceType %q in validator output\n%s", peek.ResourceType, diagnostics)
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
