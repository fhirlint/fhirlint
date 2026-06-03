package validator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// messagesHTML wraps a Messages-table body in the surrounding markup the JAR
// emits, so tests exercise the same extraction the real output needs.
func messagesHTML(rows string) string {
	return `<html><body>` +
		`<h3>Messages</h3>` +
		`<div><table class="grid">` +
		`<tr><th>Severity</th><th>Path</th><th>Description</th></tr>` +
		rows +
		`</table></div>` +
		`<h3>Metadata</h3><table class="grid"><tr><td>abstract</td><td>false</td></tr></table>` +
		`</body></html>`
}

func TestParseCompareMessages(t *testing.T) {
	rows := `<tr style="background-color: #ffcccc"><td>Error</td><td>StructureDefinition.version</td><td>Values for version differ: &#39;1.0.0&#39; vs &#39;2.0.0&#39;</td></tr>` +
		`<tr style="background-color: #ffffe6"><td>Information</td><td>Patient.name</td><td>Element minimum cardinalities differ:  &#39;0&#39; vs &#39;1&#39;</td></tr>`
	msgs := parseCompareMessages(messagesHTML(rows))

	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2: %+v", len(msgs), msgs)
	}
	if msgs[0].Severity != "error" || msgs[0].Path != "StructureDefinition.version" {
		t.Errorf("message 0 = %+v", msgs[0])
	}
	if !strings.Contains(msgs[0].Message, "'1.0.0' vs '2.0.0'") {
		t.Errorf("message 0 text not unescaped: %q", msgs[0].Message)
	}
	if msgs[1].Severity != "information" || msgs[1].Path != "Patient.name" {
		t.Errorf("message 1 = %+v", msgs[1])
	}
	// The Metadata table must not leak into the messages.
	for _, m := range msgs {
		if m.Path == "abstract" {
			t.Error("parsed a row from the Metadata table")
		}
	}
}

func TestParseCompareMessages_NoMessagesSection(t *testing.T) {
	msgs := parseCompareMessages(`<html><body><h3>Metadata</h3><table class="grid"></table></body></html>`)
	if len(msgs) != 0 {
		t.Errorf("got %d messages, want 0", len(msgs))
	}
	if msgs == nil {
		t.Error("want a non-nil empty slice for clean JSON marshalling")
	}
}

func TestParseCompareMessages_EmptyTable(t *testing.T) {
	msgs := parseCompareMessages(messagesHTML(""))
	if len(msgs) != 0 {
		t.Errorf("got %d messages, want 0 (header row only): %+v", len(msgs), msgs)
	}
}

func TestFindComparisonHTML(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"sd+1-aaa-bbb.html",
		"sd+1-aaa-bbb-union.html",
		"sd+1-aaa-bbb-intersection.html",
		"index.html",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	got := findComparisonHTML(dir)
	if filepath.Base(got) != "sd+1-aaa-bbb.html" {
		t.Errorf("got %q, want the main comparison file (not union/intersection)", got)
	}
}

func TestFindComparisonHTML_None(t *testing.T) {
	if got := findComparisonHTML(t.TempDir()); got != "" {
		t.Errorf("got %q, want empty when no comparison file exists", got)
	}
}

func TestCompareResult_Differs(t *testing.T) {
	empty := &CompareResult{Messages: []CompareMessage{}}
	if empty.Differs() {
		t.Error("Differs() = true for empty messages, want false")
	}
	some := &CompareResult{Messages: []CompareMessage{{Severity: "error"}}}
	if !some.Differs() {
		t.Error("Differs() = false with messages, want true")
	}
}

func TestRunCompare_Guards(t *testing.T) {
	if _, err := RunCompare("", "right", CompareOptions{FHIRVersion: "4.0.1", DestDir: t.TempDir()}); err == nil {
		t.Error("expected error for empty left profile")
	}
	if _, err := RunCompare("a", "b", CompareOptions{FHIRVersion: "9.9.9", DestDir: t.TempDir()}); err == nil || !strings.Contains(err.Error(), "unknown FHIR version") {
		t.Errorf("err = %v, want unknown-version error", err)
	}
	if _, err := RunCompare("a", "b", CompareOptions{FHIRVersion: "4.0.1", DestDir: ""}); err == nil {
		t.Error("expected error for empty destination directory")
	}
}

func TestCompareFailure(t *testing.T) {
	err := compareFailure("Loading\nUnable to resolve profile http://x\nDone.", nil)
	if err == nil || !strings.Contains(err.Error(), "Unable to resolve profile") {
		t.Errorf("err = %v, want the resolve failure surfaced", err)
	}
}
