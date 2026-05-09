package input

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
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

	in, err := Resolve(f.Name(), "")
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

	in, err := Resolve(dir, "")
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
	_, err := Resolve("/this/does/not/exist.json", "")
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

	in, err := Resolve("", srv.URL+"/fhir/Patient/test")
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

	in, err := Resolve("", srv.URL+"/fhir/Patient/test")
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

	_, err := Resolve("", srv.URL+"/not-found")
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
