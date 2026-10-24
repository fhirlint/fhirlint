package reporter

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/diff"
	"github.com/fhirlint/fhirlint/internal/validator"
)

// hostile is what validator 6.10.3 echoes back for a gender code carrying an
// OSC 8 hyperlink and a screen clear (#496).
const hostile = "\x1b]8;;https://evil.example\x07CLICK\x1b]8;;\x07\x1b[2J"

// noRawControl fails when out holds a control character other than a line
// break. Tests run without a TTY, so lipgloss emits no colour codes of its own:
// any ESC left is one that came from the input.
func noRawControl(t *testing.T, sink, out string) {
	t.Helper()
	for _, c := range []string{"\x1b", "\x07"} {
		if strings.Contains(out, c) {
			t.Errorf("%s output contains a raw %q:\n%q", sink, c, out)
		}
	}
	if !strings.Contains(out, `\x1b]8;;https://evil.example\x07CLICK`) {
		t.Errorf("%s output does not show the escaped value:\n%q", sink, out)
	}
}

func hostileResult(t *testing.T) *validator.Result {
	path := writeFile(t, "{\n  \"gender\": \""+hostile+"\"\n}\n")
	issue := validator.Issue{
		Severity:  "error",
		Message:   "The value provided ('" + hostile + "') was not found",
		Location:  "Patient.gender" + hostile + " (line 2, col 13)",
		MessageID: "Terminology_TX_NoValid_16",
	}
	suppressed := issue
	suppressed.SuppressReason = "reason " + hostile
	return &validator.Result{
		Filename:   path,
		SourcePath: path,
		Label:      "dir/" + hostile + ".json",
		Issues:     []validator.Issue{issue},
		Suppressed: []validator.Issue{suppressed},
	}
}

func TestTerminal_EscapesControlCharacters(t *testing.T) {
	r := hostileResult(t)
	out := captureStdout(t, func() { Terminal(r, "information", true, false, true) })
	noRawControl(t, "terminal", out)
	if strings.Count(out, `CLICK`) < 5 {
		// label, headline, location, source line, suppressed headline, ...
		t.Errorf("expected the value in every field it was put in:\n%s", out)
	}
}

// The caret has to move right by however much the escapes before it widened
// the line, or it points into the middle of "\x1b".
func TestSourceSnippet_CaretAccountsForEscapes(t *testing.T) {
	path := writeFile(t, "{\n\"a\x1b\": \"bad\"\n}\n")
	got := sourceSnippet(path, 2, 7) // the opening quote of "bad"
	if len(got) != 2 {
		t.Fatalf("expected a line and a caret, got %q", got)
	}
	off, text := caretOffset(t, got[0], got[1])
	if text[off] != '"' || !strings.HasPrefix(string(text[off:]), `"bad"`) {
		t.Errorf("caret under %q, want it under the quote of \"bad\"\n%s\n%s", string(text[off:]), got[0], got[1])
	}
}

func TestDiffLine_EscapesControlCharacters(t *testing.T) {
	out := diffLine("+", diff.Issue{File: "x" + hostile, MessageID: "id", Location: "loc" + hostile})
	noRawControl(t, "diff", out)
}

func TestGitHub_EscapesControlCharacters(t *testing.T) {
	var buf bytes.Buffer
	_ = writeGitHub(&buf, []*validator.Result{hostileResult(t)}, "information")
	// The runner decodes %0A and friends; the ESC must not survive either way.
	noRawControl(t, "github", buf.String())
}

func TestMarkdown_EscapesControlCharacters(t *testing.T) {
	out := buildMarkdownReport([]*validator.Result{hostileResult(t)}, "information", RunInfo{})
	noRawControl(t, "markdown", out)
}
