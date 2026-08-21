package validator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/fhirlint/fhirlint/internal/cache"
)

// fixtureOO loads a testdata/fixtures/*.json file as an operationOutcome.
func fixtureOO(t *testing.T, name string) operationOutcome {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "fixtures", name)
	data, err := os.ReadFile(root) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	var oo operationOutcome
	if err := json.Unmarshal(data, &oo); err != nil {
		t.Fatalf("parsing fixture %s: %v", name, err)
	}
	return oo
}

func TestToResult_NoIssues_IsValid(t *testing.T) {
	oo := fixtureOO(t, "oo-no-issues.json")
	result := toResult(oo, "patient.json")

	if !result.Valid {
		t.Error("expected Valid=true for OperationOutcome with no issues")
	}
	if len(result.Issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(result.Issues))
	}
}

func TestToResult_WarningOnly_IsValid(t *testing.T) {
	oo := fixtureOO(t, "oo-warning.json")
	result := toResult(oo, "patient.json")

	if !result.Valid {
		t.Error("expected Valid=true for warning-only OperationOutcome")
	}
	if len(result.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(result.Issues))
	}
	if result.Issues[0].Severity != "warning" {
		t.Errorf("expected severity=warning, got %q", result.Issues[0].Severity)
	}
}

func TestToResult_Error_IsInvalid(t *testing.T) {
	oo := fixtureOO(t, "oo-error.json")
	result := toResult(oo, "patient.json")

	if result.Valid {
		t.Error("expected Valid=false for OperationOutcome with error")
	}
}

func TestToResult_Fatal_IsInvalid(t *testing.T) {
	oo := fixtureOO(t, "oo-fatal.json")
	result := toResult(oo, "patient.json")

	if result.Valid {
		t.Error("expected Valid=false for OperationOutcome with fatal severity")
	}
}

func TestToResult_MixedIssues_CountsCorrectly(t *testing.T) {
	oo := fixtureOO(t, "oo-error.json")
	result := toResult(oo, "patient.json")

	if len(result.Issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(result.Issues))
	}

	errors, warnings := 0, 0
	for _, i := range result.Issues {
		switch i.Severity {
		case "error":
			errors++
		case "warning":
			warnings++
		}
	}
	if errors != 1 || warnings != 1 {
		t.Errorf("expected 1 error + 1 warning, got %d errors + %d warnings", errors, warnings)
	}
}

func TestToResult_LineColFromExtension(t *testing.T) {
	oo := fixtureOO(t, "oo-warning.json")
	result := toResult(oo, "patient.json")

	if len(result.Issues) == 0 {
		t.Fatal("expected at least 1 issue")
	}
	loc := result.Issues[0].Location
	if loc == "" {
		t.Error("expected non-empty location")
	}
	// Should contain line and col info
	for _, want := range []string{"line 3", "col 12"} {
		if !containsStr(loc, want) {
			t.Errorf("location %q should contain %q", loc, want)
		}
	}
}

func TestToResult_MessageIDFromExtension(t *testing.T) {
	oo := fixtureOO(t, "oo-warning.json")
	result := toResult(oo, "patient.json")

	if result.Issues[0].MessageID != "dom-6" {
		t.Errorf("expected messageId=dom-6, got %q", result.Issues[0].MessageID)
	}
}

func TestToResult_MultipleExpressionsJoined(t *testing.T) {
	oo := fixtureOO(t, "oo-multi-expression.json")
	result := toResult(oo, "bundle.json")

	loc := result.Issues[0].Location
	if !containsStr(loc, "Bundle.entry[2].resource") {
		t.Errorf("location %q should contain first expression", loc)
	}
	if !containsStr(loc, "Observation") {
		t.Errorf("location %q should contain second expression", loc)
	}
}

func TestToResult_FilenamePreserved(t *testing.T) {
	oo := fixtureOO(t, "oo-no-issues.json")
	result := toResult(oo, "my/custom/path.json")

	if result.Filename != "my/custom/path.json" {
		t.Errorf("expected filename %q, got %q", "my/custom/path.json", result.Filename)
	}
}

func TestToResult_MessageTextFromDetails(t *testing.T) {
	oo := fixtureOO(t, "oo-error.json")
	result := toResult(oo, "patient.json")

	want := "The value 'unknown' is not valid for element Patient.gender"
	if result.Issues[0].Message != want {
		t.Errorf("expected message %q, got %q", want, result.Issues[0].Message)
	}
}

func TestOOMError_DetectsOutOfMemory(t *testing.T) {
	stderr := "Exception in thread \"main\" java.lang.OutOfMemoryError: Java heap space"
	err := oomError(stderr)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "out of memory" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestOOMError_NilOnNormalStderr(t *testing.T) {
	err := oomError("some other warning from the JVM")
	if err != nil {
		t.Errorf("expected nil, got: %v", err)
	}
}

func TestOOMError_NilOnEmpty(t *testing.T) {
	if err := oomError(""); err != nil {
		t.Errorf("expected nil for empty stderr, got: %v", err)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{5 * time.Minute, "5m"},
		{30 * time.Second, "30s"},
		{time.Hour, "1h"},
		{90 * time.Second, "1m30s"},
		{200 * time.Millisecond, "200ms"},
		{90 * time.Minute, "1h30m"},
	}
	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestRunMultiple_TimesOut(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess timeout test")
	}

	dir := t.TempDir()

	// Fake "java" that hangs indefinitely.
	// exec replaces the shell process so that SIGKILL reaches the sleep directly,
	// preventing the child from holding the stderr pipe open past the timeout.
	fakeJava := filepath.Join(dir, "java")
	if err := os.WriteFile(fakeJava, []byte("#!/bin/sh\nexec sleep 60\n"), 0755); err != nil { //nolint:gosec // test helper
		t.Fatal(err)
	}

	// Dummy JAR file with ZIP magic bytes so EnsureJAR passes the isValidJAR check.
	fakeJAR := filepath.Join(dir, "validator_cli.jar")
	if err := os.WriteFile(fakeJAR, []byte{0x50, 0x4B, 0x03, 0x04}, 0600); err != nil {
		t.Fatal(err)
	}

	// Dummy FHIR input file.
	fakeInput := filepath.Join(dir, "patient.json")
	if err := os.WriteFile(fakeInput, []byte(`{"resourceType":"Patient"}`), 0600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, err := RunMultiple([]string{fakeInput}, Options{
		FHIRVersion: "4.0.1",
		JARPath:     fakeJAR,
		Timeout:     200 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected 'timed out' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "--timeout") {
		t.Errorf("expected '--timeout' in error, got: %v", err)
	}
}

// TestRun_Integration runs the actual validator — skipped in short mode.
func TestRun_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test (requires Java + JAR download)")
	}
	_, file, _, _ := runtime.Caller(0)
	patient := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "patient.json")

	result, err := Run(patient, Options{FHIRVersion: "4.0.1", NoTerminologyServer: true})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result == nil {
		t.Fatal("Run() returned nil result")
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}

// The three shapes an unreachable terminology server takes in practice, all
// captured from the real JAR.
const (
	txRefusedStderr = "org.hl7.fhir.exceptions.FHIRException: Error fetching the server's " +
		"capability statement: Failed to connect to /127.0.0.1:9\n" +
		"\tat org.hl7.fhir.r4.utils.client.FHIRToolingClient.getCapabilitiesStatement(FHIRToolingClient.java:146)\n"
	txUnknownHostStderr = "org.hl7.fhir.exceptions.FHIRException: Error fetching the server's " +
		"capability statement: tx.invalid.example\n"
)

func TestTxUnreachableError_DetectsRefusedConnection(t *testing.T) {
	err := txUnreachableError(txRefusedStderr, Options{})
	if err == nil {
		t.Fatal("an unreachable terminology server must be recognised, not reported as a crash")
	}
	if !strings.Contains(err.Error(), "terminology") {
		t.Errorf("error should name the cause, got: %v", err)
	}
}

func TestTxUnreachableError_DetectsUnknownHost(t *testing.T) {
	if err := txUnreachableError(txUnknownHostStderr, Options{}); err == nil {
		t.Fatal("an unresolvable terminology host must be recognised too")
	}
}

func TestTxUnreachableError_IgnoresUnrelatedStderr(t *testing.T) {
	if err := txUnreachableError("java.lang.NullPointerException\n\tat Foo.bar(Foo.java:1)\n", Options{}); err != nil {
		t.Errorf("an unrelated failure must not be reported as a terminology problem, got: %v", err)
	}
}

func TestTxUnreachableError_EmptyStderr(t *testing.T) {
	if err := txUnreachableError("", Options{}); err != nil {
		t.Errorf("expected nil for empty stderr, got: %v", err)
	}
}

func TestTrimJavaFrames_KeepsMessageAndCountsRest(t *testing.T) {
	stderr := "org.hl7.fhir.exceptions.FHIRException: boom\n" +
		"\tat A.a(A.java:1)\n\tat B.b(B.java:2)\n\tat C.c(C.java:3)\n" +
		"\tat D.d(D.java:4)\n\tat E.e(E.java:5)\n"
	got := trimJavaFrames(stderr)

	if !strings.Contains(got, "FHIRException: boom") {
		t.Error("the exception message must survive trimming — it is the whole point")
	}
	if !strings.Contains(got, "A.a") || !strings.Contains(got, "C.c") {
		t.Error("the first frames should be kept")
	}
	if strings.Contains(got, "D.d") || strings.Contains(got, "E.e") {
		t.Error("frames beyond the limit should be dropped")
	}
	if !strings.Contains(got, "2 more frames") {
		t.Errorf("dropped frames must be accounted for, got:\n%s", got)
	}
}

func TestTrimJavaFrames_ShortTraceUnchanged(t *testing.T) {
	stderr := "some error\n\tat A.a(A.java:1)\n"
	got := trimJavaFrames(stderr)
	if strings.Contains(got, "more frames") {
		t.Errorf("a short trace needs no summary line, got:\n%s", got)
	}
	if !strings.Contains(got, "A.a") {
		t.Error("the frame should be kept")
	}
}

// A JAR older than the parameter refuses to start and produces no output at
// all, which reaches the user as "validator produced no output". Catch it with
// a message that names the cause.
func TestCheckOptionsSupported_CodeSystemSizeLimit(t *testing.T) {
	limit := 5000
	set := Options{CodeSystemSizeLimit: &limit}

	if err := checkOptionsSupported(set, "6.9.6"); err == nil {
		t.Error("err = nil for a JAR that predates the parameter, want a rejection")
	} else if !strings.Contains(err.Error(), "6.10.2") || !strings.Contains(err.Error(), "6.9.6") {
		t.Errorf("err = %q, want it to name both the required and the actual version", err)
	}

	for _, version := range []string{"6.10.2", "6.11.0", "7.0.0"} {
		if err := checkOptionsSupported(set, version); err != nil {
			t.Errorf("checkOptionsSupported(%s) = %v, want nil", version, err)
		}
	}

	// Unset: nothing to check, whatever the JAR is.
	if err := checkOptionsSupported(Options{}, "6.9.6"); err != nil {
		t.Errorf("unset option rejected on an old JAR: %v", err)
	}

	// Unknown or unorderable versions are not evidence — let the run proceed
	// and fail upstream rather than refusing on a guess.
	for _, version := range []string{"", "custom-build"} {
		if err := checkOptionsSupported(set, version); err != nil {
			t.Errorf("checkOptionsSupported(%q) = %v, want nil", version, err)
		}
	}
}

// -no-http-access blocks every request the validator makes, loopback included:
// the PROHIBITED policy has no exemption, verified against 6.10.2. So a run
// replaying terminology from fhirlint's own local server cannot use it.
func TestBlocksJARNetwork(t *testing.T) {
	cases := []struct {
		name string
		opts Options
		want bool
	}{
		{"offline, terminology skipped", Options{Offline: true, NoTerminologyServer: true}, true},
		{"offline, replaying from loopback", Options{Offline: true, TerminologyServer: "http://127.0.0.1:8081"}, false},
		{"offline, remote terminology", Options{Offline: true, TerminologyServer: "https://tx.fhir.org"}, false},
		{"not offline", Options{NoTerminologyServer: true}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := blocksJARNetwork(tc.opts); got != tc.want {
				t.Errorf("blocksJARNetwork = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBuildArgs_NoHTTPAccess(t *testing.T) {
	args := buildArgs("jar", []string{"p.json"}, "out",
		Options{FHIRVersion: "4.0.1", Offline: true, NoTerminologyServer: true})
	if !containsArg(args, "-no-http-access") {
		t.Errorf("offline run did not block the JAR's network: %v", args)
	}

	// Replaying terminology: the block would cut off the replay server too.
	args = buildArgs("jar", []string{"p.json"}, "out",
		Options{FHIRVersion: "4.0.1", Offline: true, TerminologyServer: "http://127.0.0.1:8081"})
	if containsArg(args, "-no-http-access") {
		t.Errorf("replay run passed -no-http-access, which would block its own server: %v", args)
	}

	args = buildArgs("jar", []string{"p.json"}, "out", Options{FHIRVersion: "4.0.1"})
	if containsArg(args, "-no-http-access") {
		t.Errorf("ordinary run blocked the JAR's network: %v", args)
	}
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func TestCheckOptionsSupported_NoHTTPAccessNeedsVersion(t *testing.T) {
	offline := Options{Offline: true, NoTerminologyServer: true}

	if err := checkOptionsSupported(offline, "6.9.6"); err == nil {
		t.Error("err = nil for a JAR without -no-http-access, want a rejection")
	} else if !strings.Contains(err.Error(), "--offline") || !strings.Contains(err.Error(), "6.10.0") {
		t.Errorf("err = %q, want it to name the flag and the required version", err)
	}

	if err := checkOptionsSupported(offline, "6.10.2"); err != nil {
		t.Errorf("checkOptionsSupported(6.10.2) = %v, want nil", err)
	}

	// Replaying terminology means no -no-http-access, so no version floor.
	replay := Options{Offline: true, TerminologyServer: "http://127.0.0.1:8081"}
	if err := checkOptionsSupported(replay, "6.9.6"); err != nil {
		t.Errorf("replay run rejected on an older JAR: %v", err)
	}
}

// An offline run must fail before EnsureJAR reaches for the network, and only
// when there is nothing cached to run.
func TestRequireCachedJAR(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(cache.DirEnvVar, dir)

	err := requireCachedJAR(Options{Offline: true})
	if err == nil {
		t.Fatal("err = nil with an empty cache, want a refusal to download")
	}
	if !strings.Contains(err.Error(), "fhirlint update") {
		t.Errorf("err = %q, want it to say how to populate the cache", err)
	}

	// An explicit --jar is the user pointing at a local file: nothing to fetch.
	if err := requireCachedJAR(Options{Offline: true, JARPath: "/some/local.jar"}); err != nil {
		t.Errorf("explicit JAR path rejected: %v", err)
	}

	// Not offline: EnsureJAR may download as usual.
	if err := requireCachedJAR(Options{}); err != nil {
		t.Errorf("online run rejected: %v", err)
	}

	jar := filepath.Join(dir, "validator_cli.jar")
	if writeErr := os.WriteFile(jar, []byte("not really a jar"), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	if err := requireCachedJAR(Options{Offline: true}); err != nil {
		t.Errorf("cached JAR rejected: %v", err)
	}
}
