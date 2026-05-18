package ndjson_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fhirlint/fhirlint/internal/ndjson"
)

func writeNDJSON(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "export-Patient.ndjson")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestIsNDJSON(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"export.ndjson", true},
		{"export.NDJSON", true},
		{"patient.json", false},
		{"bundle.xml", false},
		{"export.ndjson.bak", false},
	}
	for _, tc := range cases {
		if got := ndjson.IsNDJSON(tc.path); got != tc.want {
			t.Errorf("IsNDJSON(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestSplit_BasicLines(t *testing.T) {
	path := writeNDJSON(t, `{"resourceType":"Patient","id":"1"}
{"resourceType":"Patient","id":"2"}
{"resourceType":"Patient","id":"3"}
`)
	ins, err := ndjson.Split(path)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	defer func() {
		for _, in := range ins {
			in.Cleanup()
		}
	}()

	if len(ins) != 3 {
		t.Fatalf("expected 3 inputs, got %d", len(ins))
	}
}

func TestSplit_SkipsEmptyLines(t *testing.T) {
	path := writeNDJSON(t, `{"resourceType":"Patient","id":"1"}

{"resourceType":"Patient","id":"2"}

`)
	ins, err := ndjson.Split(path)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	defer func() {
		for _, in := range ins {
			in.Cleanup()
		}
	}()
	if len(ins) != 2 {
		t.Fatalf("expected 2 inputs (empty lines skipped), got %d", len(ins))
	}
}

func TestSplit_Labels(t *testing.T) {
	path := writeNDJSON(t, `{"resourceType":"Patient","id":"1"}
{"resourceType":"Patient","id":"2"}
`)
	ins, err := ndjson.Split(path)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	defer func() {
		for _, in := range ins {
			in.Cleanup()
		}
	}()

	if ins[0].Label != "export-Patient.ndjson:1" {
		t.Errorf("expected label export-Patient.ndjson:1, got %q", ins[0].Label)
	}
	if ins[1].Label != "export-Patient.ndjson:2" {
		t.Errorf("expected label export-Patient.ndjson:2, got %q", ins[1].Label)
	}
}

func TestSplit_LabelUsesLineNumber(t *testing.T) {
	// Line 1 is empty, first resource is on line 2.
	path := writeNDJSON(t, `
{"resourceType":"Patient","id":"1"}
`)
	ins, err := ndjson.Split(path)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	defer func() {
		for _, in := range ins {
			in.Cleanup()
		}
	}()
	if len(ins) != 1 {
		t.Fatalf("expected 1 input, got %d", len(ins))
	}
	if ins[0].Label != "export-Patient.ndjson:2" {
		t.Errorf("expected label :2 (actual line number), got %q", ins[0].Label)
	}
}

func TestSplit_TempFilesContainJSON(t *testing.T) {
	path := writeNDJSON(t, `{"resourceType":"Patient","id":"x"}
`)
	ins, err := ndjson.Split(path)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	defer func() {
		for _, in := range ins {
			in.Cleanup()
		}
	}()

	data, err := os.ReadFile(ins[0].Path)
	if err != nil {
		t.Fatalf("reading temp file: %v", err)
	}
	if string(data) != `{"resourceType":"Patient","id":"x"}` {
		t.Errorf("unexpected temp file content: %q", string(data))
	}
}

func TestSplit_EmptyFile(t *testing.T) {
	path := writeNDJSON(t, "")
	ins, err := ndjson.Split(path)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(ins) != 0 {
		t.Errorf("expected 0 inputs for empty file, got %d", len(ins))
	}
}

func TestSplit_MissingFile(t *testing.T) {
	_, err := ndjson.Split("/nonexistent/export.ndjson")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestSplit_TempFilesHaveJSONExtension(t *testing.T) {
	path := writeNDJSON(t, `{"resourceType":"Patient"}
`)
	ins, err := ndjson.Split(path)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	defer func() {
		for _, in := range ins {
			in.Cleanup()
		}
	}()
	if filepath.Ext(ins[0].Path) != ".json" {
		t.Errorf("expected .json temp file, got %q", ins[0].Path)
	}
}
