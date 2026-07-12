//go:build integration

package validator

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestIntegration_ServerLifecycle starts a real validator server, validates two
// resources through it, and stops it — exercising StartServer, RunMultipleViaServer
// and Stop end to end.
func TestIntegration_ServerLifecycle(t *testing.T) {
	cfg := ServerConfig{
		Port:                18744,
		FHIRVersion:         "4.0.1",
		NoTerminologyServer: true,
	}
	srv, err := StartServer(cfg, io.Discard, 5*time.Minute)
	if err != nil {
		t.Fatalf("StartServer: %v", err)
	}
	defer func() { _ = srv.Stop() }()

	dir := t.TempDir()
	valid := filepath.Join(dir, "valid.json")
	invalid := filepath.Join(dir, "invalid.json")
	if err := os.WriteFile(valid, []byte(`{"resourceType":"Patient","id":"p1","name":[{"family":"X"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(invalid, []byte(`{"resourceType":"Patient","id":"BAD ID"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	results, err := RunMultipleViaServer(srv.URL(), []string{valid, invalid}, Options{Timeout: time.Minute})
	if err != nil {
		t.Fatalf("RunMultipleViaServer: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if !results[0].Valid {
		t.Errorf("expected valid Patient to pass, issues: %+v", results[0].Issues)
	}
	if results[1].Valid {
		t.Error("expected invalid Patient (bad id) to fail")
	}

	// A second validation should be fast (warm) — just assert it still works.
	again, err := RunMultipleViaServer(srv.URL(), []string{valid}, Options{Timeout: time.Minute})
	if err != nil {
		t.Fatalf("second RunMultipleViaServer: %v", err)
	}
	if len(again) != 1 || !again[0].Valid {
		t.Fatalf("warm re-validation failed: %+v", again)
	}
}

// TestIntegration_ServerStartTimeout verifies StartServer gives up (and stops
// the process) when the server does not become ready within the timeout, rather
// than hanging.
func TestIntegration_ServerStartTimeout(t *testing.T) {
	// A tiny timeout fires long before package loading finishes.
	_, err := StartServer(ServerConfig{Port: 18745, FHIRVersion: "4.0.1", NoTerminologyServer: true}, io.Discard, 10*time.Millisecond)
	if err == nil {
		t.Fatal("expected StartServer to time out")
	}
}
