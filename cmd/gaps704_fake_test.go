// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// suppress export without -o writes the advisor file to stdout and keeps the
// summary on stderr, so `suppress export > advisor.json` is a clean file. The
// summary names every rule it could not carry over, by kind (#704).
func TestFakeSuppressExportStdoutAndSummary(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "fhirlint.yml", `suppress:
  - messageId:GLOBAL_ID
  - constraint:dom-6
overrides:
  - files: ["examples/**"]
    suppress:
      - messageId:SCOPED_ID
severity-override:
  - messageId: SOME_ID
    severity: warning
`)
	r := runFake(t, dir, "suppress", "export")
	mustPass(t, r)
	var file struct {
		Suppress []string `json:"suppress"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &file); err != nil || len(file.Suppress) != 1 || file.Suppress[0] != "GLOBAL_ID" {
		t.Errorf("stdout must be the advisor file with only the global rule: %v\n%s", err, r.stdout)
	}
	for _, want := range []string{
		"Exported 1 of 2 suppression rule(s) as 1 advisor entry.",
		"Not exported — no advisor equivalent:",
		"constraint:dom-6",
		"1 rule(s) under `overrides:`",
		"1 `severity-override:` rule(s)",
	} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("stderr must contain %q:\n%s", want, r.stderr)
		}
	}
	if strings.Contains(r.stdout, "Exported") {
		t.Errorf("the summary must not end up in the advisor file:\n%s", r.stdout)
	}

	// --strict counts the scoped and re-levelled rules too, not only the
	// dropped one: all three are missing from the file.
	r = runFakeKeepEnv(t, dir, "suppress", "export", "--strict", "-o", filepath.Join(dir, "advisor.json"))
	if r.err == nil || !strings.Contains(r.err.Error(), "3 rule(s) could not be exported") {
		t.Errorf("--strict: %v", r.err)
	}
}

// Every config error stops the export with the reason, and an output path that
// cannot be written names the path (#704).
func TestFakeSuppressExportErrors(t *testing.T) {
	cases := []struct {
		name, config, want string
		out                bool
	}{
		{"suppress not a list", "suppress: messageId:X\n", "suppress config must be a list", false},
		{"suppress rule of the wrong type", "suppress:\n  - 42\n", "suppress rule must be a string or map", false},
		{"override with a bad rule", "overrides:\n  - files: [\"a/**\"]\n    suppress: nope\n", "overrides: suppress config must be a list", false},
		{"severity-override not a list", "severity-override: warning\n", "severity-override config must be a list", false},
		{"unwritable output", "suppress:\n  - messageId:X\n", "writing ", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "fhirlint.yml", c.config)
			args := []string{"suppress", "export"}
			if c.out {
				args = append(args, "-o", filepath.Join(dir, "missing", "advisor.json"))
			}
			r := runFake(t, dir, args...)
			if r.err == nil || !strings.Contains(r.err.Error(), c.want) {
				t.Errorf("err = %v, want %q", r.err, c.want)
			}
		})
	}
}

// writePackage puts a package straight into the cache under HOME, for the
// cases cachePackage does not cover: no FHIR version, a payload of a given size.
func writePackage(t *testing.T, name, version string, fhirVersions []string, payload int) {
	t.Helper()
	dir := filepath.Join(os.Getenv("HOME"), ".fhir", "packages", name+"#"+version, "package")
	m := map[string]any{"name": name, "version": version}
	if fhirVersions != nil {
		m["fhirVersions"] = fhirVersions
	}
	b, _ := json.Marshal(m)
	writeFile(t, dir, "package.json", string(b))
	writeFile(t, dir, "payload.bin", strings.Repeat("x", payload))
}

// The listing: a package under two spellings says so and names each spelling,
// a package without a FHIR version shows a dash, --sort size puts the largest
// first, and an unknown --sort is refused (#704).
func TestFakePackagesListing(t *testing.T) {
	dir := t.TempDir()
	mustPass(t, runFake(t, dir, "packages"))
	writePackage(t, "KBV.Basis", "1.1.3", []string{"4.0.1"}, 10)
	writePackage(t, "kbv.basis", "1.5.0", []string{"4.0.1"}, 10)
	writePackage(t, "small.pkg", "1.0.0", nil, 10)
	writePackage(t, "zz.large", "1.0.0", []string{"4.0.1"}, 3<<10)

	r := runFakeKeepEnv(t, dir, "packages")
	mustPass(t, r)
	if !strings.Contains(r.stdout, "(installed under more than one spelling)") {
		t.Errorf("two spellings of one package must be flagged:\n%s", r.stdout)
	}
	for _, spelling := range []string{"KBV.Basis", "kbv.basis"} {
		found := false
		for _, line := range strings.Split(r.stdout, "\n") {
			if strings.HasPrefix(line, "  ") && strings.HasSuffix(line, "  "+spelling) {
				found = true
			}
		}
		if !found {
			t.Errorf("each version row must name its spelling %s:\n%s", spelling, r.stdout)
		}
	}
	if !strings.Contains(r.stdout, "FHIR —") {
		t.Errorf("a package without a FHIR version must show a dash:\n%s", r.stdout)
	}
	if strings.Contains(r.stdout, "small.pkg  (installed") {
		t.Errorf("a single-spelling package must not be flagged:\n%s", r.stdout)
	}
	if !strings.Contains(r.stdout, "3 KB") {
		t.Errorf("sizes from 1 KB on are shown in KB:\n%s", r.stdout)
	}

	r = runFakeKeepEnv(t, dir, "packages", "--sort", "size")
	mustPass(t, r)
	if i, j := strings.Index(r.stdout, "zz.large"), strings.Index(r.stdout, "small.pkg"); i < 0 || j < 0 || i > j {
		t.Errorf("--sort size must put the largest package first:\n%s", r.stdout)
	}

	r = runFakeKeepEnv(t, dir, "packages", "--sort", "date")
	if r.err == nil || !strings.Contains(r.err.Error(), `unknown --sort "date": use name or size`) {
		t.Errorf("unknown --sort: %v", r.err)
	}
}

func TestHumanBytes(t *testing.T) {
	for n, want := range map[int64]string{
		0:                 "0 B",
		1023:              "1023 B",
		1 << 10:           "1 KB",
		(1 << 20) - 1:     "1023 KB",
		1 << 20:           "1 MB",
		5<<20 + 1:         "5 MB",
		1 << 30:           "1.0 GB",
		3<<30 + 1<<29:     "3.5 GB",
		int64(2048) << 30: "2048.0 GB",
	} {
		if got := humanBytes(n); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", n, got, want)
		}
	}
}
