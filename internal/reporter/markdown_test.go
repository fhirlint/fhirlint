package reporter

import (
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

func TestBuildMarkdownReport_Summary(t *testing.T) {
	valid := makeResult(true)
	bad := makeResult(false,
		issue("error", "err msg", "Patient.gender"),
		issue("warning", "warn msg", "Patient"),
	)
	out := buildMarkdownReport([]*validator.Result{valid, bad}, "information")

	for _, want := range []string{
		"## FHIR Validation Report",
		"| Files | 2 |",
		"| ✅ Valid | 1 |",
		"| ❌ Errors | 1 |",
		"| ⚠️ Warnings | 1 |",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected report to contain %q\n---\n%s", want, out)
		}
	}
}

func TestBuildMarkdownReport_OmitsValidFiles(t *testing.T) {
	valid := makeResult(true)
	valid.Label = "clean.json"
	out := buildMarkdownReport([]*validator.Result{valid}, "information")

	if strings.Contains(out, "### ") {
		t.Errorf("valid file should not get a detail section\n---\n%s", out)
	}
	if strings.Contains(out, "clean.json") {
		t.Errorf("valid file should be omitted from detail\n---\n%s", out)
	}
}

func TestBuildMarkdownReport_FileHeadingEmoji(t *testing.T) {
	errFile := makeResult(false, issue("error", "boom", "Patient"))
	errFile.Label = "err.json"
	warnFile := makeResult(false, issue("warning", "meh", "Patient"))
	warnFile.Label = "warn.json"
	out := buildMarkdownReport([]*validator.Result{errFile, warnFile}, "information")

	if !strings.Contains(out, "### ❌ err.json") {
		t.Errorf("file with an error should use ❌\n---\n%s", out)
	}
	if !strings.Contains(out, "### ⚠️ warn.json") {
		t.Errorf("file with only warnings should use ⚠️\n---\n%s", out)
	}
}

func TestBuildMarkdownReport_SeverityFilter(t *testing.T) {
	r := makeResult(false,
		issue("error", "an error", "Patient.gender"),
		issue("warning", "a warning", "Patient"),
		issue("information", "an info", ""),
	)
	out := buildMarkdownReport([]*validator.Result{r}, "error")

	if !strings.Contains(out, "an error") {
		t.Error("error issue should be present when --severity error")
	}
	if strings.Contains(out, "a warning") || strings.Contains(out, "an info") {
		t.Errorf("below-threshold issues should be filtered out\n---\n%s", out)
	}
	if !strings.Contains(out, "| ⚠️ Warnings | 0 |") {
		t.Errorf("filtered warnings should not be counted in summary\n---\n%s", out)
	}
}

func TestBuildMarkdownReport_EscapesPipesAndNewlines(t *testing.T) {
	r := makeResult(false, issue("error", "value a|b\nsecond line", "Patient.code"))
	out := buildMarkdownReport([]*validator.Result{r}, "information")

	if !strings.Contains(out, `value a\|b second line`) {
		t.Errorf("pipes should be escaped and newlines flattened\n---\n%s", out)
	}
}

func TestBuildMarkdownReport_SuppressedDetails(t *testing.T) {
	r := makeResult(true)
	r.Suppressed = []validator.Issue{
		{Severity: "warning", Message: "suppressed thing", Location: "Patient", SuppressReason: "known false positive"},
	}
	out := buildMarkdownReport([]*validator.Result{r}, "information")

	for _, want := range []string{
		"<summary>Suppressed (1)</summary>",
		"suppressed thing",
		"known false positive",
		"</details>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected suppressed block to contain %q\n---\n%s", want, out)
		}
	}
}

func TestBuildMarkdownReport_NoSuppressedBlockWhenEmpty(t *testing.T) {
	r := makeResult(false, issue("error", "boom", "Patient"))
	out := buildMarkdownReport([]*validator.Result{r}, "information")
	if strings.Contains(out, "<details>") {
		t.Errorf("no <details> block expected when nothing is suppressed\n---\n%s", out)
	}
}

func TestBuildMarkdownReport_FatalCountsAsError(t *testing.T) {
	r := makeResult(false, issue("fatal", "fatal boom", "Patient"))
	r.Label = "fatal.json"
	out := buildMarkdownReport([]*validator.Result{r}, "information")
	if !strings.Contains(out, "| ❌ Errors | 1 |") {
		t.Errorf("fatal should count as an error\n---\n%s", out)
	}
	if !strings.Contains(out, "### ❌ fatal.json") {
		t.Errorf("fatal file should use ❌\n---\n%s", out)
	}
	if !strings.Contains(out, "| FATAL |") {
		t.Errorf("fatal severity label expected\n---\n%s", out)
	}
}

func TestBuildMarkdownReport_NamesTheReportedSeverity(t *testing.T) {
	downgraded := validator.Issue{
		Severity: "warning", OriginalSeverity: "error",
		Message: "bundle entry has no fullUrl", Location: "Bundle.entry[0]",
	}
	out := buildMarkdownReport([]*validator.Result{
		{Filename: "b.json", Label: "b.json", Issues: []validator.Issue{downgraded}},
	}, "information")

	if !strings.Contains(out, "WARNING (reported as ERROR)") {
		t.Errorf("expected the reported severity alongside the effective one, got:\n%s", out)
	}
}
