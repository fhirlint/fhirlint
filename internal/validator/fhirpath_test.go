package validator

import (
	"strings"
	"testing"
)

// sampleFHIRPathOutput wraps a result block in the surrounding log noise the JAR
// prints to stdout, so tests exercise the same extraction the real output needs.
func sampleFHIRPathOutput(expr, block string) string {
	return strings.Join([]string{
		"FHIR Validation tool Version 6.9.7",
		"  Params: fhirpath " + expr + " patient.json -version 4.0.1 -tx n/a",
		"Loading",
		"  ...go! (00:04.177)",
		"Cached new session. Cache size = 1",
		" ...evaluating " + expr,
		block,
		"Done. Times: Loading: 00:26.753. Max Memory = 2Gb",
	}, "\n")
}

func TestParseFHIRPathOutput_MultipleItems(t *testing.T) {
	out := sampleFHIRPathOutput("Patient.name.given", "Erika,Maria,Eri")
	r, err := parseFHIRPathOutput(out, "Patient.name.given", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"Erika", "Maria", "Eri"}
	if len(r.Items) != len(want) {
		t.Fatalf("got %d items %q, want %d", len(r.Items), r.Items, len(want))
	}
	for i, w := range want {
		if r.Items[i] != w {
			t.Errorf("item %d = %q, want %q", i, r.Items[i], w)
		}
	}
	if r.Empty() {
		t.Error("Empty() = true, want false")
	}
}

func TestParseFHIRPathOutput_Scalars(t *testing.T) {
	cases := map[string]string{
		"Patient.name.exists()": "true",
		"Patient.name.count()":  "2",
		"Patient.name.first()":  "name=HumanName[4 children]",
	}
	for expr, block := range cases {
		r, err := parseFHIRPathOutput(sampleFHIRPathOutput(expr, block), expr, nil)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", expr, err)
		}
		if len(r.Items) != 1 || r.Items[0] != block {
			t.Errorf("%s: got %q, want single item %q", expr, r.Items, block)
		}
	}
}

func TestParseFHIRPathOutput_Empty(t *testing.T) {
	out := sampleFHIRPathOutput("Patient.name.where(use='foo')", "")
	r, err := parseFHIRPathOutput(out, "Patient.name.where(use='foo')", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.Empty() {
		t.Errorf("Empty() = false, want true (items: %q)", r.Items)
	}
	if r.Items == nil {
		t.Error("Items should be a non-nil empty slice for clean JSON marshalling")
	}
}

func TestParseFHIRPathOutput_MalformedExpression(t *testing.T) {
	block := strings.Join([]string{
		fhirpathErrorLine,
		`org.hl7.fhir.r5.fhirpath.FHIRLexer$FHIRLexerException: Error @1, 15: Premature ExpressionNode termination at unexpected token ".."`,
		"\tat org.hl7.fhir.r5.fhirpath.FHIRLexer.error(FHIRLexer.java:140)",
	}, "\n")
	out := sampleFHIRPathOutput("Patient.name..given(", block)
	_, err := parseFHIRPathOutput(out, "Patient.name..given(", nil)
	if err == nil {
		t.Fatal("expected an error for a malformed expression")
	}
	if !strings.Contains(err.Error(), "Premature ExpressionNode termination") {
		t.Errorf("error = %q, want it to carry the lexer message", err)
	}
	if strings.Contains(err.Error(), "FHIRLexerException") {
		t.Errorf("error = %q, want the exception class prefix stripped", err)
	}
}

func TestParseFHIRPathOutput_UnparseableResource(t *testing.T) {
	block := strings.Join([]string{
		fhirpathErrorLine,
		"java.io.IOException: Error parsing JSON source: JSON syntax error - found String expecting Colon at Line 1 (path=[/json])",
	}, "\n")
	out := sampleFHIRPathOutput("Patient.name", block)
	_, err := parseFHIRPathOutput(out, "Patient.name", nil)
	if err == nil {
		t.Fatal("expected an error for an unparseable resource")
	}
	if !strings.Contains(err.Error(), "Error parsing JSON source") {
		t.Errorf("error = %q, want the JSON parse message", err)
	}
}

func TestParseFHIRPathOutput_NoMarker(t *testing.T) {
	out := strings.Join([]string{
		"FHIR Validation tool Version 6.9.7",
		"Unable to parse command line arguments: Missing required parameter: '<source>'",
	}, "\n")
	_, err := parseFHIRPathOutput(out, "Patient.name", nil)
	if err == nil {
		t.Fatal("expected an error when the evaluating marker is absent")
	}
	if !strings.Contains(err.Error(), "Missing required parameter") {
		t.Errorf("error = %q, want the CLI argument failure surfaced", err)
	}
}

func TestRunFHIRPath_EmptyExpression(t *testing.T) {
	_, err := RunFHIRPath("   ", "patient.json", FHIRPathOptions{FHIRVersion: "4.0.1"})
	if err == nil || !strings.Contains(err.Error(), "empty FHIRPath expression") {
		t.Errorf("error = %v, want empty-expression error", err)
	}
}

func TestRunFHIRPath_InvalidVersion(t *testing.T) {
	_, err := RunFHIRPath("Patient.name", "patient.json", FHIRPathOptions{FHIRVersion: "9.9.9"})
	if err == nil || !strings.Contains(err.Error(), "unknown FHIR version") {
		t.Errorf("error = %v, want unknown-version error", err)
	}
}
