package fhirlint

import (
	"testing"

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
		FHIRVersion:         "4.3.0",
		Profiles:            []string{"http://example.com/profile"},
		IGs:                 []string{"kbv.basis#1.5.0"},
		NoTerminologyServer: true,
		TerminologyServer:   "https://my-tx.example.com",
		BestPractice:        "ignore",
		TxCache:             "/tmp/tx",
		Locale:              "de",
		AllowExampleURLs:    true,
		JARPath:             "/opt/validator.jar",
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
	if out.JARPath != "/opt/validator.jar" {
		t.Errorf("JARPath: got %q", out.JARPath)
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
