// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package validator

import (
	"archive/zip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// RunWatch and the rest of RunMultiple against the fake validator (#686).

// captureStdout runs fn with os.Stdout going to a pipe and returns what it wrote.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	_ = w.Close()
	os.Stdout = old
	return <-done
}

// loggedArgs returns the arguments of every call the fake logged.
func loggedArgs(t *testing.T, log string) [][]string {
	t.Helper()
	data, err := os.ReadFile(log) //nolint:gosec // test file
	if err != nil {
		t.Fatal(err)
	}
	var calls [][]string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var call []string
		if err := json.Unmarshal([]byte(line), &call); err != nil {
			t.Fatal(err)
		}
		calls = append(calls, call)
	}
	return calls
}

func TestFakeRunWatch_PassesTheWatchSettings(t *testing.T) {
	opts, dir := fakeSetup(t)
	log := filepath.Join(t.TempDir(), "calls.jsonl")
	t.Setenv("FAKEJAVA_LOG", log)
	p := put(t, dir, "p.json", `{"resourceType":"Patient","note":"FAKE_ERROR"}`)

	var err error
	out := captureStdout(t, func() {
		err = RunWatch([]string{p}, opts, WatchConfig{Mode: "all", ScanDelayMS: 250, SettleTimeMS: 100})
	})
	if err != nil {
		t.Fatalf("RunWatch: %v", err)
	}
	if !strings.Contains(out, "Watching for changes (mode all, scan delay 250, settle time 100)") {
		t.Errorf("the validator's watch output goes to stdout as it is:\n%s", out)
	}
	call := loggedArgs(t, log)[0]
	// By base name: on Windows the logged path can carry the 8.3 short form of
	// the temp directory (RUNNER~1) where p has the long one.
	hasInput := slices.ContainsFunc(call, func(a string) bool { return filepath.Base(a) == filepath.Base(p) })
	if i := slices.Index(call, "-watch-mode"); i < 0 || call[i+1] != "all" || !hasInput {
		t.Errorf("the JAR must get -watch-mode all and the input: %v", call)
	}

	// Without delays the JAR's own defaults stand.
	_ = os.Remove(log)
	_ = captureStdout(t, func() { err = RunWatch([]string{p}, opts, WatchConfig{Mode: "single"}) })
	call = loggedArgs(t, log)[0]
	if err != nil || slices.Contains(call, "-watch-scan-delay") || slices.Contains(call, "-watch-settle-time") {
		t.Errorf("zero delays must not be passed: %v %v", err, call)
	}
}

func TestFakeRunWatch_RefusesBadOptionsAndReportsAFailedJAR(t *testing.T) {
	opts, dir := fakeSetup(t)
	p := put(t, dir, "p.json", `{"resourceType":"Patient"}`)
	for name, o := range map[string]Options{
		"FHIR version":  {FHIRVersion: "9.9.9", JARPath: opts.JARPath},
		"extra arg":     {FHIRVersion: "4.0.1", JARPath: opts.JARPath, ExtraArgs: []string{"-output"}},
		"best practice": {FHIRVersion: "4.0.1", JARPath: opts.JARPath, BestPractice: "loud"},
	} {
		if err := RunWatch([]string{p}, o, WatchConfig{Mode: "all"}); err == nil {
			t.Errorf("%s: an invalid option must be refused before the JAR starts", name)
		}
	}
	t.Setenv("FAKEJAVA_REFUSE", "1")
	if err := RunWatch([]string{p}, opts, WatchConfig{Mode: "all"}); err == nil {
		t.Error("a JAR that exits non-zero must be an error")
	}
}

func TestFakeRunMultiple_FailureModes(t *testing.T) {
	opts, dir := fakeSetup(t)
	for _, tc := range []struct {
		name, body, want string
	}{
		{"out of memory", `{"resourceType":"Patient","note":"FAKE_OOM"}`, "out of memory"},
		{"terminology server unreachable", `{"resourceType":"Patient","note":"FAKE_TX_DOWN"}`, "terminology server unreachable"},
		{"output that isn't JSON", `{"resourceType":"Patient","note":"FAKE_BAD_OUTPUT"}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := put(t, dir, strings.ReplaceAll(tc.name, " ", "-")+".json", tc.body)
			rs, err := RunMultiple([]string{p}, opts)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("want an error with %q, got %v (%+v)", tc.want, err, rs)
			}
		})
	}
	if rs, err := RunMultiple(nil, opts); rs != nil || err != nil {
		t.Errorf("no inputs is no run: %v %v", rs, err)
	}
	p := put(t, dir, "ok.json", `{"resourceType":"Patient"}`)
	for name, o := range map[string]Options{
		"FHIR version":         {FHIRVersion: "9.9.9", JARPath: opts.JARPath},
		"extra arg":            {FHIRVersion: "4.0.1", JARPath: opts.JARPath, ExtraArgs: []string{"-output"}},
		"best practice":        {FHIRVersion: "4.0.1", JARPath: opts.JARPath, BestPractice: "loud"},
		"expansion parameters": {FHIRVersion: "4.0.1", JARPath: opts.JARPath, ExpansionParameters: filepath.Join(dir, "missing.json")},
	} {
		if _, err := RunMultiple([]string{p}, o); err == nil {
			t.Errorf("%s: an invalid option must be refused before the JAR starts", name)
		}
	}
}

// After a crash the rest is validated again in a new JVM; if --timeout runs
// out during that, the files left are reported as not validated, with why.
func TestFakeRunMultiple_TimeoutWhileRerunningAfterACrash(t *testing.T) {
	opts, dir := fakeSetup(t)
	crash := put(t, dir, "a.json", `{"resourceType":"Patient","note":"FAKE_CRASH"}`)
	slow := put(t, dir, "b.json", `{"resourceType":"Patient","note":"FAKE_SLOW"}`)
	opts.Timeout = 1500 * time.Millisecond
	rs, err := RunMultiple([]string{crash, slow}, opts)
	if err != nil {
		t.Fatalf("a timeout after a crash is reported per file, not as an error: %v", err)
	}
	if rs[0].Issues[0].MessageID != MessageIDValidatorCrash {
		t.Errorf("the crashed file: %+v", rs[0].Issues)
	}
	if rs[1].Issues[0].MessageID != MessageIDNotValidated || !strings.Contains(rs[1].Issues[0].Message, "--timeout") {
		t.Errorf("the file left must say --timeout ran out: %+v", rs[1].Issues)
	}

	// Without a crash first, the timeout is the run's error.
	if _, err := RunMultiple([]string{slow}, opts); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("a plain timeout: %v", err)
	}
}

// The checks before the JAR starts: --offline needs a cached JAR, and
// registry routing a validator that has it.
func TestFakeRunMultiple_StartupChecks(t *testing.T) {
	opts, dir := fakeSetup(t)
	p := put(t, dir, "ok.json", `{"resourceType":"Patient"}`)
	if _, err := RunMultiple([]string{p}, Options{FHIRVersion: "4.0.1", Offline: true}); err == nil || !strings.Contains(err.Error(), "--offline: no validator JAR in the cache") {
		t.Errorf("--offline with an empty cache: %v", err)
	}

	old := filepath.Join(t.TempDir(), "validator_cli.jar")
	writeManifestJAR(t, old, "6.10.4")
	if _, err := RunMultiple([]string{p}, Options{FHIRVersion: "4.0.1", JARPath: old, Ecosystem: "http://127.0.0.1:1/tx-reg"}); err == nil || !strings.Contains(err.Error(), MinEcosystemVersion) {
		t.Errorf("registry routing with 6.10.4 must ask for %s: %v", MinEcosystemVersion, err)
	}
	if _, err := RunMultiple([]string{p}, opts); err != nil {
		t.Errorf("the fake's 7.0.5 runs: %v", err)
	}
}

func writeManifestJAR(t *testing.T, path, version string) {
	t.Helper()
	f, err := os.Create(path) //nolint:gosec // test temp file
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, _ := zw.Create("META-INF/MANIFEST.MF")
	_, _ = w.Write([]byte("Manifest-Version: 1.0\r\nClass-Path: org.hl7.fhir.validation/" + version + "/x.jar\r\n"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
