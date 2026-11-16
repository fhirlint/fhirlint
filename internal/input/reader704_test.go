// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package input

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fhirlintTemps lists the temp files fhirlint made in dir.
func fhirlintTemps(t *testing.T, dir string) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(dir, "fhirlint-*"))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// useTempDir points os.TempDir at dir, on every platform.
func useTempDir(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("TMPDIR", dir)
	if runtime.GOOS == "windows" {
		t.Setenv("TMP", dir)
		t.Setenv("TEMP", dir)
	}
}

// A body that ends before its Content-Length is a failed fetch, not a short
// resource handed to the validator, and the partial file is removed (#704).
func TestResolve_URLBodyCutOff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write([]byte(`{"resourceType":`))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("the test server cannot drop the connection")
			return
		}
		conn, _, err := hj.Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}))
	defer srv.Close()
	tmp := t.TempDir()
	useTempDir(t, tmp)

	in, err := Resolve("", srv.URL+"/p.json", 0)
	if err == nil || !strings.Contains(err.Error(), "reading response") {
		t.Errorf("err = %v (input %+v), want a read failure", err, in)
	}
	if left := fhirlintTemps(t, tmp); len(left) != 0 {
		t.Errorf("the partial download must be removed, found %q", left)
	}
}

// No usable temp directory: --url and stdin both say what failed (#704).
func TestResolve_NoTempDirectory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"resourceType":"Patient"}`))
	}))
	defer srv.Close()
	useTempDir(t, filepath.Join(t.TempDir(), "absent"))

	if _, err := Resolve("", srv.URL+"/p.json", 0); err == nil || !strings.Contains(err.Error(), "creating temp file") {
		t.Errorf("--url: %v", err)
	}
	if _, err := writeTempFileDetectExt([]byte(`{}`), "stdin"); err == nil || !strings.Contains(err.Error(), "creating temp file") {
		t.Errorf("stdin: %v", err)
	}
}

// Stdin that cannot be read is an error naming stdin (#704). A directory
// stands in for it: reading one fails on every platform.
func TestResolve_StdinUnreadable(t *testing.T) {
	d, err := os.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	old := os.Stdin
	os.Stdin = d
	defer func() { os.Stdin = old }()

	if _, err := Resolve("-", "", 0); err == nil || !strings.Contains(err.Error(), "reading stdin") {
		t.Errorf("err = %v, want a stdin read error", err)
	}
}
