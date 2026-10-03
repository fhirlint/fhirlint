package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWarnSkippedIGFiles(t *testing.T) {
	dir := t.TempDir()
	skipped := filepath.Join(dir, "StructureDefinition-patient-template.json")
	for _, p := range []string{skipped, filepath.Join(dir, "StructureDefinition-patient.json")} {
		if err := os.WriteFile(p, []byte(`{}`), 0600); err != nil {
			t.Fatal(err)
		}
	}

	var buf bytes.Buffer
	warnSkippedIGFiles(&buf, []string{dir, "kbv.basis#1.9.0"})

	out := buf.String()
	if strings.Count(out, "warning:") != 1 || !strings.Contains(out, skipped) {
		t.Errorf("want one warning naming %s, got:\n%s", skipped, out)
	}
}

func TestWarnSkippedIGFiles_SilentForPackages(t *testing.T) {
	var buf bytes.Buffer
	warnSkippedIGFiles(&buf, []string{"kbv.basis#1.9.0", "de.basisprofil.r4"})
	if buf.Len() != 0 {
		t.Errorf("want no output for package ids, got %q", buf.String())
	}
}
