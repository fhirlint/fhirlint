// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// lsp's config, --offline, --server and startup failures, and the
// suppressor's file errors (#690).

// runLSPSession runs lsp over a session that only initializes and exits.
func runLSPSession(t *testing.T, dir string, args ...string) fakeRun {
	t.Helper()
	session := frame(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{}}}`) +
		frame(`{"jsonrpc":"2.0","id":2,"method":"shutdown"}`) +
		frame(`{"jsonrpc":"2.0","method":"exit"}`)
	f, err := os.Open(writeFile(t, t.TempDir(), "session.txt", session)) //nolint:gosec // a test file
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	old := os.Stdin
	os.Stdin = f
	defer func() { os.Stdin = old }()
	return runFakeKeepEnv(t, dir, append([]string{"lsp", "--no-suppress-action"}, args...)...)
}

// fhir-version and ig from fhirlint.yml reach the server when no flag names
// them, and --offline reaches it as -no-http-access.
func TestFakeLSP_ConfigAndOfflineReachTheServer(t *testing.T) {
	prepareFake(t)
	log := filepath.Join(t.TempDir(), "calls.jsonl")
	t.Setenv(fakeJavaLogEnv, log)
	dir := t.TempDir()
	writeFile(t, dir, "fhirlint.yml", "fhir-version: 4.3.0\nig:\n  - example.ig#1.0.0\n")

	// --offline cannot download, so an IG not in the cache stops the start.
	r := runLSPSession(t, dir, "--offline")
	if r.err == nil || !strings.Contains(r.err.Error(), "--offline forbids downloading") {
		t.Fatalf("an uncached IG under --offline must fail: %v", r.err)
	}
	cachePackage(t, "example.ig", "1.0.0", nil)
	r = runLSPSession(t, dir, "--offline")
	if r.err != nil {
		t.Fatalf("lsp: %v\n%s", r.err, r.stderr)
	}
	calls := fakeJavaCalls(t, log)
	server := calls[len(calls)-1]
	if v := argAfter(server, "-version"); len(v) != 1 || v[0] != "4.3.0" {
		t.Errorf("fhir-version from the config must reach the server: %v", server)
	}
	if !slices.Contains(argAfter(server, "-ig"), "example.ig#1.0.0") {
		t.Errorf("ig from the config must reach the server: %v", server)
	}
	if !slices.Contains(server, "-no-http-access") {
		t.Errorf("--offline must reach the server as -no-http-access: %v", server)
	}

	// A flag beats the config.
	r = runLSPSession(t, dir, "--fhir-version", "4.0.1")
	if r.err != nil {
		t.Fatalf("lsp: %v\n%s", r.err, r.stderr)
	}
	calls = fakeJavaCalls(t, log)
	if v := argAfter(calls[len(calls)-1], "-version"); len(v) != 1 || v[0] != "4.0.1" {
		t.Errorf("--fhir-version must win over the config: %v", calls[len(calls)-1])
	}
}

// With --server no JAR is started, and --offline says it cannot apply.
func TestFakeLSP_AnExternalServer(t *testing.T) {
	prepareFake(t)
	log := filepath.Join(t.TempDir(), "calls.jsonl")
	t.Setenv(fakeJavaLogEnv, log)
	r := runLSPSession(t, t.TempDir(), "--server", "http://127.0.0.1:9", "--offline")
	if r.err != nil {
		t.Fatalf("lsp: %v\n%s", r.err, r.stderr)
	}
	if !strings.Contains(r.stderr, "--offline has no effect with --server") || !strings.Contains(r.stderr, "ready (validator at http://127.0.0.1:9)") {
		t.Errorf("stderr:\n%s", r.stderr)
	}
	if _, err := os.Stat(log); err == nil {
		t.Errorf("no validator may be started for an external server: %v", fakeJavaCalls(t, log))
	}
	if strings.Contains(r.stdout, "fhirlint lsp:") {
		t.Errorf("log lines must stay off the protocol stream:\n%s", r.stdout)
	}
}

func TestFakeLSP_StartupFailures(t *testing.T) {
	prepareFake(t)
	t.Setenv("FAKEJAVA_SERVER_FAIL", "1")
	if r := runLSPSession(t, t.TempDir()); r.err == nil {
		t.Error("a validator server that fails to start must fail lsp")
	}

	t.Setenv("PATH", t.TempDir())
	if r := runLSPSession(t, t.TempDir()); r.err == nil || !strings.Contains(r.err.Error(), "java not found") {
		t.Errorf("no Java must be said: %v", r.err)
	}
}

func TestConfigSuppressor_FileErrors(t *testing.T) {
	dir := t.TempDir()
	if err := (&configSuppressor{path: dir}).Suppress("dom-6"); err == nil || !strings.Contains(err.Error(), "reading ") {
		t.Errorf("a config that cannot be read must be an error, not overwritten: %v", err)
	}
	if err := (&configSuppressor{path: filepath.Join(dir, "missing", "fhirlint.yml")}).Suppress("dom-6"); err == nil || !strings.Contains(err.Error(), "writing ") {
		t.Errorf("a config that cannot be written must be an error: %v", err)
	}
}
