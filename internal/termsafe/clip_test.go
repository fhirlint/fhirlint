package termsafe

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

func TestMessage_ShortPassesThrough(t *testing.T) {
	for _, s := range []string{"", "Wert ungültig: äöü", strings.Repeat("x", MaxMessageRunes)} {
		if got := Message(s); got != s {
			t.Errorf("Message changed a %d-rune message", utf8.RuneCountInString(s))
		}
		if got := Clip(s); got != s {
			t.Errorf("Clip changed a %d-rune message", utf8.RuneCountInString(s))
		}
	}
}

// The measured case from #519: a value of 300,000 characters quoted three
// times. The start says what is wrong, the end where, and both must survive.
func TestMessage_LongIsCutInTheMiddle(t *testing.T) {
	value := strings.Repeat("ä", 300_000)
	msg := "The value '" + value + "' is not valid in '" + value + "' at Patient.gender"
	for name, got := range map[string]string{"Message": Message(msg), "Clip": Clip(msg)} {
		if !strings.HasPrefix(got, "The value '") || !strings.HasSuffix(got, "at Patient.gender") {
			t.Errorf("%s lost the start or the end", name)
		}
		omitted := utf8.RuneCountInString(msg) - MaxMessageRunes
		want := " … [" + groupThousands(omitted) + " characters omitted; full text with --format json] … "
		if !strings.Contains(got, want) {
			t.Errorf("%s: marker %q missing", name, want)
		}
		if n := utf8.RuneCountInString(got); n != MaxMessageRunes+utf8.RuneCountInString(want) {
			t.Errorf("%s: %d runes, want limit + marker", name, n)
		}
	}
}

func TestMessage_CutNeverSplitsAnEscape(t *testing.T) {
	msg := strings.Repeat("\x1b", MaxMessageRunes) // each renders as 4 runes
	got := Message(msg)
	head, tail, ok := strings.Cut(got, " … [")
	if !ok {
		t.Fatal("no marker")
	}
	_, tail, _ = strings.Cut(tail, "] … ")
	for _, part := range []string{head, tail} {
		if strings.Trim(part, `\x1b`) != "" || len(part)%4 != 0 {
			t.Fatalf("broken escape in %q", part)
		}
	}
	if strings.ContainsFunc(got, unicode.IsControl) {
		t.Error("control character left")
	}
}

func TestMessageText_KeepsNewlines(t *testing.T) {
	if got := MessageText("a\n\x1bb"); got != "a\n\\x1bb" {
		t.Errorf("MessageText = %q", got)
	}
}

func TestGroupThousands(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 12345: "12,345", 900442: "900,442", 1234567: "1,234,567"} {
		if got := groupThousands(n); got != want {
			t.Errorf("groupThousands(%d) = %q, want %q", n, got, want)
		}
	}
}

// FuzzClip runs the cut with a small limit, so that short fuzz inputs reach it.
// The output stays within limit + marker, keeps no control character, and the
// part before and after the marker is the rendering of a whole prefix and a
// whole suffix of the input: no escape and no rune is split.
func FuzzClip(f *testing.F) {
	for _, s := range []string{"\x1b]8;;u\x07x\x1b]8;;\x07", "a\tb\nc äöü", "\u009b31m", "\xff\xfe", strings.Repeat("ab\x1b", 9)} {
		f.Add(s, uint8(5), false)
	}
	f.Fuzz(func(t *testing.T, s string, lim uint8, keepNewlines bool) {
		// Messages arrive decoded from the validator's JSON, so valid UTF-8.
		s = strings.ToValidUTF8(s, "�")
		limit := int(lim%40) + 1
		rendered := escape(s, keepNewlines)
		got := clipMessage(s, limit, true, keepNewlines)

		if got == rendered {
			if utf8.RuneCountInString(got) > limit {
				t.Fatalf("not cut: %d runes > %d", utf8.RuneCountInString(got), limit)
			}
			return
		}
		i := strings.Index(got, " … [")
		j := strings.LastIndex(got, "] … ")
		if i < 0 || j < i {
			t.Fatalf("cut without marker: %q", got)
		}
		head, tail := got[:i], got[j+len("] … "):]
		if n := utf8.RuneCountInString(head) + utf8.RuneCountInString(tail); n > limit {
			t.Fatalf("kept %d runes, limit %d", n, limit)
		}
		if !isRenderedPrefix(s, head, keepNewlines) {
			t.Fatalf("head %q is not the rendering of a prefix of %q", head, s)
		}
		if !isRenderedSuffix(s, tail, keepNewlines) {
			t.Fatalf("tail %q is not the rendering of a suffix of %q", tail, s)
		}
		for _, r := range got {
			if unicode.IsControl(r) && (!keepNewlines || r != '\n' && r != '\r') {
				t.Fatalf("control character %U left", r)
			}
		}
	})
}

func isRenderedPrefix(s, head string, keepNewlines bool) bool {
	for i := range len(s) + 1 {
		if (i == len(s) || utf8.RuneStart(s[i])) && escape(s[:i], keepNewlines) == head {
			return true
		}
	}
	return false
}

func isRenderedSuffix(s, tail string, keepNewlines bool) bool {
	for i := range len(s) + 1 {
		if (i == len(s) || utf8.RuneStart(s[i])) && escape(s[i:], keepNewlines) == tail {
			return true
		}
	}
	return false
}
