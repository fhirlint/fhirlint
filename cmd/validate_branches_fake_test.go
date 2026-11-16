// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/txreplay"
)

// The branches of runValidate no test reached before (#689): the start of a
// replay, the input dispatch, the post-processing and the reports, each
// asserting what the user sees.

// writeRecording writes a recording by hand: a manifest and one entry for
// GET /metadata, enough for a replay to start.
func writeRecording(t *testing.T, dir string, m txreplay.Manifest) {
	t.Helper()
	store, err := txreplay.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	in := &txreplay.Interaction{Method: "GET", Path: "/metadata", Status: 200, Response: []byte(`{"resourceType":"CapabilityStatement"}`)}
	if err := store.Put(in, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteManifest(m); err != nil {
		t.Fatal(err)
	}
}

func TestFakeValidate_ReplayStartupBranches(t *testing.T) {
	t.Run("a corrupt recording is an error", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "p.json", cleanPatient)
		writeFile(t, dir, ".fhirlint-tx/broken.json", "{not json")
		r := runFake(t, dir, "validate", "--tx-offline", "p.json")
		if r.err == nil || !strings.Contains(r.err.Error(), "is corrupt") {
			t.Errorf("got %v", r.err)
		}
	})

	t.Run("a registry recording routes the validator through the replay", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "p.json", cleanPatient)
		writeRecording(t, filepath.Join(dir, ".fhirlint-tx"), txreplay.Manifest{Upstream: "https://tx.fhir.org/r4", Routing: txreplay.RoutingRegistry, Ecosystem: txreplay.DefaultEcosystem})
		prepareFake(t)
		log := filepath.Join(t.TempDir(), "calls.jsonl")
		t.Setenv(fakeJavaLogEnv, log)
		mustPass(t, runFakeKeepEnv(t, dir, "validate", "--tx-offline", "p.json"))
		calls := fakeJavaCalls(t, log)
		last := calls[len(calls)-1]
		eco, tx := argAfter(last, "-ecosystem"), argAfter(last, "-tx")
		if len(eco) == 0 || !strings.HasSuffix(eco[0], "/tx-reg") || len(tx) == 0 || !strings.HasSuffix(tx[0], "/r4") {
			t.Errorf("the validator must ask the replayed registry, and the replay under the recorded path: %v", last)
		}
	})

	t.Run("expansion parameters that differ from the recording's", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "p.json", cleanPatient)
		writeFile(t, dir, "exp.json", `{"resourceType":"Parameters","id":"other"}`)
		writeRecording(t, filepath.Join(dir, ".fhirlint-tx"), txreplay.Manifest{Upstream: "https://tx.example.org", ExpansionParameters: "0123456789ab"})
		r := runFake(t, dir, "validate", "--tx-offline", "--expansion-parameters", "exp.json", "p.json")
		if !strings.Contains(r.stderr, "recording was made with different --expansion-parameters") {
			t.Errorf("want the drift warning:\n%s", r.stderr)
		}
		r = runFakeKeepEnv(t, dir, "validate", "--tx-offline", "--expansion-parameters", "missing.json", "p.json")
		if r.err == nil || !strings.Contains(r.err.Error(), "missing.json") {
			t.Errorf("an expansion parameters file that can't be read: %v", r.err)
		}
	})
}

func TestFakeValidate_TerminologyStartupErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "p.json", cleanPatient)

	prepareFake(t)
	t.Setenv("FHIRLINT_TX_TOKEN", "s3cret")
	r := runFakeKeepEnv(t, dir, "validate", "--terminology-server", "http://tx.example.org/r4", "p.json")
	if r.err == nil || !strings.Contains(r.err.Error(), "over plain HTTP") {
		t.Errorf("credentials must not go over plain HTTP: %v", r.err)
	}
	t.Setenv("FHIRLINT_TX_TOKEN", "")

	r = runFakeKeepEnv(t, dir, "validate", "--terminology-server", "http://tx.example.org/r4", "--allow-insecure-tx", "--validator-arg=-fhir-settings", "--validator-arg=x.json", "p.json")
	if r.err == nil || !strings.Contains(r.err.Error(), "fhir-settings") {
		t.Errorf("the insecure exemption needs its own settings file: %v", r.err)
	}

	r = runFakeKeepEnv(t, dir, "validate", "--offline", "--ig", "not.cached#1.0.0", "p.json")
	if r.err == nil || !strings.Contains(r.err.Error(), "not.cached") {
		t.Errorf("--offline with an IG that isn't cached must say which: %v", r.err)
	}
}

func TestFakeValidate_ServerIgnoresVersionAndIGs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "p.json", cleanPatient)
	// No server listens: the note comes before the run fails to reach it.
	r := runFake(t, dir, "validate", "--server", "http://127.0.0.1:1", "--ig", "example.ig#1.0.0", "p.json")
	if !strings.Contains(r.stderr, "--fhir-version/--ig are ignored") {
		t.Errorf("want the note that --ig doesn't reach a running server:\n%s", r.stderr)
	}
}

// gitRepo makes dir a git repository with p.json committed.
func gitRepo(t *testing.T, dir string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	writeFile(t, dir, "p.json", cleanPatient)
	for _, args := range [][]string{
		{"init", "-q"}, {"add", "p.json"}, {"commit", "-q", "-m", "init"},
	} {
		c := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.org", "-c", "commit.gpgsign=false"}, args...)...) //nolint:gosec // git with the test's own fixed arguments
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func TestFakeValidate_SinceOnAFileAndOnNothingChanged(t *testing.T) {
	dir := t.TempDir()
	gitRepo(t, dir)

	// Nothing changed since HEAD: the run says so instead of a silent pass.
	r := runFake(t, dir, "validate", "--since", "HEAD", ".")
	if r.err != nil || !strings.Contains(r.stderr, "No changed files to validate (--since HEAD)") {
		t.Errorf("got %v\n%s", r.err, r.stderr)
	}

	// A changed file named directly: the repository is found from its
	// directory, and the change is validated.
	writeFile(t, dir, "p.json", errPatient)
	r = runFakeKeepEnv(t, dir, "validate", "--since", "HEAD", filepath.Join(dir, "p.json"))
	mustFail(t, r)
	if !strings.Contains(r.stdout, "Fake error found") {
		t.Errorf("the changed file must be validated:\n%s", r.stdout)
	}
}

func TestFakeValidate_InputDispatchErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "p.json", cleanPatient)
	writeFile(t, dir, "sub/q.json", cleanPatient)
	writeFile(t, dir, "map.map", "map \"http://example.org/m\" = \"m\"\n")
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--watch=single", "missing.json"}, "missing.json"},
		{[]string{"--url", "https://example.org/Patient/1", "p.json"}, "cannot combine a file argument with --url"},
		{[]string{"p.json", "missing.json"}, "missing.json"},
		{[]string{"--extract-each", "$.items", "sub"}, "--extract-each cannot be used with a directory"},
		{[]string{"--extract-each", "$.items", "map.map"}, "--extract-each cannot be used with FHIR Mapping Language input"},
		{[]string{"--extract", "$.data", "map.map"}, "--extract cannot be used with FHIR Mapping Language input"},
	} {
		r := runFake(t, dir, append([]string{"validate"}, tc.args...)...)
		if r.err == nil || !strings.Contains(r.err.Error(), tc.want) {
			t.Errorf("%q: want %q, got %v", tc.args, tc.want, r.err)
		}
	}
}

// --skip-non-fhir on one file that is not FHIR: nothing is validated and the
// run passes, instead of handing the validator a file it would reject.
func TestFakeValidate_SkipNonFHIROnASingleFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name":"not-fhir","version":"1.0.0"}`)
	log := filepath.Join(t.TempDir(), "calls.jsonl")
	prepareFake(t)
	t.Setenv(fakeJavaLogEnv, log)
	mustPass(t, runFakeKeepEnv(t, dir, "validate", "--skip-non-fhir", "package.json"))
	if _, err := os.Stat(log); err == nil && len(fakeJavaCalls(t, log)) > 0 {
		t.Error("a skipped file must not reach the validator")
	}
}

func TestFakeValidate_PostProcessingErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "p.json", cleanPatient)
	writeFile(t, dir, "bad-baseline.json", "{not json")
	for _, tc := range []struct {
		name, yaml string
		args       []string
		want       string
	}{
		{"a rule that doesn't compile", "rules:\n  - {id: r1, assert: 'name.('}\n", nil, "r1"},
		{"a severity override with a bad severity", "severity-override:\n  - {messageId: X, severity: loud}\n", nil, "loud"},
		{"a baseline that isn't JSON", "", []string{"--baseline", "bad-baseline.json"}, "reading baseline bad-baseline.json"},
		{"a baseline that can't be written", "", []string{"--generate-baseline", filepath.Join("no", "such", "dir", "b.json")}, "writing baseline"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writeFile(t, dir, "fhirlint.yml", tc.yaml+"\n")
			r := runFake(t, dir, append(append([]string{"validate"}, tc.args...), "p.json")...)
			if r.err == nil || !strings.Contains(r.err.Error(), tc.want) {
				t.Errorf("want %q, got %v", tc.want, r.err)
			}
		})
	}
}

// A report that can't be written is an error, for every format with a file.
func TestFakeValidate_ReportWriteErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "p.json", cleanPatient)
	out := filepath.Join("no", "such", "dir", "report")
	for _, format := range []string{"json", "html", "junit", "sarif"} {
		r := runFake(t, dir, "validate", "--format", format, "--output", out, "p.json")
		if r.err == nil {
			t.Errorf("--format %s into a missing directory must fail", format)
		}
	}
}

// --package-baseline: a file from an installed package whose own QA build
// found errors in it too doesn't fail the run (#402).
func TestFakeValidate_PackageBaseline(t *testing.T) {
	prepareFake(t)
	pkg := filepath.Join(os.Getenv("HOME"), ".fhir", "packages", "demo.ig#1.0.0", "package")
	file := writeFile(t, pkg, "Patient-p2.json", errPatient)
	writeFile(t, pkg, "other/validation-summary.json", `{"Patient/p2":{"errors":1,"warnings":0}}`)
	dir := t.TempDir()

	r := runFakeKeepEnv(t, dir, "validate", "--no-color", file)
	mustFail(t, r)
	if !strings.Contains(r.stdout, "Package baseline: 1 resource(s) checked against demo.ig#1.0.0") || !strings.Contains(r.stdout, "agrees with the published build (1 expected failure(s))") {
		t.Errorf("want the package baseline summary:\n%s", r.stdout)
	}

	// The same from fhirlint.yml, with no-color too: the expected failure no
	// longer fails the run.
	writeFile(t, dir, "fhirlint.yml", "package-baseline: true\nno-color: true\n")
	r = runFakeKeepEnv(t, dir, "validate", file)
	if r.err != nil {
		t.Errorf("an expected failure must not fail the run under package-baseline: %v\n%s", r.err, r.stdout)
	}
	if strings.Contains(r.stdout, "\x1b[") {
		t.Errorf("no-color from the config must leave the output plain:\n%q", r.stdout)
	}
}
