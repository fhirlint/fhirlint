// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package validator

import (
	"archive/zip"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Run and RunMultiple against internal/tools/fakejava, a stand-in for "java"
// running the HL7 validator (#595). The fake's input markers are described
// there; the integration tests cover the real JAR.

var fakeJavaDir string

func init() {
	var dir string
	setUpTests = func() error {
		var err error
		dir, err = os.MkdirTemp("", "fhirlint-fakejava-")
		if err != nil {
			return nil //nolint:nilerr // the fake's tests skip; the rest still run
		}
		if err := buildFakeJava(dir); err != nil {
			fmt.Fprintf(os.Stderr, "fake java not built, its tests are skipped: %v\n", err)
			return nil
		}
		fakeJavaDir = dir
		return nil
	}
	tearDownTests = func(int) {
		if dir != "" {
			_ = os.RemoveAll(dir)
		}
	}
}

func buildFakeJava(dir string) error {
	gobin, err := exec.LookPath("go")
	if err != nil {
		return err
	}
	name := "java"
	if runtime.GOOS == "windows" {
		name = "java.exe"
	}
	cmd := exec.Command(gobin, "build", "-o", filepath.Join(dir, name), "github.com/fhirlint/fhirlint/internal/tools/fakejava") //nolint:gosec // the go toolchain, fixed arguments
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%v: %s", err, out)
	}
	return nil
}

// fakeSetup puts the fake java first on PATH, HOME in a temp dir, and
// returns options that use a JAR whose manifest names validator 7.0.5.
func fakeSetup(t *testing.T) (Options, string) {
	t.Helper()
	if fakeJavaDir == "" {
		t.Skip("fake java could not be built")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PATH", fakeJavaDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	jar := filepath.Join(home, "validator_cli.jar")
	f, err := os.Create(jar) //nolint:gosec // test temp file
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, _ := zw.Create("META-INF/MANIFEST.MF")
	_, _ = w.Write([]byte("Manifest-Version: 1.0\r\nClass-Path: org.hl7.fhir.validation/7.0.5/x.jar\r\n"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return Options{FHIRVersion: "4.0.1", JARPath: jar, NoTerminologyServer: true}, t.TempDir()
}

func put(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFakeRun_SingleFile(t *testing.T) {
	opts, dir := fakeSetup(t)
	r, err := Run(put(t, dir, "p.json", `{"resourceType":"Patient","note":"FAKE_ERROR"}`), opts)
	if err != nil {
		t.Fatal(err)
	}
	if r.Valid || len(r.Issues) != 1 {
		t.Fatalf("want one error: %+v", r)
	}
	iss := r.Issues[0]
	if iss.MessageID != "FAKE_ERROR_ID" || iss.Severity != "error" || !strings.Contains(iss.Location, "Patient.name (line 1, col 2)") {
		t.Errorf("issue: %+v", iss)
	}
}

func TestFakeRunMultiple_MapsResultsToInputsInOrder(t *testing.T) {
	opts, dir := fakeSetup(t)
	a := put(t, dir, "a.json", `{"resourceType":"Patient"}`)
	b := put(t, dir, "b.json", `{"resourceType":"Patient","note":"FAKE_WARNING"}`)
	rs, err := RunMultiple([]string{a, b}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 || rs[0].Filename != a || rs[1].Filename != b {
		t.Fatalf("results out of order: %+v", rs)
	}
	if !rs[0].Valid || rs[1].Issues[0].MessageID != "FAKE_WARNING_ID" {
		t.Errorf("a is clean, b has the warning: %+v %+v", rs[0], rs[1])
	}
}

func TestFakeRunMultiple_ACrashIsNotAPass(t *testing.T) {
	opts, dir := fakeSetup(t)
	a := put(t, dir, "a.json", `{"resourceType":"Patient"}`)
	c := put(t, dir, "c.json", `{"resourceType":"Patient","note":"FAKE_CRASH_LOADING"}`)
	// A crash fhirlint can't pin on a file (while loading, before any
	// "Validate" line) fails the run and quotes what the JAR printed, instead
	// of reporting the files as valid.
	rs, err := RunMultiple([]string{a, c}, opts)
	if err == nil {
		t.Fatalf("an unrecognised crash must fail the run, got %+v", rs)
	}
	for _, want := range []string{"validator produced no result", "c.json", "IllegalStateException: fake crash while loading"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q: %v", want, err)
		}
	}
}

func TestFakeRun_TimeoutBoundsTheRun(t *testing.T) {
	opts, dir := fakeSetup(t)
	opts.Timeout = time.Nanosecond
	if _, err := Run(put(t, dir, "p.json", `{"resourceType":"Patient"}`), opts); err == nil || !strings.Contains(err.Error(), "--timeout") {
		t.Fatalf("an expired --timeout must say so, got %v", err)
	}
}

func TestFakeRun_PassesOptionsToTheJAR(t *testing.T) {
	opts, dir := fakeSetup(t)
	log := filepath.Join(t.TempDir(), "calls.jsonl")
	t.Setenv("FAKEJAVA_LOG", log)
	opts.IGs = []string{"kbv.basis#1.5.0"}
	opts.Profiles = []string{"http://example.org/StructureDefinition/p", "de.basisprofil.r4#1.5.0"}
	opts.Locale = "de"
	if _, err := Run(put(t, dir, "p.json", `{"resourceType":"Patient"}`), opts); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(log) //nolint:gosec // test log
	if err != nil {
		t.Fatal(err)
	}
	call := string(data)
	for _, want := range []string{`"-ig","kbv.basis#1.5.0"`, `"-profile","http://example.org/StructureDefinition/p"`, `"-ig","de.basisprofil.r4#1.5.0"`, `"-locale","de"`, `"-tx","n/a"`} {
		if !strings.Contains(call, want) {
			t.Errorf("call lacks %s: %s", want, call)
		}
	}
}

func TestFakeRun_RefusedRunIsReportedWithWhatTheJARSaid(t *testing.T) {
	opts, dir := fakeSetup(t)
	t.Setenv("FAKEJAVA_REFUSE", "1")
	_, err := Run(put(t, dir, "p.json", `{"resourceType":"Patient"}`), opts)
	if err == nil || !strings.Contains(err.Error(), "validator produced no result") || !strings.Contains(err.Error(), "validation refused") {
		t.Fatalf("a validator that wrote no result must fail and quote its stderr, got %v", err)
	}
}

func freeTestPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestFakeServer_ValidatesAndStops(t *testing.T) {
	opts, dir := fakeSetup(t)
	srv, err := StartServer(ServerConfig{Port: freeTestPort(t), FHIRVersion: "4.0.1", JARPath: opts.JARPath, NoTerminologyServer: true}, io.Discard, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	a := put(t, dir, "a.json", `{"resourceType":"Patient"}`)
	b := put(t, dir, "b.json", `{"resourceType":"Patient","note":"FAKE_ERROR"}`)
	rs, err := RunMultipleViaServer(srv.URL(), []string{a, b}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 || !rs[0].Valid || rs[1].Valid || rs[1].Issues[0].MessageID != "FAKE_ERROR_ID" {
		t.Fatalf("results via the server: %+v %+v", rs[0], rs[1])
	}
	r, err := ValidateBytesViaServer(srv.URL(), []byte(`<Patient xmlns="http://hl7.org/fhir"><note value="FAKE_WARNING"/></Patient>`), "buffer.xml", Options{Profiles: []string{"http://example.org/p"}})
	if err != nil || r.Issues[0].MessageID != "FAKE_WARNING_ID" {
		t.Fatalf("in-memory XML via the server: %+v, %v", r, err)
	}
	if _, err := ValidateBytesViaServer(srv.URL(), []byte("FAKE_CRASH"), "crash.json", Options{}); err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("a server error must be an error, got %v", err)
	}
	// Stop's own error is not checked: it kills the process after asking it
	// to stop, and the exit status of a killed process differs by platform
	// (Windows reports 1). What matters is that the server is gone.
	_ = srv.Stop()
	if _, err := RunMultipleViaServer(srv.URL(), []string{a}, Options{Timeout: time.Second}); err == nil {
		t.Error("a stopped server must be an error, not a pass")
	}
}

func TestFakeServer_StartupFailures(t *testing.T) {
	opts, _ := fakeSetup(t)
	cfg := ServerConfig{Port: freeTestPort(t), FHIRVersion: "4.0.1", JARPath: opts.JARPath}
	t.Setenv("FAKEJAVA_SERVER_FAIL", "1")
	if _, err := StartServer(cfg, io.Discard, 10*time.Second); err == nil || !strings.Contains(err.Error(), "exited during startup") {
		t.Errorf("a server that exits before the banner: %v", err)
	}
	t.Setenv("FAKEJAVA_SERVER_FAIL", "")
	t.Setenv("FAKEJAVA_SERVER_SILENT", "1")
	cfg.Port = freeTestPort(t)
	if _, err := StartServer(cfg, io.Discard, 500*time.Millisecond); err == nil || !strings.Contains(err.Error(), "did not become ready") {
		t.Errorf("a server that never says it is ready: %v", err)
	}
	if _, err := StartServer(ServerConfig{Port: 0, FHIRVersion: "4.0.1"}, io.Discard, time.Second); err == nil {
		t.Error("port 0 must be rejected")
	}
	if _, err := StartServer(ServerConfig{Port: 1, FHIRVersion: "9.9.9", JARPath: opts.JARPath}, io.Discard, time.Second); err == nil {
		t.Error("an unknown FHIR version must be rejected")
	}
}

func TestFakeRunCompare(t *testing.T) {
	opts, _ := fakeSetup(t)
	co := CompareOptions{FHIRVersion: "4.0.1", JARPath: opts.JARPath, DestDir: t.TempDir(), IGs: []string{"kbv.basis#1.5.0"}}
	res, err := RunCompare("http://x/A", "http://x/B", co)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Differs() || res.Messages[0].Path != "Patient.name" || res.HTMLFile == "" {
		t.Errorf("differing profiles: %+v", res)
	}
	co.DestDir = t.TempDir()
	// --output names the destination, and a name with glob characters must
	// still find the page the JAR wrote (#705).
	bracketed := co
	bracketed.DestDir = filepath.Join(t.TempDir(), "out[1]")
	if res, err := RunCompare("http://x/A", "http://x/B", bracketed); err != nil || !res.Differs() {
		t.Errorf("destination %q: %v %+v", bracketed.DestDir, err, res)
	}
	if res, err := RunCompare("http://x/A", "http://x/A", co); err != nil || res.Differs() {
		t.Errorf("a profile against itself: %+v %v", res, err)
	}
	co.DestDir = t.TempDir()
	if _, err := RunCompare("unresolvable", "http://x/A", co); err == nil || !strings.Contains(err.Error(), "Unable to resolve") {
		t.Errorf("a side the JAR can't resolve must surface its line, got %v", err)
	}
	for _, bad := range []CompareOptions{{FHIRVersion: "4.0.1"}, {FHIRVersion: "9.9.9", DestDir: t.TempDir()}} {
		if _, err := RunCompare("a", "b", bad); err == nil {
			t.Errorf("%+v must be rejected", bad)
		}
	}
	if _, err := RunCompare("", "b", co); err == nil {
		t.Error("an empty side must be rejected")
	}
}

func TestFakeRunFHIRPath(t *testing.T) {
	opts, dir := fakeSetup(t)
	fo := FHIRPathOptions{FHIRVersion: "4.0.1", JARPath: opts.JARPath, Timeout: time.Minute}
	p := put(t, dir, "p.json", `{"resourceType":"Patient","name":[{"given":["Anna","Lena"]}]}`)

	res, err := RunFHIRPath("Patient.name.given", p, fo)
	if err != nil || strings.Join(res.Items, "|") != "Anna|Lena" {
		t.Errorf("items: %+v %v", res, err)
	}
	if res, err := RunFHIRPath("Patient.birthDate", p, fo); err != nil || !res.Empty() {
		t.Errorf("no match must be an empty result, not an error: %+v %v", res, err)
	}
	if _, err := RunFHIRPath("Patient.name.(", p, fo); err == nil || !strings.Contains(err.Error(), "Expression terminated unexpectedly") || strings.Contains(err.Error(), "FHIRLexerException") {
		t.Errorf("a parse error, without the exception class: %v", err)
	}
	crash := put(t, dir, "crash.json", `{"resourceType":"Patient","note":"FAKE_CRASH"}`)
	if _, err := RunFHIRPath("Patient.name", crash, fo); err == nil || !strings.Contains(err.Error(), "fake crash before evaluation") {
		t.Errorf("a failure before evaluation: %v", err)
	}
}
