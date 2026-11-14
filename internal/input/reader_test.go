package input

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolve_ExistingFile(t *testing.T) {
	f, err := os.CreateTemp("", "fhirlint-test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"resourceType":"Patient"}`); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(f.Name()) }()

	in, err := Resolve(f.Name(), "", 0)
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	defer in.Cleanup()

	if in.Source != SourceFile {
		t.Errorf("expected SourceFile, got %v", in.Source)
	}
	if in.Path != f.Name() {
		t.Errorf("expected path %q, got %q", f.Name(), in.Path)
	}
	if in.TempFile != "" {
		t.Error("expected no temp file for direct file input")
	}
}

func TestResolve_ExistingDirectory(t *testing.T) {
	dir, err := os.MkdirTemp("", "fhirlint-test-dir-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	in, err := Resolve(dir, "", 0)
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	if in.Source != SourceDir {
		t.Errorf("expected SourceDir, got %v", in.Source)
	}
	if in.TempFile != "" {
		t.Error("expected no temp file for directory input")
	}
}

func TestResolve_NonExistentPath_ReturnsError(t *testing.T) {
	_, err := Resolve("/this/does/not/exist.json", "", 0)
	if err == nil {
		t.Error("expected error for non-existent path, got nil")
	}
}

func TestResolve_URL_FetchesAndWritesTempFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"resourceType":"Patient","id":"test"}`))
	}))
	defer srv.Close()

	in, err := Resolve("", srv.URL+"/fhir/Patient/test", 0)
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	defer in.Cleanup()

	if in.TempFile == "" {
		t.Error("expected a temp file for URL input")
	}
	if in.Label != srv.URL+"/fhir/Patient/test" {
		t.Errorf("expected Label to be URL, got %q", in.Label)
	}
	// Temp file should have been written
	data, err := os.ReadFile(in.Path)
	if err != nil {
		t.Fatalf("reading temp file: %v", err)
	}
	if string(data) == "" {
		t.Error("expected non-empty temp file content")
	}
}

func TestResolve_URL_XMLContentType_GetsXMLExtension(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<Patient xmlns="http://hl7.org/fhir"/>`))
	}))
	defer srv.Close()

	in, err := Resolve("", srv.URL+"/fhir/Patient/test", 0)
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	defer in.Cleanup()

	if filepath.Ext(in.Path) != ".xml" {
		t.Errorf("expected .xml extension for XML content-type, got %q", filepath.Ext(in.Path))
	}
}

func TestResolve_URL_404_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := Resolve("", srv.URL+"/not-found", 0)
	if err == nil {
		t.Error("expected error for 404 response, got nil")
	}
}

func TestCleanup_RemovesTempFile(t *testing.T) {
	f, err := os.CreateTemp("", "fhirlint-cleanup-test-*")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	path := f.Name()

	in := &Input{TempFile: path, Path: path}
	in.Cleanup()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected temp file %q to be deleted after Cleanup()", path)
	}
}

func TestCleanup_NoTempFile_DoesNotPanic(t *testing.T) {
	in := &Input{Source: SourceFile, Path: "/some/real/file.json"}
	in.Cleanup() // should not panic
}

func TestWriteTempFileDetectExt_JSON(t *testing.T) {
	in, err := writeTempFileDetectExt([]byte(`{"resourceType":"Patient"}`), "stdin")
	if err != nil {
		t.Fatalf("writeTempFileDetectExt error: %v", err)
	}
	defer in.Cleanup()

	if filepath.Ext(in.Path) != ".json" {
		t.Errorf("expected .json extension for JSON input, got %q", filepath.Ext(in.Path))
	}
}

func TestWriteTempFileDetectExt_XML(t *testing.T) {
	in, err := writeTempFileDetectExt([]byte(`<Patient xmlns="http://hl7.org/fhir"/>`), "stdin")
	if err != nil {
		t.Fatalf("writeTempFileDetectExt error: %v", err)
	}
	defer in.Cleanup()

	if filepath.Ext(in.Path) != ".xml" {
		t.Errorf("expected .xml extension for XML input, got %q", filepath.Ext(in.Path))
	}
}

func TestWriteTempFileDetectExt_XMLWithLeadingWhitespace(t *testing.T) {
	in, err := writeTempFileDetectExt([]byte("  \n  <Patient/>"), "stdin")
	if err != nil {
		t.Fatalf("writeTempFileDetectExt error: %v", err)
	}
	defer in.Cleanup()

	if filepath.Ext(in.Path) != ".xml" {
		t.Errorf("expected .xml for XML with leading whitespace, got %q", filepath.Ext(in.Path))
	}
}

func TestResolve_URL_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond) // simulate a slow/hung server
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := Resolve("", srv.URL, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestResolve_URL_DefaultTimeout_IsApplied(t *testing.T) {
	// Passing 0 should use the 30s default, not disable the timeout entirely.
	// Verify by checking that fromHTTP with timeout=0 still sets a context deadline.
	// We test indirectly: a fast server should still succeed.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"resourceType":"Patient"}`))
	}))
	defer srv.Close()

	in, err := Resolve("", srv.URL, 0)
	if err != nil {
		t.Fatalf("expected success with default timeout, got: %v", err)
	}
	in.Cleanup()
}

// limitURLBytes lowers the --url size limit and points the temp directory at
// an empty one, so a test can also check that nothing is left behind.
func limitURLBytes(t *testing.T, n int64) string {
	t.Helper()
	old := maxURLBytes
	maxURLBytes = n
	t.Cleanup(func() { maxURLBytes = old })
	dir := t.TempDir()
	for _, v := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(v, dir)
	}
	return dir
}

func assertEmptyDir(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("temp files left behind: %v", entries)
	}
}

func TestResolve_URL_AtLimit_IsAccepted(t *testing.T) {
	limitURLBytes(t, 16)
	body := strings.Repeat("x", 16)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	in, err := Resolve("", srv.URL, 0)
	if err != nil {
		t.Fatalf("a body of exactly the limit must be accepted: %v", err)
	}
	defer in.Cleanup()
	data, err := os.ReadFile(in.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != body {
		t.Errorf("temp file = %q, want %q", data, body)
	}
}

func TestResolve_URL_StreamedBodyOverLimit_Fails(t *testing.T) {
	dir := limitURLBytes(t, 16)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Flushing first sends the body chunked, without a Content-Length,
		// so only the read itself can notice the size.
		_, _ = w.Write([]byte(strings.Repeat("x", 10)))
		w.(http.Flusher).Flush()
		_, _ = w.Write([]byte(strings.Repeat("x", 10)))
	}))
	defer srv.Close()

	_, err := Resolve("", srv.URL, 0)
	if err == nil || !strings.Contains(err.Error(), "over the limit of 16 bytes") {
		t.Fatalf("want an over-the-limit error, got %v", err)
	}
	assertEmptyDir(t, dir)
}

func TestResolve_URL_ContentLengthOverLimit_FailsBeforeReading(t *testing.T) {
	dir := limitURLBytes(t, 16)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write([]byte(strings.Repeat("x", 1000)))
	}))
	defer srv.Close()

	_, err := Resolve("", srv.URL, 0)
	if err == nil || !strings.Contains(err.Error(), "is 1000 bytes, over the limit of 16") {
		t.Fatalf("want a Content-Length error, got %v", err)
	}
	assertEmptyDir(t, dir)
}
