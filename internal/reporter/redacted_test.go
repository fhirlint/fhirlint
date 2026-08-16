package reporter_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/redact"
	"github.com/fhirlint/fhirlint/internal/reporter"
	"github.com/fhirlint/fhirlint/internal/validator"
)

// phi is the value the validator quoted into its message. The point of this
// file is that it reaches no output, whichever format is asked for — the claim
// --redact makes is about every report, not about the ones anyone remembered.
const phi = "1974-03-11"

// sourceLine is what --show-source would render. It lives in a real file so
// that a reporter which still tried to read it would succeed, and the test
// would catch it rather than passing because the path happened to be broken.
const sourceLine = `  "birthDate": "1974-03-11"`

func redactedResults(t *testing.T) []*validator.Result {
	t.Helper()

	dir := t.TempDir()
	src := filepath.Join(dir, "patient.json")
	body := "{\n" + sourceLine + "\n}\n"
	if err := os.WriteFile(src, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}

	results := []*validator.Result{{
		Filename:   src,
		Label:      "patients/p1.json",
		SourcePath: src,
		Valid:      false,
		Issues: []validator.Issue{{
			Severity:  "error",
			Message:   "The value '" + phi + "' is not a valid date",
			Location:  "Patient.birthDate (line 2, col 16)",
			MessageID: "Type_Specific_Checks_DT_Date_Valid",
		}},
		Suppressed: []validator.Issue{{
			Severity:       "warning",
			Message:        "Narrative for '" + phi + "' is missing",
			Location:       "Patient",
			MessageID:      "dom-6",
			SuppressReason: "accepted",
		}},
	}}

	redact.Apply(results)
	return results
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	fn()
	_ = w.Close()
	return <-done
}

func TestRedactedResultsLeakNothingToStdoutReporters(t *testing.T) {
	// showSource is deliberately passed as true: --redact is supposed to make
	// the snippet unrenderable, not merely to turn the flag off upstream.
	cases := map[string]func(results []*validator.Result){
		"terminal": func(rs []*validator.Result) {
			for _, r := range rs {
				reporter.Terminal(r, "information", true, false, true)
			}
			reporter.TerminalSummary(rs, "information")
		},
		"terminal --group": func(rs []*validator.Result) {
			reporter.TerminalGrouped(rs, "information", true, true)
		},
		"github": func(rs []*validator.Result) {
			if err := reporter.GitHub(rs, "information"); err != nil {
				t.Fatal(err)
			}
		},
	}

	for name, render := range cases {
		t.Run(name, func(t *testing.T) {
			out := captureStdout(t, func() { render(redactedResults(t)) })
			assertClean(t, name, out)
		})
	}
}

func TestRedactedResultsLeakNothingToFileReporters(t *testing.T) {
	cases := map[string]func(results []*validator.Result, dest string) error{
		"json": func(rs []*validator.Result, dest string) error {
			return reporter.JSON(rs, "information", dest)
		},
		"html": func(rs []*validator.Result, dest string) error {
			return reporter.HTML(rs, "information", "4.0.1", dest)
		},
		"junit": func(rs []*validator.Result, dest string) error {
			return reporter.JUnit(rs, "information", dest)
		},
		"sarif": func(rs []*validator.Result, dest string) error {
			return reporter.SARIF(rs, "information", "1.0.0", dest)
		},
		"markdown": func(rs []*validator.Result, dest string) error {
			return reporter.Markdown(rs, "information", dest)
		},
		"codeclimate": func(rs []*validator.Result, dest string) error {
			return reporter.CodeClimate(rs, "information", dest)
		},
	}

	for name, render := range cases {
		t.Run(name, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "report."+name)
			if err := render(redactedResults(t), dest); err != nil {
				t.Fatalf("%s reporter: %v", name, err)
			}
			data, err := os.ReadFile(dest) //nolint:gosec // test-controlled path
			if err != nil {
				t.Fatal(err)
			}
			assertClean(t, name, string(data))
		})
	}
}

func assertClean(t *testing.T, format, out string) {
	t.Helper()
	if strings.Contains(out, phi) {
		t.Errorf("%s output contains the redacted value %q:\n%s", format, phi, out)
	}
	if strings.Contains(out, "is not a valid date") {
		t.Errorf("%s output contains the original message text:\n%s", format, out)
	}
	if strings.Contains(out, strings.TrimSpace(sourceLine)) {
		t.Errorf("%s output contains the source line:\n%s", format, out)
	}
	// The location and message ID are what make a redacted report usable, so a
	// reporter that stripped them too would technically pass the leak check
	// while producing something nobody can act on.
	if !strings.Contains(out, "Type_Specific_Checks_DT_Date_Valid") &&
		!strings.Contains(out, "Patient.birthDate") {
		t.Errorf("%s output kept neither the message ID nor the location:\n%s", format, out)
	}
}

func TestTerminalShowsMessageIDForRedactedFindings(t *testing.T) {
	// With the message gone, the ID is the only handle left on the line. The
	// explain hint below it only appears for IDs fhirlint can explain, so it is
	// not a substitute.
	results := redactedResults(t)
	out := captureStdout(t, func() {
		reporter.Terminal(results[0], "information", false, false, false)
	})

	if !strings.Contains(out, "Type_Specific_Checks_DT_Date_Valid") {
		t.Errorf("want the message ID on the finding line, got:\n%s", out)
	}
	if !strings.Contains(out, redact.Placeholder) {
		t.Errorf("want the redaction placeholder to be visible, got:\n%s", out)
	}
}
