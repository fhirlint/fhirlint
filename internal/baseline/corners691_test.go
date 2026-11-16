// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package baseline_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/baseline"
	"github.com/fhirlint/fhirlint/internal/validator"
)

// The corners of the baseline file (#691).

// An absolute path is stored relative to the working directory, so a
// baseline written on one machine applies on another; entries are ordered by
// file, then location, then message id, so the file diffs cleanly.
func TestGenerate_RelativePathsAndOrder(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	abs := filepath.Join(cwd, "examples", "p.json")
	bf := baseline.Generate([]*validator.Result{
		{Filename: "b.json", Issues: []validator.Issue{{MessageID: "Z", Location: "Patient.a"}}},
		{Filename: abs, Issues: []validator.Issue{
			{MessageID: "B", Location: "Patient.name"},
			{MessageID: "A", Location: "Patient.name"},
			{MessageID: "C", Location: "Patient.birthDate"},
		}},
	})
	var got []string
	for _, e := range bf.Entries {
		got = append(got, e.File+"|"+e.Location+"|"+e.MessageID)
	}
	want := "b.json|Patient.a|Z examples/p.json|Patient.birthDate|C examples/p.json|Patient.name|A examples/p.json|Patient.name|B"
	if strings.Join(got, " ") != want {
		t.Errorf("entries:\n got %v\nwant %s", got, want)
	}
}

func TestReadWrite_Failures(t *testing.T) {
	dir := t.TempDir()
	if _, err := baseline.Read(dir); err == nil {
		t.Error("reading a directory must fail")
	}
	bad := filepath.Join(dir, "b.json")
	if err := os.WriteFile(bad, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := baseline.Read(bad); err == nil || !strings.Contains(err.Error(), "parsing") {
		t.Errorf("a corrupt baseline: %v", err)
	}
	if err := baseline.Write(filepath.Join(dir, "missing", "b.json"), &baseline.BaselineFile{}); err == nil {
		t.Error("writing below a missing directory must fail")
	}
}
