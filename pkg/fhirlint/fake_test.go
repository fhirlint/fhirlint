package fhirlint

import (
	"archive/zip"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The library's Validate functions run against internal/tools/fakejava, a
// stand-in for "java" running the HL7 validator (#595), so they are tested
// end to end without Java. The fake's input markers are described there.

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

// fakeOpts puts the fake java first on PATH and returns options that use a
// JAR whose manifest names validator 7.0.5.
func fakeOpts(t *testing.T) Options {
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
	w, err := zw.Create("META-INF/MANIFEST.MF")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("Manifest-Version: 1.0\r\nClass-Path: org.hl7.fhir.validation/7.0.5/x.jar\r\n"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return Options{JARPath: jar, NoTerminologyServer: true}
}

const (
	okPatient  = `{"resourceType":"Patient","id":"p1"}`
	badPatient = `{"resourceType":"Patient","id":"p2","note":"FAKE_ERROR"}`
)

func TestFakeValidate_Bytes(t *testing.T) {
	opts := fakeOpts(t)
	r, err := Validate([]byte(badPatient), opts)
	if err != nil {
		t.Fatal(err)
	}
	if r.Valid || len(r.Issues) != 1 || r.Issues[0].MessageID != "FAKE_ERROR_ID" || !strings.Contains(r.Issues[0].Location, "line 1") {
		t.Fatalf("want one error with id and line: %+v", r)
	}
	r, err = Validate([]byte(okPatient), opts)
	if err != nil || !r.Valid {
		t.Fatalf("clean resource: %+v, %v", r, err)
	}
	if _, err := Validate([]byte(`<Patient xmlns="http://hl7.org/fhir"/>`), opts); err != nil {
		t.Fatalf("XML content: %v", err)
	}
}

func TestFakeValidate_FileAndRedact(t *testing.T) {
	opts := fakeOpts(t)
	p := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(p, []byte(badPatient), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := ValidateFile(p, opts)
	if err != nil {
		t.Fatal(err)
	}
	if r.Label != p || r.Valid || r.Issues[0].Message != "Fake error found" {
		t.Fatalf("file result: %+v", r)
	}
	opts.Redact = true
	r, err = ValidateFile(p, opts)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(r.Issues[0].Message, "Fake error found") || !r.Issues[0].Redacted || r.Issues[0].MessageID != "FAKE_ERROR_ID" {
		t.Fatalf("Redact must replace the text and keep the id: %+v", r.Issues[0])
	}
	if _, err := ValidateFile(filepath.Join(t.TempDir(), "missing.json"), opts); err == nil {
		t.Error("a missing file must be an error")
	}
}

func TestFakeValidate_Dir(t *testing.T) {
	opts := fakeOpts(t)
	dir := t.TempDir()
	for name, body := range map[string]string{"a.json": okPatient, "b.json": badPatient, "notes.txt": "not FHIR"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	rs, err := ValidateDir(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 {
		t.Fatalf("want the two FHIR files, got %d", len(rs))
	}
	var invalid int
	for _, r := range rs {
		if !r.Valid {
			invalid++
			if !strings.HasSuffix(r.Label, "b.json") {
				t.Errorf("the error belongs to b.json, got %s", r.Label)
			}
		}
	}
	if invalid != 1 {
		t.Errorf("want one invalid file: %+v", rs)
	}
	empty, err := ValidateDir(t.TempDir(), opts)
	if err != nil || empty != nil {
		t.Errorf("an empty directory: %v, %v", empty, err)
	}
	if _, err := ValidateDir(filepath.Join(dir, "missing"), opts); err == nil {
		t.Error("a missing directory must be an error")
	}
}

func TestFakeValidate_URL(t *testing.T) {
	opts := fakeOpts(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Patient/p2" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(badPatient))
	}))
	defer srv.Close()
	r, err := ValidateURL(srv.URL+"/Patient/p2", opts)
	if err != nil {
		t.Fatal(err)
	}
	if r.Label != srv.URL+"/Patient/p2" || r.Valid {
		t.Fatalf("URL result: %+v", r)
	}
	if _, err := ValidateURL(srv.URL+"/missing", opts); err == nil {
		t.Error("a 404 must be an error")
	}
}
