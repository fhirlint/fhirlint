// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// corpus builds a two-dataset corpus: one with a canonical base, one without.
func corpus(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "ig", baseFile), "https://example.org/fhir/\n")
	write(t, filepath.Join(dir, "ig", "package", "package.json"), `{"name":"x","version":"1"}`)
	write(t, filepath.Join(dir, "ig", "package", "Patient-p1.json"), `{"resourceType":"Patient","id":"p1"}`)
	write(t, filepath.Join(dir, "ig", "package", "Observation-Obs1.json"),
		`{"resourceType":"Observation","id":"Obs1","subject":{"reference":"Patient/p1"},"performer":[{"reference":"Practitioner/gone"}]}`)
	write(t, filepath.Join(dir, "ig", "package", "ValueSet-vs.json"),
		`{"resourceType":"ValueSet","id":"vs","url":"http://other.org/ValueSet/vs"}`)
	write(t, filepath.Join(dir, "ig", "notes.txt"), "not json")
	write(t, filepath.Join(dir, "synthea", "a.json"),
		`{"resourceType":"ValueSet","id":"vs2","url":"http://other.org/ValueSet/vs2"}`)
	write(t, filepath.Join(dir, "README"), "a file next to the datasets is ignored")
	return dir
}

func scan(t *testing.T, dir string) []Finding {
	t.Helper()
	var stderr bytes.Buffer
	fs, err := scanCorpus(dir, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	return fs
}

func TestScanCorpus(t *testing.T) {
	fs := scan(t, corpus(t))
	var got []string
	for _, f := range fs {
		got = append(got, f.Dataset+"|"+f.File+"|"+f.Rule+"|"+f.Location)
	}
	want := []string{
		"ig|package/Observation-Obs1.json|lint:id-kebab-case|id",
		"ig|package/Observation-Obs1.json|ref:unresolved|Observation.performer[0].reference",
		"ig|package/ValueSet-vs.json|lint:canonical-url-pattern|url",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("findings:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// synthea has no base file, so canonical-url-pattern is skipped there,
	// and its references are checked against its own set only.
}

func TestScanCorpusWithoutDatasets(t *testing.T) {
	var stderr bytes.Buffer
	if _, err := scanCorpus(t.TempDir(), &stderr); err == nil || !strings.Contains(err.Error(), "corpus-fetch.sh") {
		t.Fatalf("want a hint to fetch the corpus, got %v", err)
	}
}

func TestScanWriteReadRoundTrip(t *testing.T) {
	dir := corpus(t)
	out := filepath.Join(t.TempDir(), "scan.jsonl")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"scan", "-o", out, dir}, &stdout, &stderr); code != 0 {
		t.Fatalf("scan exit %d: %s", code, stderr.String())
	}
	fs, err := readFindings(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 3 || fs[0].Severity != "warning" {
		t.Fatalf("round trip: %+v", fs)
	}
}

func TestDiffAndMustStay(t *testing.T) {
	tmp := t.TempDir()
	a := []Finding{
		{Dataset: "d", File: "x.json", Rule: "ref:unresolved", Location: "L1", Message: "m1"},
		{Dataset: "d", File: "y.json", Rule: "ref:unresolved", Location: "L2", Message: "m2"},
	}
	b := []Finding{
		{Dataset: "d", File: "y.json", Rule: "ref:unresolved", Location: "L2", Message: "m2"},
		{Dataset: "d", File: "z.json", Rule: "lint:id-kebab-case", Location: "id", Message: "m3"},
	}
	pa, pb := filepath.Join(tmp, "a.jsonl"), filepath.Join(tmp, "b.jsonl")
	for path, fs := range map[string][]Finding{pa: a, pb: b} {
		f, err := os.Create(path) //nolint:gosec // test temp file
		if err != nil {
			t.Fatal(err)
		}
		if err := writeFindings(f, fs); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
	}

	var out, errb bytes.Buffer
	if code := run([]string{"diff", pa, pb}, &out, &errb); code != 0 {
		t.Fatalf("diff exit %d: %s", code, errb.String())
	}
	for _, want := range []string{
		"lint:id-kebab-case: A 0, B 1, added 1, removed 0",
		"  + d/z.json id: m3",
		"ref:unresolved: A 2, B 1, added 0, removed 1",
		"  - d/x.json L1: m1",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("diff output lacks %q:\n%s", want, out.String())
		}
	}

	must := filepath.Join(tmp, "must.txt")
	write(t, must, "# why\nref:unresolved\td/y.json\tL2\n")
	out.Reset()
	if code := run([]string{"diff", "-must", must, pa, pb}, &out, &errb); code != 0 {
		t.Fatalf("must-stay entry present in B, want exit 0, got %d:\n%s", code, out.String())
	}
	write(t, must, "ref:unresolved\td/x.json\tL1\n")
	out.Reset()
	if code := run([]string{"diff", "-must", must, pa, pb}, &out, &errb); code != 1 || !strings.Contains(out.String(), "MISSING must-stay-reported: ref:unresolved d/x.json L1") {
		t.Fatalf("must-stay entry dropped by B, want exit 1 and a MISSING line, got %d:\n%s", code, out.String())
	}
}

func TestDiffLimitsOutput(t *testing.T) {
	var a, b []Finding
	for i := 0; i < 5; i++ {
		b = append(b, Finding{Dataset: "d", File: string(rune('a'+i)) + ".json", Rule: "r", Location: "id", Message: "m"})
	}
	var out bytes.Buffer
	printDiff(&out, a, b, 2)
	if !strings.Contains(out.String(), "  + … 3 more") {
		t.Fatalf("want the overflow line, got:\n%s", out.String())
	}
}

func TestReadMustListErrors(t *testing.T) {
	tmp := t.TempDir()
	bad := filepath.Join(tmp, "bad.txt")
	write(t, bad, "ref:unresolved d/x.json L1\n")
	if _, err := readMustList(bad); err == nil {
		t.Error("a line without tabs must be rejected")
	}
	empty := filepath.Join(tmp, "empty.txt")
	write(t, empty, "# only comments\n")
	if _, err := readMustList(empty); err == nil {
		t.Error("a list without entries must be rejected")
	}
}

func TestSampleOnePerResource(t *testing.T) {
	var fs []Finding
	for i := 0; i < 4; i++ {
		fs = append(fs, Finding{Dataset: "d", File: "same.json", Rule: "r", Location: string(rune('a' + i))})
	}
	fs = append(fs,
		Finding{Dataset: "d", File: "other.json", Rule: "r", Location: "x"},
		Finding{Dataset: "d", File: "third.json", Rule: "other-rule", Location: "x"},
	)
	got := sample(fs, "r", 20, 1)
	if len(got) != 2 {
		t.Fatalf("want one finding per resource of rule r, got %+v", got)
	}
	if again := sample(fs, "r", 1, 7); len(again) != 1 {
		t.Fatalf("want n to cap the sample, got %+v", again)
	}

	var out, errb bytes.Buffer
	scanFile := filepath.Join(t.TempDir(), "s.jsonl")
	f, err := os.Create(scanFile) //nolint:gosec // test temp file
	if err != nil {
		t.Fatal(err)
	}
	_ = writeFindings(f, fs)
	_ = f.Close()
	if code := run([]string{"sample", "-rule", "r", scanFile}, &out, &errb); code != 0 || strings.Count(out.String(), "\n") != 2 {
		t.Fatalf("sample exit %d, output:\n%s", code, out.String())
	}
}

func TestMask(t *testing.T) {
	if got := mask(`unresolved "Patient/a0df9d3c8e7f4b1a9c2d5e6f7a8b9c0d"`); got != `unresolved "Patient/a0df9d…"` {
		t.Errorf("mask = %q", got)
	}
	if got := mask("Patient/example-without-digits-but-long"); got != "Patient/example-without-digits-but-long" {
		t.Errorf("a long value without digits is kept, got %q", got)
	}
	if got := mask(strings.Repeat("x ", 100)); !strings.HasSuffix(got, "…") || len(got) > 170 {
		t.Errorf("long message not clipped: %q", got)
	}
}

func TestUsage(t *testing.T) {
	var out, errb bytes.Buffer
	for _, args := range [][]string{nil, {"bogus"}, {"scan"}, {"diff", "one"}, {"sample", "x"}} {
		errb.Reset()
		if code := run(args, &out, &errb); code != 2 || !strings.Contains(errb.String(), "usage:") {
			t.Errorf("run(%q) = %d, stderr %q", args, code, errb.String())
		}
	}
}
