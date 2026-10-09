package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestWarnBaselineIDDrift(t *testing.T) {
	var buf bytes.Buffer
	warnBaselineIDDrift(&buf, 0)
	if buf.Len() != 0 {
		t.Errorf("no drift, but warned: %q", buf.String())
	}

	warnBaselineIDDrift(&buf, 3)
	out := buf.String()
	for _, want := range []string{"3 finding(s)", "message id", "--generate-baseline", "--validator-version"} {
		if !strings.Contains(out, want) {
			t.Errorf("warning %q does not mention %q", out, want)
		}
	}
	if strings.Count(out, "\n") != 1 {
		t.Errorf("want a single line, got %q", out)
	}
}
