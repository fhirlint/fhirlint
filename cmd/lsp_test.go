package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddSuppression_CreatesBlockInAnEmptyConfig(t *testing.T) {
	got, err := addSuppression("", "dom-6")
	if err != nil {
		t.Fatal(err)
	}
	if got != "suppress:\n  - messageId:dom-6\n" {
		t.Errorf("unexpected content:\n%q", got)
	}
}

func TestAddSuppression_ExtendsAnExistingBlock(t *testing.T) {
	in := "severity: warning\n\nsuppress:\n  - messageId:dom-6\n"

	got, err := addSuppression(in, "ref:external")
	if err != nil {
		t.Fatal(err)
	}
	// A second suppress: key would make the document invalid, so the existing
	// one has to be extended in place.
	if strings.Count(got, "suppress:") != 1 {
		t.Errorf("expected exactly one suppress key:\n%s", got)
	}
	if !strings.Contains(got, "messageId:ref:external") {
		t.Errorf("new rule missing:\n%s", got)
	}
	if !strings.Contains(got, "messageId:dom-6") {
		t.Errorf("existing rule lost:\n%s", got)
	}
}

func TestAddSuppression_PreservesComments(t *testing.T) {
	in := "# our house rules\nseverity: warning  # keep this\n\nsuppress:\n  # agreed with the team\n  - messageId:dom-6\n"

	got, err := addSuppression(in, "ref:external")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# our house rules", "# keep this", "# agreed with the team"} {
		if !strings.Contains(got, want) {
			t.Errorf("comment %q was lost:\n%s", want, got)
		}
	}
}

func TestAddSuppression_IsIdempotent(t *testing.T) {
	in := "suppress:\n  - messageId:dom-6\n"

	got, err := addSuppression(in, "dom-6")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("an existing rule should be left alone, got:\n%s", got)
	}
}

func TestAddSuppression_IgnoresCommentedOutExamples(t *testing.T) {
	// fhirlint.yml.example style: the rule is present but commented out, so it
	// is not actually in effect and must still be added.
	in := "suppress:\n  # - messageId:dom-6\n"

	got, err := addSuppression(in, "dom-6")
	if err != nil {
		t.Fatal(err)
	}
	if got == "" {
		t.Fatal("a commented-out rule is not an active rule; it should have been added")
	}
	if !strings.Contains(got, "  - messageId:dom-6") {
		t.Errorf("active rule missing:\n%s", got)
	}
}

func TestAddSuppression_RejectsEmptyID(t *testing.T) {
	if _, err := addSuppression("", "  "); err == nil {
		t.Error("an empty message id should be rejected")
	}
}

func TestAddSuppression_SeparatesFromExistingContent(t *testing.T) {
	got, err := addSuppression("severity: warning", "dom-6")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "severity: warning\n") {
		t.Errorf("existing content should be kept verbatim:\n%q", got)
	}
	if !strings.Contains(got, "\nsuppress:\n  - messageId:dom-6\n") {
		t.Errorf("suppress block malformed:\n%q", got)
	}
}

func TestConfigSuppressor_WritesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fhirlint.yml")
	s := &configSuppressor{path: path}

	if err := s.Suppress("dom-6"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path) //nolint:gosec // test-controlled path
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "messageId:dom-6") {
		t.Errorf("suppression not written:\n%s", data)
	}

	// Applying the same suppression twice must not duplicate it.
	if err := s.Suppress("dom-6"); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path) //nolint:gosec // test-controlled path
	if got := strings.Count(string(data), "messageId:dom-6"); got != 1 {
		t.Errorf("expected the rule once, got %d times:\n%s", got, data)
	}
}
