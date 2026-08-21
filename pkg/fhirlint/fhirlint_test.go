package fhirlint

import (
	"strings"
	"testing"
	"time"

	"github.com/fhirlint/fhirlint/internal/validator"
)

func TestToInternalOpts_DefaultFHIRVersion(t *testing.T) {
	opts := toInternalOpts(Options{})
	if opts.FHIRVersion != "4.0.1" {
		t.Errorf("expected default FHIRVersion=4.0.1, got %q", opts.FHIRVersion)
	}
}

func TestToInternalOpts_ExplicitFHIRVersion(t *testing.T) {
	opts := toInternalOpts(Options{FHIRVersion: "5.0.0"})
	if opts.FHIRVersion != "5.0.0" {
		t.Errorf("expected FHIRVersion=5.0.0, got %q", opts.FHIRVersion)
	}
}

func TestToInternalOpts_AllFields(t *testing.T) {
	in := Options{
		FHIRVersion:              "4.3.0",
		Profiles:                 []string{"http://example.com/profile"},
		IGs:                      []string{"kbv.basis#1.5.0"},
		NoTerminologyServer:      true,
		TerminologyServer:        "https://my-tx.example.com",
		BestPractice:             "ignore",
		TxCache:                  "/tmp/tx",
		Locale:                   "de",
		AllowExampleURLs:         true,
		AllowInsecureTx:          true,
		TxLog:                    "/tmp/tx.log",
		Jurisdiction:             "urn:iso:std:iso:3166#DE",
		DisplayIssuesAreWarnings: true,
		POFiles:                  []string{"validator-messages-de.po"},
		JARPath:                  "/opt/validator.jar",
		Timeout:                  5 * time.Minute,
		HTTPTimeout:              45 * time.Second,
	}
	out := toInternalOpts(in)

	if out.FHIRVersion != "4.3.0" {
		t.Errorf("FHIRVersion: got %q", out.FHIRVersion)
	}
	if len(out.Profiles) != 1 {
		t.Errorf("Profiles: expected 1, got %d", len(out.Profiles))
	}
	if out.IGs[0] != "kbv.basis#1.5.0" {
		t.Errorf("IGs: got %q", out.IGs[0])
	}
	if !out.NoTerminologyServer {
		t.Error("NoTerminologyServer should be true")
	}
	if out.TerminologyServer != "https://my-tx.example.com" {
		t.Errorf("TerminologyServer: got %q", out.TerminologyServer)
	}
	if out.BestPractice != "ignore" {
		t.Errorf("BestPractice: got %q", out.BestPractice)
	}
	if out.TxCache != "/tmp/tx" {
		t.Errorf("TxCache: got %q", out.TxCache)
	}
	if out.Locale != "de" {
		t.Errorf("Locale: got %q", out.Locale)
	}
	if !out.AllowExampleURLs {
		t.Error("AllowExampleURLs should be true")
	}
	if !out.AllowInsecureTx {
		t.Error("AllowInsecureTx should be true")
	}
	if out.TxLog != "/tmp/tx.log" {
		t.Errorf("TxLog: got %q", out.TxLog)
	}
	if out.Jurisdiction != "urn:iso:std:iso:3166#DE" {
		t.Errorf("Jurisdiction: got %q", out.Jurisdiction)
	}
	if !out.DisplayIssuesAreWarnings {
		t.Error("DisplayIssuesAreWarnings should be true")
	}
	if len(out.POFiles) != 1 || out.POFiles[0] != "validator-messages-de.po" {
		t.Errorf("POFiles: got %v", out.POFiles)
	}
	if out.JARPath != "/opt/validator.jar" {
		t.Errorf("JARPath: got %q", out.JARPath)
	}
	if out.Timeout != 5*time.Minute {
		t.Errorf("Timeout: got %v", out.Timeout)
	}
}

func TestToInternalOpts_TimeoutZero_NoTimeout(t *testing.T) {
	out := toInternalOpts(Options{Timeout: 0})
	if out.Timeout != 0 {
		t.Errorf("expected zero Timeout to pass through as 0, got %v", out.Timeout)
	}
}

func TestToPublicResult_Valid(t *testing.T) {
	r := &validator.Result{
		Label:  "patient.json",
		Valid:  true,
		Issues: []validator.Issue{},
	}
	pub := toPublicResult(r)
	if !pub.Valid {
		t.Error("expected Valid=true")
	}
	if pub.Label != "patient.json" {
		t.Errorf("Label: got %q", pub.Label)
	}
	if len(pub.Issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(pub.Issues))
	}
}

func TestToPublicResult_Issues(t *testing.T) {
	r := &validator.Result{
		Valid: false,
		Issues: []validator.Issue{
			{
				Severity:  "error",
				Message:   "Missing required field",
				Location:  "Patient.name (line 3, col 5)",
				MessageID: "req-1",
			},
		},
	}
	pub := toPublicResult(r)
	if pub.Valid {
		t.Error("expected Valid=false")
	}
	if len(pub.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(pub.Issues))
	}
	iss := pub.Issues[0]
	if iss.Severity != "error" {
		t.Errorf("Severity: got %q", iss.Severity)
	}
	if iss.Message != "Missing required field" {
		t.Errorf("Message: got %q", iss.Message)
	}
	if iss.Location != "Patient.name (line 3, col 5)" {
		t.Errorf("Location: got %q", iss.Location)
	}
	if iss.MessageID != "req-1" {
		t.Errorf("MessageID: got %q", iss.MessageID)
	}
}

func TestValidate_DetectsXML(t *testing.T) {
	// Should fail with a JAR error, not a format detection error.
	// We just verify the function doesn't panic on XML input.
	_, _ = Validate([]byte(`<Patient xmlns="http://hl7.org/fhir"/>`), Options{
		FHIRVersion: "4.0.1",
		JARPath:     "/nonexistent/validator.jar",
	})
}

func TestValidate_DetectsJSON(t *testing.T) {
	_, _ = Validate([]byte(`{"resourceType":"Patient"}`), Options{
		FHIRVersion: "4.0.1",
		JARPath:     "/nonexistent/validator.jar",
	})
}

func TestOptions_ExtraArgsReachInternalOpts(t *testing.T) {
	opts := Options{ExtraArgs: []string{"-some-new-flag", "value"}}
	internal := toInternalOpts(opts)
	if len(internal.ExtraArgs) != 2 ||
		internal.ExtraArgs[0] != "-some-new-flag" || internal.ExtraArgs[1] != "value" {
		t.Errorf("ExtraArgs = %v, want [-some-new-flag value]", internal.ExtraArgs)
	}
}

func TestToInternalOpts_PassesRunBounds(t *testing.T) {
	got := toInternalOpts(Options{
		FHIRVersion:       "4.0.1",
		ValidationTimeout: 2 * time.Minute,
		MaxMessages:       500,
	})
	if got.ValidationTimeout != 2*time.Minute {
		t.Errorf("ValidationTimeout = %v, want %v", got.ValidationTimeout, 2*time.Minute)
	}
	if got.MaxMessages != 500 {
		t.Errorf("MaxMessages = %d, want 500", got.MaxMessages)
	}
}

func TestApplyRedaction_StripsMessagesWhenEnabled(t *testing.T) {
	r := &validator.Result{Issues: []validator.Issue{{
		Severity:  "error",
		Message:   "The value '1974-03-11' is not a valid date",
		Location:  "Patient.birthDate",
		MessageID: "Type_Specific_Checks_DT_Date_Valid",
	}}}

	applyRedaction(Options{Redact: true}, r)
	got := toPublicResult(r)

	if strings.Contains(got.Issues[0].Message, "1974-03-11") {
		t.Errorf("message still carries the value: %q", got.Issues[0].Message)
	}
	if !got.Issues[0].Redacted {
		t.Error("Redacted must reach the public Issue")
	}
	// The handles a caller acts on have to survive, or the option is useless
	// rather than merely safe.
	if got.Issues[0].MessageID != "Type_Specific_Checks_DT_Date_Valid" {
		t.Errorf("messageID = %q", got.Issues[0].MessageID)
	}
	if got.Issues[0].Location != "Patient.birthDate" {
		t.Errorf("location = %q", got.Issues[0].Location)
	}
}

func TestApplyRedaction_LeavesResultsAloneByDefault(t *testing.T) {
	msg := "The value '1974-03-11' is not a valid date"
	r := &validator.Result{Issues: []validator.Issue{{Severity: "error", Message: msg}}}

	applyRedaction(Options{}, r)
	got := toPublicResult(r)

	if got.Issues[0].Message != msg {
		t.Errorf("message = %q, want it untouched", got.Issues[0].Message)
	}
	if got.Issues[0].Redacted {
		t.Error("Redacted must stay false when the option is off")
	}
}

func TestToInternalOpts_CodeSystemSizeLimit(t *testing.T) {
	// nil leaves the validator's default alone.
	if got := toInternalOpts(Options{}).CodeSystemSizeLimit; got != nil {
		t.Errorf("unset CodeSystemSizeLimit = %v, want nil", *got)
	}

	// 0 is a real setting upstream ("no limit"), so it must survive the trip.
	zero := 0
	got := toInternalOpts(Options{CodeSystemSizeLimit: &zero}).CodeSystemSizeLimit
	if got == nil || *got != 0 {
		t.Errorf("CodeSystemSizeLimit = %v, want a pointer to 0", got)
	}

	limit := 5000
	got = toInternalOpts(Options{CodeSystemSizeLimit: &limit}).CodeSystemSizeLimit
	if got == nil || *got != 5000 {
		t.Errorf("CodeSystemSizeLimit = %v, want a pointer to 5000", got)
	}
}
