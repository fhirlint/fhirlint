package cmd

import (
	"bufio"
	"strings"
	"testing"
)

func TestSplitCSV(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{"kbv.basis#1.5.0, mii#2024.0.0", []string{"kbv.basis#1.5.0", "mii#2024.0.0"}},
		{"single", []string{"single"}},
		{"", nil},
		{"  ,  , ", nil},
	}
	for _, tc := range cases {
		got := splitCSV(tc.input)
		if len(got) != len(tc.want) {
			t.Errorf("splitCSV(%q) = %v, want %v", tc.input, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("splitCSV(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
			}
		}
	}
}

func TestPrompt_UsesDefault(t *testing.T) {
	sc := bufio.NewScanner(strings.NewReader("\n"))
	got := prompt(sc, "Q", "default-value")
	if got != "default-value" {
		t.Errorf("expected default-value, got %q", got)
	}
}

func TestPrompt_UsesInput(t *testing.T) {
	sc := bufio.NewScanner(strings.NewReader("custom\n"))
	got := prompt(sc, "Q", "default-value")
	if got != "custom" {
		t.Errorf("expected custom, got %q", got)
	}
}

func TestPromptBool_YesVariants(t *testing.T) {
	for _, input := range []string{"y", "yes", "Y", "YES"} {
		sc := bufio.NewScanner(strings.NewReader(input + "\n"))
		if !promptBool(sc, "Q", false) {
			t.Errorf("expected true for input %q", input)
		}
	}
}

func TestPromptBool_NoVariants(t *testing.T) {
	for _, input := range []string{"n", "no", "N", "NO"} {
		sc := bufio.NewScanner(strings.NewReader(input + "\n"))
		if promptBool(sc, "Q", true) {
			t.Errorf("expected false for input %q", input)
		}
	}
}

func TestPromptBool_EmptyUsesDefault(t *testing.T) {
	sc := bufio.NewScanner(strings.NewReader("\n"))
	if !promptBool(sc, "Q", true) {
		t.Error("expected true (default) for empty input")
	}
}

func TestPromptChoice_ValidInput(t *testing.T) {
	sc := bufio.NewScanner(strings.NewReader("warning\n"))
	got := promptChoice(sc, "Q", "error", []string{"error", "warning", "information"})
	if got != "warning" {
		t.Errorf("expected warning, got %q", got)
	}
}

func TestPromptChoice_InvalidUsesDefault(t *testing.T) {
	sc := bufio.NewScanner(strings.NewReader("typo\n"))
	got := promptChoice(sc, "Q", "error", []string{"error", "warning"})
	if got != "error" {
		t.Errorf("expected error (default), got %q", got)
	}
}
