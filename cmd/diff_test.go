package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const reportWithError = `{"valid":false,"files":[{"filename":"a.json","valid":false,` +
	`"issues":[{"severity":"error","message":"bad","location":"A","messageId":"dom-4"}]}],` +
	`"summary":{"total":1,"errors":1,"warnings":0,"info":0}}`

const reportClean = `{"valid":true,"files":[{"filename":"a.json","valid":true,"issues":[]}],` +
	`"summary":{"total":0,"errors":0,"warnings":0,"info":0}}`

func writeReport(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func resetDiffFlags() {
	flagDiffFormat = []string{"terminal"}
	flagDiffOutput = ""
	flagDiffSeverity = "information"
	flagDiffShowUnchanged = false
}

func TestRunDiff_NewIssuesReturnValidationFailed(t *testing.T) {
	resetDiffFlags()
	base := writeReport(t, "base.json", reportClean)
	cur := writeReport(t, "cur.json", reportWithError)

	err := runDiff(nil, []string{base, cur})
	if !errors.Is(err, errValidationFailed) {
		t.Errorf("expected errValidationFailed (exit 1) when new issues found, got %v", err)
	}
}

func TestRunDiff_NoNewIssuesReturnsNil(t *testing.T) {
	resetDiffFlags()
	base := writeReport(t, "base.json", reportWithError)
	cur := writeReport(t, "cur.json", reportWithError)

	if err := runDiff(nil, []string{base, cur}); err != nil {
		t.Errorf("expected nil (exit 0) when no new issues, got %v", err)
	}
}

func TestRunDiff_ResolvedOnlyReturnsNil(t *testing.T) {
	resetDiffFlags()
	base := writeReport(t, "base.json", reportWithError)
	cur := writeReport(t, "cur.json", reportClean)

	if err := runDiff(nil, []string{base, cur}); err != nil {
		t.Errorf("expected nil (exit 0) when issues only resolved, got %v", err)
	}
}

func TestRunDiff_MalformedInputExitsTwo(t *testing.T) {
	resetDiffFlags()
	base := writeReport(t, "base.json", "not json")
	cur := writeReport(t, "cur.json", reportClean)

	err := runDiff(nil, []string{base, cur})
	var ee *exitErr
	if !errors.As(err, &ee) {
		t.Fatalf("expected *exitErr, got %T: %v", err, err)
	}
	if ee.code != 2 {
		t.Errorf("expected exit code 2 for malformed input, got %d", ee.code)
	}
}

func TestRunDiff_MissingFileExitsTwo(t *testing.T) {
	resetDiffFlags()
	cur := writeReport(t, "cur.json", reportClean)

	err := runDiff(nil, []string{"/no/such/report.json", cur})
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != 2 {
		t.Errorf("expected *exitErr with code 2 for missing file, got %v", err)
	}
}

func TestRunDiff_UnknownFormatExitsTwo(t *testing.T) {
	resetDiffFlags()
	flagDiffFormat = []string{"xml"}
	base := writeReport(t, "base.json", reportClean)
	cur := writeReport(t, "cur.json", reportClean)

	err := runDiff(nil, []string{base, cur})
	var ee *exitErr
	if !errors.As(err, &ee) || ee.code != 2 {
		t.Errorf("expected *exitErr with code 2 for unknown format, got %v", err)
	}
}

func TestDiffOutputFile(t *testing.T) {
	resetDiffFlags()

	flagDiffOutput = ""
	if got := diffOutputFile("json"); got != "" {
		t.Errorf("empty output should stay empty, got %q", got)
	}

	flagDiffOutput = "report.json"
	flagDiffFormat = []string{"json"}
	if got := diffOutputFile("json"); got != "report.json" {
		t.Errorf("single format keeps name as-is, got %q", got)
	}

	flagDiffFormat = []string{"json", "sarif"}
	if got := diffOutputFile("sarif"); got != "report.json.sarif" {
		t.Errorf("multiple formats append ext, got %q", got)
	}
}
