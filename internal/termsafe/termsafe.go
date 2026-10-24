// Package termsafe renders untrusted text for a terminal.
//
// Validator messages quote values from the input file verbatim, so a resource
// can carry ESC, BEL or C1 sequences that a terminal would act on: hidden
// hyperlinks, a cleared screen, a forged "✓ Valid" line. Text from input files,
// paths and the validator goes through here before it is printed (#496).
//
// Control characters are made visible rather than dropped. The messages quote
// the user's value, and '20\x1b[31m01' says what is in the file where
// '20[31m01' would mislead. Only the terminal-facing sinks use this; the JSON,
// SARIF, JUnit and HTML reporters keep the exact text, which their encoders
// escape on their own.
package termsafe

import (
	"fmt"
	"strings"
	"unicode"
)

// Line renders s for a single terminal line: every control character,
// newlines included, is replaced by its escape, and a tab by a space.
func Line(s string) string { return escape(s, false) }

// Text is Line for output that handles line breaks itself, such as GitHub
// workflow commands and Markdown cells: CR and LF pass through unchanged.
func Text(s string) string { return escape(s, true) }

func escape(s string, keepNewlines bool) string {
	if !strings.ContainsFunc(s, unicode.IsControl) {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\t':
			b.WriteByte(' ')
		case keepNewlines && (r == '\n' || r == '\r'):
			b.WriteRune(r)
		case !unicode.IsControl(r):
			b.WriteRune(r)
		case r < 0x80:
			fmt.Fprintf(&b, `\x%02x`, r)
		default:
			fmt.Fprintf(&b, `\u%04x`, r)
		}
	}
	return b.String()
}
