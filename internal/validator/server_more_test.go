// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package validator

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fhirlint/fhirlint/internal/cache"
)

// The corners of the validator server and the client side of it (#691).

// The server gets the registry and the settings file it is meant to use, and
// a registry only with a terminology server to route from.
func TestServerArgs_EcosystemAndSettings(t *testing.T) {
	args := serverArgs("v.jar", ServerConfig{Port: 8080, FHIRVersion: "4.0.1", TerminologyServer: "http://127.0.0.1:1",
		Ecosystem: "http://127.0.0.1:1/tx-reg", FHIRSettings: "s.json", Profiles: []string{"http://p"}})
	for _, pair := range [][2]string{{"-ecosystem", "http://127.0.0.1:1/tx-reg"}, {"-fhir-settings", "s.json"}, {"-profile", "http://p"}} {
		i := slices.Index(args, pair[0])
		if i < 0 || i+1 >= len(args) || args[i+1] != pair[1] {
			t.Errorf("want %s %s in %q", pair[0], pair[1], args)
		}
	}
	args = serverArgs("v.jar", ServerConfig{Port: 8080, FHIRVersion: "4.0.1", NoTerminologyServer: true, Ecosystem: "http://x/tx-reg"})
	if slices.Contains(args, "-ecosystem") {
		t.Errorf("no registry without a terminology server to route from: %q", args)
	}
	// A terminology server alone (a single-server recording) gets no registry.
	args = serverArgs("v.jar", ServerConfig{Port: 8080, FHIRVersion: "4.0.1", TerminologyServer: "http://127.0.0.1:1"})
	if slices.Contains(args, "-ecosystem") {
		t.Errorf("no -ecosystem without a registry: %q", args)
	}
}

// What StartServer refuses before it starts any JVM.
func TestStartServer_RefusesUpFront(t *testing.T) {
	t.Setenv(cache.DirEnvVar, t.TempDir())
	if _, err := StartServer(ServerConfig{Port: 8080, FHIRVersion: "4.0.1", Offline: true}, io.Discard, time.Second); err == nil || !strings.Contains(err.Error(), "--offline: no validator JAR in the cache") {
		t.Errorf("--offline without a cached JAR: %v", err)
	}
	old := writeJARWithManifest(t, t.TempDir(), "6.9.0")
	if _, err := StartServer(ServerConfig{Port: 8080, FHIRVersion: "4.0.1", JARPath: old, Offline: true}, io.Discard, time.Second); err == nil || !strings.Contains(err.Error(), "--offline") {
		t.Errorf("--offline needs a validator that can block its own HTTP: %v", err)
	}
	pre := writeJARWithManifest(t, t.TempDir(), "7.0.3")
	if _, err := StartServer(ServerConfig{Port: 8080, FHIRVersion: "4.0.1", JARPath: pre, TerminologyServer: "http://127.0.0.1:1", Ecosystem: "http://127.0.0.1:1/tx-reg"}, io.Discard, time.Second); err == nil || !strings.Contains(err.Error(), "registry routing") {
		t.Errorf("a registry recording needs 7.0.4 or later: %v", err)
	}
}

// The ready banner is found however the output is split, also after more
// than the rolling buffer has gone by.
func TestReadyWriter_MarkerAfterALongLog(t *testing.T) {
	rw := &readyWriter{w: io.Discard, marker: serverReadyMarker, ready: make(chan struct{})}
	_, _ = rw.Write([]byte(strings.Repeat("loading packages …\n", 1000)))
	half := len(serverReadyMarker) / 2
	_, _ = rw.Write([]byte("noise " + serverReadyMarker[:half]))
	_, _ = rw.Write([]byte(serverReadyMarker[half:] + " on port 8080\n"))
	select {
	case <-rw.ready:
	default:
		t.Fatal("the banner split across writes after a long log must still be seen")
	}
	if len(rw.buf) > 8192 {
		t.Errorf("the buffer must stay bounded, holds %d bytes", len(rw.buf))
	}
}

func TestValidateBytesViaServer_Failures(t *testing.T) {
	answer := func(status int, body string) string {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, body)
		}))
		t.Cleanup(srv.Close)
		return srv.URL
	}
	for _, tc := range []struct{ name, url, want string }{
		{"not JSON", answer(200, "<html>oops</html>"), ""},
		{"an empty Bundle", answer(200, `{"resourceType":"Bundle","type":"collection","entry":[]}`), "produced no result for p.json"},
		{"an invalid server URL", "http://bad host", "invalid server URL"},
	} {
		_, err := ValidateBytesViaServer(tc.url, []byte(`{"resourceType":"Patient"}`), "p.json", Options{})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want an error with %q", tc.name, err, tc.want)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, okOperationOutcome) }))
	defer srv.Close()
	missing := filepath.Join(t.TempDir(), "gone.json")
	if _, err := RunMultipleViaServer(srv.URL, []string{missing}, Options{}); err == nil || !strings.Contains(err.Error(), "reading") {
		t.Errorf("a file that can't be read: %v", err)
	}
	ok := filepath.Join(t.TempDir(), "p.json")
	if err := os.WriteFile(ok, []byte(`{"resourceType":"Patient"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if rs, err := RunMultipleViaServer(srv.URL, []string{ok, ok}, Options{}); err != nil || len(rs) != 2 {
		t.Errorf("one result per file: %d, %v", len(rs), err)
	}
}
