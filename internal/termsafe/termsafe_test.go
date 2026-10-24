package termsafe

import (
	"testing"
	"unicode"
)

func TestLine(t *testing.T) {
	cases := map[string]string{
		"plain text, äöü ✓":            "plain text, äöü ✓",
		"20\x1b[31m01-99-99":           `20\x1b[31m01-99-99`,
		"\x1b]8;;https://e.x\x07CLICK": `\x1b]8;;https://e.x\x07CLICK`,
		"a\tb":                         "a b",
		"line\n  ✓ Valid":              `line\x0a  ✓ Valid`,
		"cr\roverwrite":                `cr\x0doverwrite`,
		"del\x7f":                      `del\x7f`,
		"c1 csi \u009b2J":              `c1 csi \u009b2J`,
		"":                             "",
	}
	for in, want := range cases {
		if got := Line(in); got != want {
			t.Errorf("Line(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestText(t *testing.T) {
	if got, want := Text("a\x1b[2J\r\nb\tc"), "a\\x1b[2J\r\nb c"; got != want {
		t.Errorf("Text = %q, want %q", got, want)
	}
}

// Whatever goes in, nothing a terminal acts on may come out.
func FuzzLine(f *testing.F) {
	for _, s := range []string{"\x1b]8;;u\x07x\x1b]8;;\x07", "a\tb\nc", "\u009b31m", "\xff\xfe"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		for _, r := range Line(s) {
			if unicode.IsControl(r) {
				t.Fatalf("Line(%q) still contains %U", s, r)
			}
		}
		for _, r := range Text(s) {
			if unicode.IsControl(r) && r != '\n' && r != '\r' {
				t.Fatalf("Text(%q) still contains %U", s, r)
			}
		}
	})
}
