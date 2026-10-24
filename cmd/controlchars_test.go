package cmd

import (
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

// fhirpath prints values from the resource, compare prints canonicals, paths
// and messages from third-party profiles. None of it may reach the terminal
// with a raw control character (#496).
func TestTerminalOutput_EscapesControlCharacters(t *testing.T) {
	const bad = "\x1b]8;;u\x07x\x1b[2J"
	outputs := map[string]string{
		"fhirpath one": captureStdout(t, func() {
			printFHIRPathTerminal(&validator.FHIRPathResult{Items: []string{bad}})
		}),
		"fhirpath many": captureStdout(t, func() {
			printFHIRPathTerminal(&validator.FHIRPathResult{Items: []string{bad, bad}})
		}),
		"compare": captureStdout(t, func() {
			printCompareTerminal(&validator.CompareResult{
				Left: "l" + bad, Right: "r" + bad,
				Messages: []validator.CompareMessage{{Severity: "error", Path: "p" + bad, Message: "m" + bad}},
			})
		}),
	}
	for name, out := range outputs {
		if strings.Contains(out, "\x1b") || strings.Contains(out, "\x07") {
			t.Errorf("%s: raw control character in output:\n%q", name, out)
		}
		if !strings.Contains(out, `\x1b]8;;u\x07x\x1b[2J`) {
			t.Errorf("%s: escaped value missing:\n%s", name, out)
		}
	}
}
