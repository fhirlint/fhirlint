//go:build !integration

package validator

import (
	"archive/zip"
	"fmt"
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

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fhirlint-fakejava-")
	if err == nil {
		if err = buildFakeJava(dir); err == nil {
			fakeJavaDir = dir
		} else {
			fmt.Fprintf(os.Stderr, "fake java not built, its tests are skipped: %v\n", err)
		}
	}
	code := m.Run()
	if dir != "" {
		_ = os.RemoveAll(dir)
	}
	os.Exit(code)
}

func buildFakeJava(dir string) error {
	gobin := filepath.Join(runtime.GOROOT(), "bin", "go")
	if _, err := os.Stat(gobin); err != nil {
		if gobin, err = exec.LookPath("go"); err != nil {
			return err
		}
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
	c := put(t, dir, "c.json", `{"resourceType":"Patient","note":"FAKE_CRASH"}`)
	// An exception fhirlint doesn't recognise fails the run and quotes what
	// the JAR printed, instead of reporting the files as valid.
	rs, err := RunMultiple([]string{a, c}, opts)
	if err == nil {
		t.Fatalf("an unrecognised crash must fail the run, got %+v", rs)
	}
	for _, want := range []string{"validator produced no result", "c.json", "IllegalStateException: fake crash"} {
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
