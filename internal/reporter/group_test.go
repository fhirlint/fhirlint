package reporter

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

// captureStdout runs fn with stdout redirected and returns what it printed.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// resultWith builds a result whose issues all carry the same message ID.
func resultWith(label string, issues ...validator.Issue) *validator.Result {
	return &validator.Result{Filename: label, Label: label, SourcePath: label, Issues: issues}
}

func narrativeIssue() validator.Issue {
	return validator.Issue{
		Severity:  "warning",
		MessageID: "dom-6",
		Message:   "A resource should have narrative for robust management",
		Location:  "Patient (line 1, col 2)",
	}
}

func TestGroupFindings_CollapsesIdenticalFindings(t *testing.T) {
	var results []*validator.Result
	for i := 1; i <= 5; i++ {
		results = append(results, resultWith(fmt.Sprintf("p%d.json", i), narrativeIssue()))
	}

	groups := groupFindings(results, func(r *validator.Result) []validator.Issue { return r.Issues })

	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1: %+v", len(groups), groups)
	}
	if groups[0].occurrences != 5 || groups[0].files != 5 {
		t.Errorf("occurrences=%d files=%d, want 5 and 5", groups[0].occurrences, groups[0].files)
	}
	if got := groups[0].countLabel(); got != "5 files" {
		t.Errorf("countLabel = %q, want %q", got, "5 files")
	}
}

func TestGroupFindings_SeparatesDifferentMessages(t *testing.T) {
	other := narrativeIssue()
	other.Message = "A totally different problem"
	results := []*validator.Result{resultWith("a.json", narrativeIssue(), other)}

	groups := groupFindings(results, func(r *validator.Result) []validator.Issue { return r.Issues })

	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2 — same ID but different text is a different problem", len(groups))
	}
}

func TestGroupFindings_SeparatesDifferentSeverities(t *testing.T) {
	// A severity-override or a per-file override can leave the same message at
	// two levels in one run; merging them would report a level that is only
	// true for some occurrences.
	downgraded := narrativeIssue()
	downgraded.Severity = "information"
	results := []*validator.Result{
		resultWith("a.json", narrativeIssue()),
		resultWith("b.json", downgraded),
	}

	groups := groupFindings(results, func(r *validator.Result) []validator.Issue { return r.Issues })

	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}
}

func TestGroupFindings_NormalisesWhitespace(t *testing.T) {
	wrapped := narrativeIssue()
	wrapped.Message = "A resource should have\n   narrative for robust management"
	results := []*validator.Result{
		resultWith("a.json", narrativeIssue()),
		resultWith("b.json", wrapped),
	}

	groups := groupFindings(results, func(r *validator.Result) []validator.Issue { return r.Issues })

	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1 — wrapping is not a different finding", len(groups))
	}
}

func TestGroupFindings_CountsOccurrencesAndFilesSeparately(t *testing.T) {
	// One bundle failing the same constraint three times is three occurrences
	// in one file — the summary counts three, so the group must say three.
	iss := narrativeIssue()
	results := []*validator.Result{
		resultWith("bundle.json", iss, iss, iss),
		resultWith("other.json", iss),
	}

	groups := groupFindings(results, func(r *validator.Result) []validator.Issue { return r.Issues })

	if groups[0].occurrences != 4 || groups[0].files != 2 {
		t.Fatalf("occurrences=%d files=%d, want 4 and 2", groups[0].occurrences, groups[0].files)
	}
	if got := groups[0].countLabel(); got != "4 occurrences in 2 files" {
		t.Errorf("countLabel = %q", got)
	}
}

func TestGroupFindings_ExampleCap(t *testing.T) {
	var results []*validator.Result
	for i := 1; i <= 483; i++ {
		results = append(results, resultWith(fmt.Sprintf("p%d.json", i), narrativeIssue()))
	}

	groups := groupFindings(results, func(r *validator.Result) []validator.Issue { return r.Issues })

	if len(groups[0].examples) != maxGroupExamples {
		t.Fatalf("got %d examples, want the cap of %d", len(groups[0].examples), maxGroupExamples)
	}
	line := groups[0].exampleLine()
	if !strings.Contains(line, "… and 480 more") {
		t.Errorf("example line should account for the ones left out, got: %q", line)
	}
	if !strings.HasPrefix(line, "p1.json:1, p2.json:1, p3.json:1") {
		t.Errorf("examples should be the first files seen, got: %q", line)
	}
}

func TestGroupFindings_ExampleCarriesLineNumber(t *testing.T) {
	withLine := validator.Issue{Severity: "error", MessageID: "Rule_bdl_1",
		Message: "Bundle entry missing fullUrl", Location: "Bundle.entry[0] (line 14, col 3)"}
	noLine := validator.Issue{Severity: "error", MessageID: "X", Message: "no location at all"}

	groups := groupFindings([]*validator.Result{resultWith("b.json", withLine, noLine)},
		func(r *validator.Result) []validator.Issue { return r.Issues })

	var got []string
	for _, g := range groups {
		got = append(got, g.exampleLine())
	}
	joined := strings.Join(got, " | ")
	if !strings.Contains(joined, "b.json:14") {
		t.Errorf("expected a line number on the example, got: %s", joined)
	}
	if !strings.Contains(joined, "b.json |") && !strings.HasSuffix(joined, "b.json") {
		t.Errorf("expected a bare path where the validator reported no line, got: %s", joined)
	}
}

func TestGroupFindings_OrdersBySeverityThenFrequency(t *testing.T) {
	warn := narrativeIssue()
	err1 := validator.Issue{Severity: "error", MessageID: "E1", Message: "first error"}
	err2 := validator.Issue{Severity: "error", MessageID: "E2", Message: "second error"}
	fatal := validator.Issue{Severity: "fatal", MessageID: "F", Message: "fatal thing"}

	results := []*validator.Result{
		resultWith("a.json", warn, err1, err2, err2),
		resultWith("b.json", warn, warn, fatal),
	}

	groups := groupFindings(results, func(r *validator.Result) []validator.Issue { return r.Issues })

	var ids []string
	for _, g := range groups {
		ids = append(ids, g.messageID)
	}
	want := []string{"F", "E2", "E1", "dom-6"}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("order = %v, want %v (most severe first, then most frequent)", ids, want)
		}
	}
}

func TestGroupFindings_CountsMatchUngrouped(t *testing.T) {
	results := []*validator.Result{
		resultWith("a.json", narrativeIssue(), narrativeIssue(),
			validator.Issue{Severity: "error", MessageID: "E", Message: "boom"}),
		resultWith("b.json", narrativeIssue()),
	}

	var ungrouped int
	for _, r := range results {
		ungrouped += len(filterIssues(r.Issues, "information"))
	}
	var grouped int
	for _, g := range groupFindings(results, func(r *validator.Result) []validator.Issue {
		return filterIssues(r.Issues, "information")
	}) {
		grouped += g.occurrences
	}

	if grouped != ungrouped {
		t.Errorf("grouped total %d != ungrouped total %d", grouped, ungrouped)
	}
}

func TestTerminalGrouped_HonoursSeverityFilter(t *testing.T) {
	results := []*validator.Result{
		resultWith("a.json", narrativeIssue(),
			validator.Issue{Severity: "error", MessageID: "E", Message: "boom"}),
	}

	out := captureStdout(t, func() {
		TerminalGrouped(results, "error", false, false)
	})

	if strings.Contains(out, "dom-6") {
		t.Errorf("--severity error should hide the warning group, got:\n%s", out)
	}
	if !strings.Contains(out, "boom") {
		t.Errorf("expected the error group, got:\n%s", out)
	}
}

func TestTerminalGrouped_RendersHeaderMessageAndExamples(t *testing.T) {
	var results []*validator.Result
	for i := 1; i <= 4; i++ {
		results = append(results, resultWith(fmt.Sprintf("p%d.json", i), narrativeIssue()))
	}

	out := captureStdout(t, func() {
		TerminalGrouped(results, "information", false, false)
	})

	for _, want := range []string{
		"dom-6", "4 files",
		"A resource should have narrative for robust management",
		"p1.json:1", "… and 1 more",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestTerminalGrouped_NoIssues(t *testing.T) {
	out := captureStdout(t, func() {
		TerminalGrouped([]*validator.Result{{Filename: "ok.json", Label: "ok.json", Valid: true}},
			"information", false, false)
	})

	if !strings.Contains(out, "No issues") {
		t.Errorf("expected a clean-run line, got: %q", out)
	}
	if strings.Contains(out, "ok.json") {
		t.Errorf("grouped output should not list clean files, got: %q", out)
	}
}

func TestTerminalGrouped_ShowSuppressed(t *testing.T) {
	supp := narrativeIssue()
	supp.SuppressReason = "accepted"
	results := []*validator.Result{
		{Filename: "a.json", Label: "a.json", Suppressed: []validator.Issue{supp}},
		{Filename: "b.json", Label: "b.json", Suppressed: []validator.Issue{supp}},
	}

	hidden := captureStdout(t, func() { TerminalGrouped(results, "information", false, false) })
	if strings.Contains(hidden, "dom-6") {
		t.Errorf("suppressed findings must stay hidden without --show-suppressed:\n%s", hidden)
	}

	shown := captureStdout(t, func() { TerminalGrouped(results, "information", true, false) })
	if !strings.Contains(shown, "SUPP") || !strings.Contains(shown, "2 files") {
		t.Errorf("expected suppressed findings grouped too, got:\n%s", shown)
	}
}

func TestTerminalGrouped_ShowsReLevelledSeverity(t *testing.T) {
	downgraded := narrativeIssue()
	downgraded.OriginalSeverity = "error"
	out := captureStdout(t, func() {
		TerminalGrouped([]*validator.Result{resultWith("a.json", downgraded)}, "information", false, false)
	})

	if !strings.Contains(out, "reported as error") {
		t.Errorf("grouped output should keep the re-levelling visible, got:\n%s", out)
	}
}

func TestGroupHeadline_ShortensConstraintURIs(t *testing.T) {
	cases := map[string]struct{ id, message, want string }{
		"constraint URI": {
			"http://hl7.org/fhir/StructureDefinition/DomainResource#dom-6",
			"narrative missing", "dom-6",
		},
		"plain id":   {"Rule_bdl_1", "fullUrl missing", "Rule_bdl_1"},
		"no id":      {"", "something the validator did not label", "something the validator did not label"},
		"empty frag": {"http://example.org/sd#", "trailing hash", "http://example.org/sd#"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			g := findingGroup{groupKey: groupKey{messageID: tc.id}, displayMessage: tc.message}
			if got := g.headline(); got != tc.want {
				t.Errorf("headline = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGroupFindings_FullIDStillSeparatesGroups(t *testing.T) {
	// Two constraints from different StructureDefinitions can share a fragment;
	// the header shortens, the grouping key does not.
	a := validator.Issue{Severity: "warning", MessageID: "http://example.org/A#dom-6", Message: "one"}
	b := validator.Issue{Severity: "warning", MessageID: "http://example.org/B#dom-6", Message: "two"}

	groups := groupFindings([]*validator.Result{resultWith("a.json", a, b)},
		func(r *validator.Result) []validator.Issue { return r.Issues })

	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}
}
