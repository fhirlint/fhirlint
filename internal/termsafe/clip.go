package termsafe

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxMessageRunes caps an issue message in the human-facing reports (#519).
//
// The validator quotes the offending value verbatim, sometimes three times: a
// 300 KB value gave a 900 KB message, and every annotation, table cell and
// JUnit attribute repeated it. Real messages stay far below the limit: across
// 291 messages from the kbv.basis 1.10.0 examples under 6.10.3 and 7.0.3, the
// longest had 681 characters.
const MaxMessageRunes = 2000

// Message is Line for an issue message: control characters are made visible,
// then a message longer than MaxMessageRunes is cut in the middle, so the start
// (what is wrong) and the end (where) both stay. The cut never lands inside an
// escape.
func Message(s string) string { return clipMessage(s, MaxMessageRunes, true, false) }

// MessageText is Message for output that handles line breaks itself, like Text.
func MessageText(s string) string { return clipMessage(s, MaxMessageRunes, true, true) }

// Clip caps an issue message without escaping it, for the sinks whose encoder
// escapes on its own: JUnit, HTML, SARIF, CodeClimate and the LSP. The JSON
// reporter keeps the full text and is what the marker points at.
func Clip(s string) string { return clipMessage(s, MaxMessageRunes, false, false) }

func clipMessage(s string, limit int, escapeControls, keepNewlines bool) string {
	if escapeControls {
		if !strings.ContainsFunc(s, unicode.IsControl) {
			escapeControls = false
		}
	}
	if !escapeControls {
		if utf8.RuneCountInString(s) <= limit {
			return s
		}
		return clipRunes(s, limit)
	}
	// One token per input rune, holding what that rune renders as. Cutting
	// between tokens keeps every escape whole, and the budget counts what is
	// printed, not what was in the file.
	var toks []string
	total := 0
	for _, r := range s {
		t := escape(string(r), keepNewlines)
		toks = append(toks, t)
		total += utf8.RuneCountInString(t)
	}
	if total <= limit {
		return strings.Join(toks, "")
	}
	headBudget := limit / 2
	tailBudget := limit - headBudget
	head, used := 0, 0
	for head < len(toks) {
		n := utf8.RuneCountInString(toks[head])
		if used+n > headBudget {
			break
		}
		used += n
		head++
	}
	tail, used := len(toks), 0
	for tail > head {
		n := utf8.RuneCountInString(toks[tail-1])
		if used+n > tailBudget {
			break
		}
		used += n
		tail--
	}
	return strings.Join(toks[:head], "") + marker(tail-head) + strings.Join(toks[tail:], "")
}

// clipRunes cuts a string that needs no escaping, keeping the first and last
// limit/2 runes.
func clipRunes(s string, limit int) string {
	r := []rune(s)
	head := limit / 2
	tail := limit - head
	return string(r[:head]) + marker(len(r)-head-tail) + string(r[len(r)-tail:])
}

func marker(omitted int) string {
	return " … [" + groupThousands(omitted) + " characters omitted; full text with --format json] … "
}

// groupThousands formats n as 12,345: the counts run into the hundreds of
// thousands, where a bare number is hard to read.
func groupThousands(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	first := len(s) % 3
	if first > 0 {
		b.WriteString(s[:first])
	}
	for i := first; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}
