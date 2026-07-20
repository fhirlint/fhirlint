package reporter

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

func TestGitHub_SeverityMapping(t *testing.T) {
	tests := []struct {
		severity string
		want     string
	}{
		{"fatal", "::error"},
		{"error", "::error"},
		{"warning", "::warning"},
		{"information", "::notice"},
	}
	for _, tt := range tests {
		t.Run(tt.severity, func(t *testing.T) {
			results := []*validator.Result{{
				Filename: "a.json",
				Issues:   []validator.Issue{{Severity: tt.severity, Message: "m"}},
			}}
			var buf bytes.Buffer
			if err := writeGitHub(&buf, results, "information"); err != nil {
				t.Fatalf("writeGitHub: %v", err)
			}
			if !strings.HasPrefix(buf.String(), tt.want) {
				t.Errorf("severity %q -> %q, want prefix %q", tt.severity, buf.String(), tt.want)
			}
		})
	}
}

func TestGitHub_FullCommandLine(t *testing.T) {
	results := []*validator.Result{{
		Filename: "fhir/patient.json",
		Issues: []validator.Issue{{
			Severity:  "error",
			Message:   "Not a valid date format",
			Location:  "Patient.birthDate (line 5, col 28)",
			MessageID: "Type_Specific_Checks_DT_Date_Valid",
		}},
	}}
	var buf bytes.Buffer
	if err := writeGitHub(&buf, results, "information"); err != nil {
		t.Fatalf("writeGitHub: %v", err)
	}
	want := "::error file=fhir/patient.json,line=5,col=28," +
		"title=Type_Specific_Checks_DT_Date_Valid @ Patient.birthDate::Not a valid date format\n"
	if got := buf.String(); got != want {
		t.Errorf("\n got: %q\nwant: %q", got, want)
	}
}

func TestGitHub_NoLineInfo_OmitsLineAndCol(t *testing.T) {
	results := []*validator.Result{{
		Filename: "a.json",
		Issues:   []validator.Issue{{Severity: "error", Message: "m", Location: "Patient"}},
	}}
	var buf bytes.Buffer
	_ = writeGitHub(&buf, results, "information")
	got := buf.String()
	if strings.Contains(got, "line=") || strings.Contains(got, "col=") {
		t.Errorf("no line info available, but got %q", got)
	}
	if !strings.Contains(got, "title=Patient") {
		t.Errorf("expression should still be surfaced as the title, got %q", got)
	}
}

func TestGitHub_EscapesMessage(t *testing.T) {
	results := []*validator.Result{{
		Filename: "a.json",
		Issues: []validator.Issue{{
			Severity: "error",
			// A newline would otherwise terminate the command and dump the rest
			// as plain log output; % must be escaped first or it corrupts the
			// escapes introduced afterwards.
			Message: "line one\nline two 50% done\rcarriage",
		}},
	}}
	var buf bytes.Buffer
	_ = writeGitHub(&buf, results, "information")
	got := strings.TrimSuffix(buf.String(), "\n")

	if strings.Contains(got, "\n") {
		t.Error("raw newline must not survive into the command")
	}
	for _, want := range []string{"%0A", "%25", "%0D"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing escape %s in %q", want, got)
		}
	}
	// "50% done" must become "50%25 done", not "50%2525 done".
	if strings.Contains(got, "%2525") {
		t.Errorf("double-escaped percent in %q", got)
	}
}

func TestGitHub_EscapesPropertySeparators(t *testing.T) {
	results := []*validator.Result{{
		// A comma or colon in a path would otherwise be read as a parameter
		// separator or as the end of the parameter list.
		Filename: "odd,name:1.json",
		Issues:   []validator.Issue{{Severity: "error", Message: "m"}},
	}}
	var buf bytes.Buffer
	_ = writeGitHub(&buf, results, "information")
	got := buf.String()
	if !strings.Contains(got, "file=odd%2Cname%3A1.json") {
		t.Errorf("comma/colon not escaped in file property: %q", got)
	}
}

func TestGitHub_RespectsMinSeverity(t *testing.T) {
	results := []*validator.Result{{
		Filename: "a.json",
		Issues: []validator.Issue{
			{Severity: "information", Message: "info"},
			{Severity: "error", Message: "err"},
		},
	}}
	var buf bytes.Buffer
	_ = writeGitHub(&buf, results, "error")
	got := buf.String()
	if strings.Contains(got, "info") {
		t.Errorf("information issue must be filtered out at min severity error: %q", got)
	}
	if !strings.Contains(got, "err") {
		t.Errorf("error issue missing: %q", got)
	}
}

func TestGitHub_SuppressedIssuesAreNotAnnotated(t *testing.T) {
	results := []*validator.Result{{
		Filename:   "a.json",
		Issues:     []validator.Issue{{Severity: "error", Message: "kept"}},
		Suppressed: []validator.Issue{{Severity: "error", Message: "silenced"}},
	}}
	var buf bytes.Buffer
	_ = writeGitHub(&buf, results, "information")
	if strings.Contains(buf.String(), "silenced") {
		t.Errorf("suppressed issue must not produce an annotation: %q", buf.String())
	}
}

func TestGitHub_ValidFileProducesNoOutput(t *testing.T) {
	results := []*validator.Result{{Filename: "a.json", Valid: true}}
	var buf bytes.Buffer
	_ = writeGitHub(&buf, results, "information")
	if buf.Len() != 0 {
		t.Errorf("a file with no issues must produce no annotations, got %q", buf.String())
	}
}
