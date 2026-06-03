package cmd

import (
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

func TestPrintFHIRPathTerminal_Empty(t *testing.T) {
	out := captureStdout(t, func() {
		printFHIRPathTerminal(&validator.FHIRPathResult{Expression: "x", Items: []string{}})
	})
	if strings.TrimSpace(out) != "(empty)" {
		t.Errorf("got %q, want %q", strings.TrimSpace(out), "(empty)")
	}
}

func TestPrintFHIRPathTerminal_Scalar(t *testing.T) {
	out := captureStdout(t, func() {
		printFHIRPathTerminal(&validator.FHIRPathResult{Expression: "x", Items: []string{"true"}})
	})
	if strings.TrimSpace(out) != "true" {
		t.Errorf("got %q, want plain %q (no index)", strings.TrimSpace(out), "true")
	}
}

func TestPrintFHIRPathTerminal_MultipleIndexed(t *testing.T) {
	out := captureStdout(t, func() {
		printFHIRPathTerminal(&validator.FHIRPathResult{Expression: "x", Items: []string{"Erika", "Maria"}})
	})
	if !strings.Contains(out, "[0] Erika") || !strings.Contains(out, "[1] Maria") {
		t.Errorf("expected indexed items, got:\n%s", out)
	}
}

func TestPrintFHIRPathJSON(t *testing.T) {
	out := captureStdout(t, func() {
		_ = printFHIRPathJSON(&validator.FHIRPathResult{Expression: "Patient.name.given", Items: []string{"Erika", "Maria"}})
	})
	if !strings.Contains(out, `"expression": "Patient.name.given"`) {
		t.Errorf("expected expression field, got:\n%s", out)
	}
	if !strings.Contains(out, `"result"`) || !strings.Contains(out, `"Erika"`) {
		t.Errorf("expected result array, got:\n%s", out)
	}
}

func TestRunFHIRPath_BadFormat(t *testing.T) {
	flagFHIRPathFormat = "xml"
	defer func() { flagFHIRPathFormat = "terminal" }()
	err := runFHIRPath(nil, []string{"Patient.name"})
	if err == nil || !strings.Contains(err.Error(), "unknown format") {
		t.Errorf("err = %v, want unknown-format error", err)
	}
}
