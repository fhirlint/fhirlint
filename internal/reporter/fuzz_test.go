package reporter

import (
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"
)

// FuzzSourceSnippet cuts a snippet out of a fuzzed file at a fuzzed line and
// column. Both numbers come from the validator and the text is the user's, so
// neither is guaranteed to fit the other: past the end, zero, negative or in
// the middle of a multi-byte rune.
func FuzzSourceSnippet(f *testing.F) {
	f.Add([]byte("{\n  \"resourceType\": \"Patient\"\n}\n"), 2, 5)
	f.Add([]byte("\t\"gender\": \"unknown-value\""), 1, 0)
	f.Add([]byte("äöü€😀 multi-byte line that is long enough to need truncating on both ends of the caret, more and more text"), 1, 70)
	f.Add([]byte("x"), 1, -3)
	f.Add([]byte(""), 1, 1)

	f.Fuzz(func(t *testing.T, data []byte, line, col int) {
		path := filepath.Join(t.TempDir(), "r.json")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		out := sourceSnippet(path, line, col)
		if len(out) > 2 {
			t.Errorf("a snippet is one line and at most one caret line, got %d", len(out))
		}
	})
}

// FuzzTruncateAround checks the windowing on its own: the result is never
// wider than the window plus its two ellipses, and a column inside the line
// still points inside the result, so the caret lands on the cut text. Columns
// outside the line are passed through; sourceSnippet draws no caret for them.
func FuzzTruncateAround(f *testing.F) {
	f.Add("short", 3)
	f.Add("a line well over the width of the window, so it has to be cut down to size around the column", 200)
	f.Add("😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀", -5)

	f.Fuzz(func(t *testing.T, text string, col int) {
		out, newCol := truncateAround(text, col, maxSnippetWidth)
		if n := utf8.RuneCountInString(out); n > maxSnippetWidth+2 {
			t.Errorf("result is %d runes wide, the window is %d", n, maxSnippetWidth)
		}
		if col >= 1 && col <= utf8.RuneCountInString(text) {
			if newCol < 1 || newCol > utf8.RuneCountInString(out) {
				t.Errorf("column %d of %q became %d, outside %q", col, text, newCol, out)
			}
		}
	})
}
