package validator

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildArgs_RequiredFlags(t *testing.T) {
	args := buildArgs("/fake/validator.jar", []string{"/tmp/patient.json"}, "/tmp/out.json", Options{
		FHIRVersion: "4.0.1",
	})

	mustContainPair(t, args, "-jar", "/fake/validator.jar")
	mustContainPair(t, args, "-version", "4.0.1")
	mustContainPair(t, args, "-output-style", "json")
	mustContainPair(t, args, "-output", "/tmp/out.json")
	mustContain(t, args, "/tmp/patient.json")
}

func TestBuildArgs_NoTerminologyServer(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion:         "4.0.1",
		NoTerminologyServer: true,
	})

	mustContainPair(t, args, "-tx", "n/a")
}

func TestBuildArgs_CustomTerminologyServer(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion:       "4.0.1",
		TerminologyServer: "https://my-tx.example.com",
	})

	mustContainPair(t, args, "-tx", "https://my-tx.example.com")
}

func TestBuildArgs_NoTerminologyServerTakesPrecedence(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion:         "4.0.1",
		NoTerminologyServer: true,
		TerminologyServer:   "https://my-tx.example.com",
	})

	mustContainPair(t, args, "-tx", "n/a")
	mustNotContain(t, args, "https://my-tx.example.com")
}

func TestBuildArgs_DefaultHasNoTxFlag(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{FHIRVersion: "4.0.1"})

	mustNotContain(t, args, "-tx")
}

func TestBuildArgs_Profiles(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion: "4.0.1",
		Profiles:    []string{"http://example.com/profile1", "http://example.com/profile2"},
	})

	mustContainPair(t, args, "-profile", "http://example.com/profile1")
	mustContainPair(t, args, "-profile", "http://example.com/profile2")
}

func TestBuildArgs_ProfileIGRef_RoutedToIG(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion: "4.0.1",
		Profiles:    []string{"kbv.basis#1.5.0"},
	})

	mustContainPair(t, args, "-ig", "kbv.basis#1.5.0")
	mustNotContain(t, args, "-profile")
}

func TestBuildArgs_ProfileURL_RoutedToProfile(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion: "4.0.1",
		Profiles:    []string{"http://example.com/StructureDefinition/MyProfile"},
	})

	mustContainPair(t, args, "-profile", "http://example.com/StructureDefinition/MyProfile")
	mustNotContain(t, args, "-ig")
}

func TestBuildArgs_MixedProfilesAndIGRefs(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion: "4.0.1",
		Profiles: []string{
			"kbv.basis#1.5.0",
			"http://example.com/StructureDefinition/MyProfile",
		},
	})

	mustContainPair(t, args, "-ig", "kbv.basis#1.5.0")
	mustContainPair(t, args, "-profile", "http://example.com/StructureDefinition/MyProfile")
}

func TestBuildArgs_IGs(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion: "4.0.1",
		IGs:         []string{"kbv.basis#1.5.0", "de.medizininformatikinitiative.kerndatensatz.person#2025.0.1"},
	})

	mustContainPair(t, args, "-ig", "kbv.basis#1.5.0")
	mustContainPair(t, args, "-ig", "de.medizininformatikinitiative.kerndatensatz.person#2025.0.1")
}

func TestBuildArgs_FHIRVersions(t *testing.T) {
	for _, version := range []string{"4.0.1", "4.3.0", "5.0.0"} {
		args := buildArgs("jar", []string{"input"}, "out", Options{FHIRVersion: version})
		mustContainPair(t, args, "-version", version)
	}
}

func TestBuildArgs_EmptyProfilesAndIGs(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion: "4.0.1",
		Profiles:    []string{},
		IGs:         []string{},
	})

	mustNotContain(t, args, "-profile")
	mustNotContain(t, args, "-ig")
}

func TestBuildArgs_BestPracticeIgnore(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion:  "4.0.1",
		BestPractice: "ignore",
	})
	mustContainPair(t, args, "-best-practice", "ignore")
}

func TestBuildArgs_BestPracticeError(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion:  "4.0.1",
		BestPractice: "error",
	})
	mustContainPair(t, args, "-best-practice", "error")
}

func TestBuildArgs_BestPracticeEmptyOmitted(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{FHIRVersion: "4.0.1"})
	mustNotContain(t, args, "-best-practice")
}

func TestBuildArgs_TxCache(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion: "4.0.1",
		TxCache:     "/tmp/tx-cache",
	})
	mustContainPair(t, args, "-txCache", "/tmp/tx-cache")
}

func TestBuildArgs_TxCacheDisabled(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion: "4.0.1",
		TxCache:     "n/a",
	})
	mustContainPair(t, args, "-txCache", "n/a")
}

func TestBuildArgs_TxCacheEmptyOmitted(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{FHIRVersion: "4.0.1"})
	mustNotContain(t, args, "-txCache")
}

func TestBuildArgs_Locale(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion: "4.0.1",
		Locale:      "de",
	})
	mustContainPair(t, args, "-locale", "de")
}

func TestBuildArgs_LocaleEmptyOmitted(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{FHIRVersion: "4.0.1"})
	mustNotContain(t, args, "-locale")
}

func TestBuildArgs_AllowExampleURLs(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion:      "4.0.1",
		AllowExampleURLs: true,
	})
	mustContain(t, args, "-allow-example-urls")
}

func TestBuildArgs_AllowExampleURLsFalseOmitted(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{FHIRVersion: "4.0.1"})
	mustNotContain(t, args, "-allow-example-urls")
}

func TestBuildArgs_EmptyOutputPath_OmitsOutputFlags(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "", Options{FHIRVersion: "4.0.1"})
	mustNotContain(t, args, "-output-style")
	mustNotContain(t, args, "-output")
}

func TestBuildArgs_WithOutputPath_IncludesOutputFlags(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "/tmp/out.json", Options{FHIRVersion: "4.0.1"})
	mustContainPair(t, args, "-output-style", "json")
	mustContainPair(t, args, "-output", "/tmp/out.json")
}

func TestBuildArgs_MultipleInputPaths(t *testing.T) {
	paths := []string{"/tmp/a.json", "/tmp/b.json", "/tmp/c.json"}
	args := buildArgs("jar", paths, "out", Options{FHIRVersion: "4.0.1"})

	for _, p := range paths {
		mustContain(t, args, p)
	}
}

func TestParseOutput_OperationOutcome(t *testing.T) {
	oo := fixtureOO(t, "oo-error.json")
	data, err := encodeJSON(oo)
	if err != nil {
		t.Fatalf("encoding fixture: %v", err)
	}
	results, err := parseOutput(data, []string{"patient.json"}, "")
	if err != nil {
		t.Fatalf("parseOutput error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Filename != "patient.json" {
		t.Errorf("expected filename=patient.json, got %q", results[0].Filename)
	}
}

func TestParseOutput_Bundle(t *testing.T) {
	oo1 := fixtureOO(t, "oo-no-issues.json")
	oo2 := fixtureOO(t, "oo-error.json")
	bundle := map[string]interface{}{
		"resourceType": "Bundle",
		"type":         "collection",
		"entry": []map[string]interface{}{
			{"fullUrl": "file:///tmp/a.json", "resource": oo1},
			{"fullUrl": "file:///tmp/b.json", "resource": oo2},
		},
	}
	data, err := encodeJSON(bundle)
	if err != nil {
		t.Fatalf("encoding bundle: %v", err)
	}
	results, err := parseOutput(data, []string{"/tmp/a.json", "/tmp/b.json"}, "")
	if err != nil {
		t.Fatalf("parseOutput error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Filename != "/tmp/a.json" {
		t.Errorf("expected filename=/tmp/a.json, got %q", results[0].Filename)
	}
	if results[1].Filename != "/tmp/b.json" {
		t.Errorf("expected filename=/tmp/b.json, got %q", results[1].Filename)
	}
	if !results[0].Valid {
		t.Error("first result (no issues) should be valid")
	}
	if results[1].Valid {
		t.Error("second result (error) should be invalid")
	}
}

func TestParseOutput_UnknownResourceType(t *testing.T) {
	data := []byte(`{"resourceType":"Patient","id":"123"}`)
	_, err := parseOutput(data, nil, "")
	if err == nil {
		t.Error("expected error for unknown resourceType")
	}
}

func TestParseOutput_MalformedJSON(t *testing.T) {
	_, err := parseOutput([]byte(`not json`), nil, "")
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

// mustContainPair asserts that args contains flag immediately followed by value.
func mustContainPair(t *testing.T, args []string, flag, value string) {
	t.Helper()
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag && args[i+1] == value {
			return
		}
	}
	t.Errorf("expected args to contain %q %q, got: %v", flag, value, args)
}

// mustContain asserts that args contains the given value.
func mustContain(t *testing.T, args []string, value string) {
	t.Helper()
	for _, a := range args {
		if a == value {
			return
		}
	}
	t.Errorf("expected args to contain %q, got: %v", value, args)
}

// mustNotContain asserts that args does not contain the given value.
func mustNotContain(t *testing.T, args []string, value string) {
	t.Helper()
	for _, a := range args {
		if a == value {
			t.Errorf("expected args NOT to contain %q, got: %v", value, args)
			return
		}
	}
}

func encodeJSON(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func TestBuildArgs_TxLog(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion: "4.0.1",
		TxLog:       "/tmp/tx.log",
	})
	mustContainPair(t, args, "-txLog", "/tmp/tx.log")
}

func TestBuildArgs_TxLogEmpty_Omitted(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{FHIRVersion: "4.0.1"})
	mustNotContain(t, args, "-txLog")
}

func TestBuildArgs_Jurisdiction(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion:  "4.0.1",
		Jurisdiction: "urn:iso:std:iso:3166#DE",
	})
	mustContainPair(t, args, "-jurisdiction", "urn:iso:std:iso:3166#DE")
}

func TestBuildArgs_JurisdictionEmpty_Omitted(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{FHIRVersion: "4.0.1"})
	mustNotContain(t, args, "-jurisdiction")
}

func TestBuildArgs_DisplayIssuesAreWarnings(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion:              "4.0.1",
		DisplayIssuesAreWarnings: true,
	})
	mustContain(t, args, "-display-issues-are-warnings")
}

func TestBuildArgs_DisplayIssuesAreWarningsFalse_Omitted(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{FHIRVersion: "4.0.1"})
	mustNotContain(t, args, "-display-issues-are-warnings")
}

func TestBuildArgs_POFiles(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion: "4.0.1",
		POFiles:     []string{"validator-messages-de.po", "rendering-phrases-de.po"},
	})
	mustContainPair(t, args, "-po", "validator-messages-de.po")
	mustContainPair(t, args, "-po", "rendering-phrases-de.po")
}

func TestBuildArgs_POFilesEmpty_Omitted(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{FHIRVersion: "4.0.1"})
	mustNotContain(t, args, "-po")
}

func TestValidateFHIRVersion_Valid(t *testing.T) {
	for _, v := range []string{"4.0.1", "4.3.0", "5.0.0"} {
		if err := validateFHIRVersion(v); err != nil {
			t.Errorf("expected %q to be valid, got: %v", v, err)
		}
	}
}

func TestValidateFHIRVersion_Invalid(t *testing.T) {
	err := validateFHIRVersion("3.0.0")
	if err == nil {
		t.Fatal("expected error for invalid FHIR version")
	}
	if !strings.Contains(err.Error(), "3.0.0") {
		t.Errorf("error should mention the bad value, got: %v", err)
	}
}

func TestValidateBestPractice_Valid(t *testing.T) {
	for _, v := range []string{"", "ignore", "hint", "warning", "error"} {
		if err := validateBestPractice(v); err != nil {
			t.Errorf("expected %q to be valid, got: %v", v, err)
		}
	}
}

func TestValidateBestPractice_Invalid(t *testing.T) {
	err := validateBestPractice("typo")
	if err == nil {
		t.Fatal("expected error for invalid best-practice value")
	}
	if !strings.Contains(err.Error(), "typo") {
		t.Errorf("error should mention the bad value, got: %v", err)
	}
}

func TestWarnInsecureTerminologyServer_HTTP_PrintsWarning(t *testing.T) {
	var buf bytes.Buffer
	warnInsecureTerminologyServer(&buf, Options{TerminologyServer: "http://tx.example.com"})
	if !bytes.Contains(buf.Bytes(), []byte("HTTP")) {
		t.Errorf("expected HTTP warning, got: %q", buf.String())
	}
}

func TestWarnInsecureTerminologyServer_HTTPS_Silent(t *testing.T) {
	var buf bytes.Buffer
	warnInsecureTerminologyServer(&buf, Options{TerminologyServer: "https://tx.example.com"})
	if buf.Len() != 0 {
		t.Errorf("expected no output for HTTPS, got: %q", buf.String())
	}
}

func TestWarnInsecureTerminologyServer_AllowInsecureTx_Silent(t *testing.T) {
	var buf bytes.Buffer
	warnInsecureTerminologyServer(&buf, Options{
		TerminologyServer: "http://tx.example.com",
		AllowInsecureTx:   true,
	})
	if buf.Len() != 0 {
		t.Errorf("expected no output when AllowInsecureTx=true, got: %q", buf.String())
	}
}

func TestWarnInsecureTerminologyServer_Empty_Silent(t *testing.T) {
	var buf bytes.Buffer
	warnInsecureTerminologyServer(&buf, Options{})
	if buf.Len() != 0 {
		t.Errorf("expected no output for empty URL, got: %q", buf.String())
	}
}

func TestBuildArgs_ExtraArgsAppendedLast(t *testing.T) {
	opts := Options{
		FHIRVersion: "4.0.1",
		ExtraArgs:   []string{"-some-new-flag", "value"},
	}
	args := buildArgs("/jar.jar", []string{"a.json"}, "/tmp/out.json", opts)

	if len(args) < 2 {
		t.Fatalf("unexpected args: %v", args)
	}
	if got := args[len(args)-2:]; got[0] != "-some-new-flag" || got[1] != "value" {
		t.Errorf("extra args must come last, got tail %v in %v", got, args)
	}
	// The managed output flags must survive untouched.
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-output-style json") || !strings.Contains(joined, "-output /tmp/out.json") {
		t.Errorf("managed output flags missing: %v", args)
	}
}

func TestBuildArgs_NoExtraArgs_UnchangedTail(t *testing.T) {
	opts := Options{FHIRVersion: "4.0.1"}
	args := buildArgs("/jar.jar", []string{"a.json"}, "/tmp/out.json", opts)
	for _, a := range args {
		if a == "-some-new-flag" {
			t.Fatalf("unexpected extra arg in %v", args)
		}
	}
}

func TestValidateExtraArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{"empty", nil, false},
		{"harmless flag", []string{"-some-new-flag", "value"}, false},
		{"reserved output", []string{"-output", "/tmp/x"}, true},
		{"reserved output-style", []string{"-output-style", "text"}, true},
		{"reserved jar", []string{"-jar", "/other.jar"}, true},
		{"reserved with equals", []string{"-output=/tmp/x"}, true},
		{"reserved double dash", []string{"--output-style"}, true},
		{"reserved mixed case", []string{"-OUTPUT"}, true},
		{"reserved among others", []string{"-fine", "-output"}, true},
		// A value that merely looks like a reserved flag is still an argument
		// position, so it is checked too — conservative on purpose.
		{"non-reserved lookalike", []string{"-outputs"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateExtraArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateExtraArgs(%v) error = %v, wantErr %v", tt.args, err, tt.wantErr)
			}
		})
	}
}

func TestBuildArgs_ValidationTimeoutInMilliseconds(t *testing.T) {
	args := buildArgs("jar", []string{"in.json"}, "out.json", Options{
		FHIRVersion:       "4.0.1",
		ValidationTimeout: 90 * time.Second,
	})
	// The JAR takes milliseconds, so a Go duration has to be converted rather
	// than printed.
	mustContainPair(t, args, "-validation-timeout", "90000")
}

func TestBuildArgs_ValidationTimeoutOmittedWhenZero(t *testing.T) {
	args := buildArgs("jar", []string{"in.json"}, "out.json", Options{FHIRVersion: "4.0.1"})
	for _, a := range args {
		if a == "-validation-timeout" {
			t.Error("an unset validation timeout must not be passed to the JAR")
		}
	}
}

func TestBuildArgs_MaxMessages(t *testing.T) {
	args := buildArgs("jar", []string{"in.json"}, "out.json", Options{
		FHIRVersion: "4.0.1",
		MaxMessages: 500,
	})
	mustContainPair(t, args, "-max-validation-messages", "500")
}

func TestBuildArgs_MaxMessagesOmittedWhenZero(t *testing.T) {
	args := buildArgs("jar", []string{"in.json"}, "out.json", Options{FHIRVersion: "4.0.1"})
	for _, a := range args {
		if a == "-max-validation-messages" {
			t.Error("an unset message cap must not be passed to the JAR")
		}
	}
}

// Sub-second durations must not silently truncate to 0, which the JAR would
// read as "no bound" — the opposite of what was asked for.
func TestBuildArgs_SubSecondValidationTimeout(t *testing.T) {
	args := buildArgs("jar", []string{"in.json"}, "out.json", Options{
		FHIRVersion:       "4.0.1",
		ValidationTimeout: 250 * time.Millisecond,
	})
	mustContainPair(t, args, "-validation-timeout", "250")
}

func TestDefaultTerminologyEndpoint(t *testing.T) {
	// The JAR appends a version path to its own default but uses an explicit
	// -tx URL verbatim, so recording has to reconstruct exactly this.
	// R4B maps to /r4: tx.fhir.org serves no /r4b endpoint.
	tests := []struct{ version, want string }{
		{"4.0.1", "https://tx.fhir.org/r4"},
		{"4.3.0", "https://tx.fhir.org/r4"},
		{"5.0.0", "https://tx.fhir.org/r5"},
		{"", "https://tx.fhir.org/r4"},
	}
	for _, tt := range tests {
		if got := DefaultTerminologyEndpoint(tt.version); got != tt.want {
			t.Errorf("DefaultTerminologyEndpoint(%q) = %q, want %q", tt.version, got, tt.want)
		}
	}
}

func TestBuildArgs_CodeSystemSizeLimit(t *testing.T) {
	limit := 5000
	args := buildArgs("jar", []string{"p.json"}, "out", Options{FHIRVersion: "4.0.1", CodeSystemSizeLimit: &limit})
	mustContainPair(t, args, "-codesystem-validation-size-limit", "5000")
}

// Upstream reads 0 as "check every code", so it has to reach the JAR rather
// than being folded into "unset" the way a zero-valued int flag would be.
func TestBuildArgs_CodeSystemSizeLimitZeroMeansNoLimit(t *testing.T) {
	limit := 0
	args := buildArgs("jar", []string{"p.json"}, "out", Options{FHIRVersion: "4.0.1", CodeSystemSizeLimit: &limit})
	mustContainPair(t, args, "-codesystem-validation-size-limit", "0")
}

// The zero value of Options must not request anything: every existing caller
// builds Options without this field, and a validator older than 6.10.2 fails
// outright on the unknown parameter.
func TestBuildArgs_CodeSystemSizeLimitUnsetPassesNothing(t *testing.T) {
	args := buildArgs("jar", []string{"p.json"}, "out", Options{FHIRVersion: "4.0.1"})
	for _, a := range args {
		if a == "-codesystem-validation-size-limit" {
			t.Fatalf("unset limit still passed the argument: %v", args)
		}
	}
}

// The watch arguments used to be built inline in RunWatch, where nothing could
// assert them, and fhirlint sent -watch-interval for years — an option the
// validator has never defined. picocli fails the run on an unknown option, so
// the flag name is the whole feature (#405).
func TestWatchArgs_Mode(t *testing.T) {
	args := watchArgs(WatchConfig{Mode: "all"})

	mustContainPair(t, args, "-watch-mode", "all")
	if len(args) != 2 {
		t.Errorf("watchArgs(mode only) = %v, want the mode and nothing else", args)
	}
}

func TestWatchArgs_IntervalIsScanDelay(t *testing.T) {
	args := watchArgs(WatchConfig{Mode: "single", ScanDelayMS: 500})

	mustContainPair(t, args, "-watch-mode", "single")
	mustContainPair(t, args, "-watch-scan-delay", "500")

	for _, a := range args {
		if a == "-watch-interval" {
			t.Error("watchArgs sent -watch-interval; the validator only knows -watch-scan-delay")
		}
	}
}

func TestWatchArgs_ZeroIntervalOmitted(t *testing.T) {
	for _, ms := range []int{0, -1} {
		args := watchArgs(WatchConfig{Mode: "single", ScanDelayMS: ms})
		for _, a := range args {
			if a == "-watch-scan-delay" {
				t.Errorf("watchArgs(scan delay %d) sent -watch-scan-delay; %d means leave the JAR default alone", ms, ms)
			}
		}
	}
}

// Settle time is the debounce after a change is seen, not the polling period.
// It is the knob that helps when a generator writes a directory and validation
// would otherwise start against a half-written tree (#425).
func TestWatchArgs_SettleTime(t *testing.T) {
	args := watchArgs(WatchConfig{Mode: "single", SettleTimeMS: 750})

	mustContainPair(t, args, "-watch-mode", "single")
	mustContainPair(t, args, "-watch-settle-time", "750")
}

func TestWatchArgs_ZeroSettleTimeOmitted(t *testing.T) {
	for _, ms := range []int{0, -1} {
		args := watchArgs(WatchConfig{Mode: "single", SettleTimeMS: ms})
		for _, a := range args {
			if a == "-watch-settle-time" {
				t.Errorf("watchArgs(settle time %d) sent -watch-settle-time; %d means leave the JAR default alone", ms, ms)
			}
		}
	}
}

// The two delays are independent and must not be confused for one another: the
// struct exists so that transposing them is not a silent mistake, and this
// pins which value lands on which flag.
func TestWatchArgs_ScanDelayAndSettleTimeAreDistinct(t *testing.T) {
	args := watchArgs(WatchConfig{Mode: "all", ScanDelayMS: 2000, SettleTimeMS: 750})

	mustContainPair(t, args, "-watch-mode", "all")
	mustContainPair(t, args, "-watch-scan-delay", "2000")
	mustContainPair(t, args, "-watch-settle-time", "750")
}

func TestBuildArgs_ExpansionParameters(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion:         "4.0.1",
		ExpansionParameters: "/tmp/expansion-params.json",
	})

	mustContainPair(t, args, "-expansion-parameters", "/tmp/expansion-params.json")
}

func TestBuildArgs_ExpansionParametersEmpty_Omitted(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{FHIRVersion: "4.0.1"})

	for _, a := range args {
		if a == "-expansion-parameters" {
			t.Error("-expansion-parameters passed with no file; the validator has its own defaults")
		}
	}
}

// The validator reports an unreadable file as a FHIRException with a stack
// trace attached, arriving as "the JAR failed" rather than "you typed the path
// wrong" (#351). A mistyped path is the common case, so it is answered here.
func TestValidateExpansionParameters(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "params.json")
	if err := os.WriteFile(file, []byte(`{"resourceType":"Parameters"}`), 0600); err != nil {
		t.Fatal(err)
	}

	if err := validateExpansionParameters(""); err != nil {
		t.Errorf("empty path: got %v, want nil — the option is optional", err)
	}
	if err := validateExpansionParameters(file); err != nil {
		t.Errorf("readable file: got %v, want nil", err)
	}

	err := validateExpansionParameters(filepath.Join(dir, "nope.json"))
	if err == nil || !strings.Contains(err.Error(), "--expansion-parameters") {
		t.Errorf("missing file: got %v, want an error naming the flag", err)
	}

	err = validateExpansionParameters(dir)
	if err == nil || !strings.Contains(err.Error(), "directory") {
		t.Errorf("directory: got %v, want an error saying it is a directory", err)
	}
}

// From 6.10.5 the validator states what it is on every OperationOutcome, as
// the validator-version extension (hapifhir/org.hl7.fhir.core#2459). A report
// should carry the JAR's own statement when it makes one (#428).
func TestParseOutput_ValidatorVersionExtension(t *testing.T) {
	const build = "FHIR Validation tool Version 6.10.5 (Git# e9cb40e7b4ea). Built 2026-09-10T00:02:31.157+10:00 (6 days old)"
	oo := `{"resourceType":"OperationOutcome","extension":[{"url":"http://hl7.org/fhir/tools/StructureDefinition/validator-version","valueString":"` + build + `"}],"issue":[]}`

	results, err := parseOutput([]byte(oo), []string{"a.json"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if results[0].ValidatorBuild != build {
		t.Errorf("ValidatorBuild = %q, want the extension's value", results[0].ValidatorBuild)
	}

	bundle := `{"resourceType":"Bundle","entry":[{"resource":` + oo + `},{"resource":` + oo + `}]}`
	results, err = parseOutput([]byte(bundle), []string{"a.json", "b.json"}, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.ValidatorBuild != build {
			t.Errorf("%s: ValidatorBuild = %q, want the extension's value", r.Filename, r.ValidatorBuild)
		}
	}
}

// A JAR that does not state its version — every release before 6.10.5 —
// leaves the field empty rather than inventing one.
func TestParseOutput_NoValidatorVersionExtension(t *testing.T) {
	results, err := parseOutput([]byte(`{"resourceType":"OperationOutcome","issue":[]}`), []string{"a.json"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if results[0].ValidatorBuild != "" {
		t.Errorf("ValidatorBuild = %q, want empty", results[0].ValidatorBuild)
	}
}
