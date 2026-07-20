package reporter

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// maxSnippetWidth is how much of a line is shown before it is truncated around
// the column of interest. Serialised FHIR resources are frequently minified
// onto one enormous line, which would otherwise wrap into unreadable noise.
const maxSnippetWidth = 100

// sourceSnippet renders the source line an issue points at, with a caret under
// the column:
//
//	  5 │   "birthDate": "not-a-date"
//	    │                 ^
//
// It returns an empty slice whenever the line cannot be shown faithfully —
// no path, unreadable file, or a line number past the end of the file. Showing
// the wrong line is worse than showing none, so every uncertain case declines.
func sourceSnippet(path string, line, col int) []string {
	if path == "" || line <= 0 {
		return nil
	}
	text, ok := readLine(path, line)
	if !ok {
		return nil
	}

	// Tabs would misalign the caret, since one tab is one column to the
	// validator but several on screen.
	text = strings.ReplaceAll(text, "\t", " ")

	text, col = truncateAround(text, col, maxSnippetWidth)

	gutter := strconv.Itoa(line)
	pad := strings.Repeat(" ", len(gutter))
	out := []string{"  " + gutter + " │ " + text}
	// col is 1-based; col 0 means the validator gave no column.
	if col > 0 && col <= len(text)+1 {
		out = append(out, "  "+pad+" │ "+strings.Repeat(" ", col-1)+"^")
	}
	return out
}

// readLine returns the 1-based line n of the file, if it has that many lines.
func readLine(path string, n int) (string, bool) {
	f, err := os.Open(path) //nolint:gosec // path comes from the validated input set
	if err != nil {
		return "", false
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	// Minified resources can put the whole document on one line, well past the
	// scanner's default 64 KB limit.
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for i := 1; sc.Scan(); i++ {
		if i == n {
			return sc.Text(), true
		}
	}
	return "", false
}

// truncateAround narrows text to at most width runes centred on col, returning
// the new text and the column adjusted to match. Elisions are marked with "…".
func truncateAround(text string, col, width int) (string, int) {
	r := []rune(text)
	if len(r) <= width {
		return text, col
	}
	start := 0
	if col > width/2 {
		start = col - width/2
	}
	if start+width > len(r) {
		start = len(r) - width
	}
	if start < 0 {
		start = 0
	}
	out := string(r[start : start+width])
	newCol := col - start
	if start > 0 {
		out = "…" + out
		newCol++ // the ellipsis occupies a column
	}
	if start+width < len(r) {
		out += "…"
	}
	if newCol < 0 {
		newCol = 0
	}
	return out, newCol
}
