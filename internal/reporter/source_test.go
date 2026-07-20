package reporter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "resource.json")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

// caretOffset returns how far the caret sits into the rendered text, counting
// runes. Byte offsets would be wrong here: the gutter uses "│" and truncated
// lines contain "…", both multi-byte.
func caretOffset(t *testing.T, lineRow, caretRow string) (int, []rune) {
	t.Helper()
	split := func(s string) []rune {
		r := []rune(s)
		for i, c := range r {
			if c == '│' {
				return r[i+2:] // skip "│" and the following space
			}
		}
		t.Fatalf("no gutter separator in %q", s)
		return nil
	}
	text := split(lineRow)
	caret := split(caretRow)
	for i, c := range caret {
		if c == '^' {
			return i, text
		}
	}
	t.Fatalf("no caret in %q", caretRow)
	return 0, nil
}

func TestSourceSnippet_RendersLineAndCaret(t *testing.T) {
	path := writeFile(t, "{\n  \"resourceType\": \"Patient\",\n  \"gender\": \"bad\"\n}\n")

	got := sourceSnippet(path, 3, 13)
	if len(got) != 2 {
		t.Fatalf("expected a line and a caret, got %d: %q", len(got), got)
	}
	if !strings.Contains(got[0], `"gender": "bad"`) {
		t.Errorf("wrong line rendered: %q", got[0])
	}
	if !strings.Contains(got[0], "3 │") {
		t.Errorf("line number missing from gutter: %q", got[0])
	}
	// The caret must sit under column 13 of the rendered text.
	off, _ := caretOffset(t, got[0], got[1])
	if off != 12 {
		t.Errorf("caret at rune offset %d, want 12\n%s\n%s", off, got[0], got[1])
	}
}

func TestSourceSnippet_DeclinesWhenItCannotBeFaithful(t *testing.T) {
	path := writeFile(t, "{\n  \"a\": 1\n}\n")

	cases := []struct {
		name string
		path string
		line int
		col  int
	}{
		{"no path", "", 1, 1},
		{"line zero", path, 0, 1},
		{"negative line", path, -3, 1},
		{"line past end of file", path, 99, 1},
		{"file does not exist", filepath.Join(t.TempDir(), "missing.json"), 1, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sourceSnippet(tc.path, tc.line, tc.col); got != nil {
				t.Errorf("expected no snippet, got %q", got)
			}
		})
	}
}

func TestSourceSnippet_NoColumnMeansNoCaret(t *testing.T) {
	path := writeFile(t, "{\n  \"a\": 1\n}\n")
	got := sourceSnippet(path, 2, 0)
	if len(got) != 1 {
		t.Fatalf("expected only the source line, got %d: %q", len(got), got)
	}
}

func TestSourceSnippet_TruncatesLongLine(t *testing.T) {
	// A minified resource puts everything on one line; the caret must survive
	// truncation and still point at the right character.
	long := `{"resourceType":"Patient","filler":"` + strings.Repeat("x", 500) + `","gender":"bad"}`
	path := writeFile(t, long)

	col := strings.Index(long, `"bad"`) + 1
	got := sourceSnippet(path, 1, col)
	if len(got) != 2 {
		t.Fatalf("expected line and caret, got %q", got)
	}
	if len([]rune(got[0])) > maxSnippetWidth+20 {
		t.Errorf("line not truncated: %d runes", len([]rune(got[0])))
	}
	if !strings.Contains(got[0], "…") {
		t.Errorf("truncation should be marked with an ellipsis: %q", got[0])
	}
	// The caret should land on the '"' that starts "bad".
	off, line := caretOffset(t, got[0], got[1])
	if off < 0 || off >= len(line) {
		t.Fatalf("caret offset %d outside the rendered line", off)
	}
	if line[off] != '"' {
		t.Errorf("caret points at %q, want the quote starting \"bad\"\n%s\n%s",
			string(line[off]), got[0], got[1])
	}
}

func TestSourceSnippet_TabsBecomeSpacesSoCaretAligns(t *testing.T) {
	path := writeFile(t, "{\n\t\"gender\": \"bad\"\n}\n")
	got := sourceSnippet(path, 2, 2)
	if len(got) != 2 {
		t.Fatalf("expected line and caret, got %q", got)
	}
	if strings.Contains(got[0], "\t") {
		t.Errorf("tab survived into the rendered line: %q", got[0])
	}
}

func TestTruncateAround(t *testing.T) {
	text := strings.Repeat("a", 50) + "TARGET" + strings.Repeat("b", 50)
	col := 51 // 1-based, the 'T'

	out, newCol := truncateAround(text, col, 20)
	if len([]rune(out)) > 22 { // width plus up to two ellipses
		t.Errorf("result too long: %d runes (%q)", len([]rune(out)), out)
	}
	r := []rune(out)
	if newCol < 1 || newCol > len(r) {
		t.Fatalf("adjusted column %d outside %q", newCol, out)
	}
	if r[newCol-1] != 'T' {
		t.Errorf("adjusted column points at %q, want 'T' (%q)", string(r[newCol-1]), out)
	}
}

func TestTruncateAround_ShortLineUnchanged(t *testing.T) {
	out, col := truncateAround("short line", 4, 100)
	if out != "short line" || col != 4 {
		t.Errorf("short line should pass through unchanged, got %q col %d", out, col)
	}
}
