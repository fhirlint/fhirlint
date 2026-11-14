// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fhirlint/fhirlint/internal/validator"
)

// The warm validator server, validate --server and the language server against
// the fake validator's server mode (#611).

func freeServerPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port
}

func waitForPort(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if c, err := net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(port)); err == nil {
			_ = c.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("nothing listens on port %d", port)
}

func TestFakeServe_StartsAnswersAndStops(t *testing.T) {
	prepareFake(t)
	dir := t.TempDir()
	port := freeServerPort(t)
	done := make(chan fakeRun, 1)
	go func() {
		done <- runFakeKeepEnv(t, dir, "serve", "--port", strconv.Itoa(port), "--no-terminology-server")
	}()
	waitForPort(t, port)

	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	resp, err := http.Post(base+"/validateResource", "application/fhir+json", strings.NewReader(errPatient)) //nolint:gosec // loopback test server
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), "FAKE_ERROR_ID") {
		t.Errorf("the server must answer like the validator: %s", body)
	}
	// The server stopping on its own ends serve, as a crash of the JVM would.
	resp, err = http.Post(base+"/stop", "", nil) //nolint:gosec // loopback test server
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("serve: %v\n%s", r.err, r.stderr)
		}
		if !strings.Contains(r.stdout, fmt.Sprintf("ready at http://localhost:%d", port)) || !strings.Contains(r.stdout, "--server") {
			t.Errorf("serve must say where it is and how to use it:\n%s", r.stdout)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("serve did not return after the server stopped")
	}
}

func TestFakeServe_UsageErrors(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{
		{"serve", "--offline", "--terminology-server", "https://tx.example.org"},
		{"serve", "--tx-offline", "--no-terminology-server"},
		{"serve", "--tx-offline", "--terminology-server", "https://tx.example.org"},
		{"serve", "--tx-offline"}, // no recording
	} {
		if r := runFake(t, dir, args...); r.err == nil {
			t.Errorf("%q must fail", args)
		}
	}
	t.Setenv("FAKEJAVA_SERVER_FAIL", "1")
	r := runFake(t, dir, "serve", "--port", strconv.Itoa(freeServerPort(t)), "--no-terminology-server")
	var ee *exitErr
	if r.err == nil || !errors.As(r.err, &ee) || ee.code != 2 {
		t.Errorf("a server that fails to start exits 2, got %v", r.err)
	}
}

func TestFakeValidate_ViaServer(t *testing.T) {
	jar := prepareFake(t)
	srv, err := validator.StartServer(validator.ServerConfig{Port: freeServerPort(t), FHIRVersion: "4.0.1", JARPath: jar, NoTerminologyServer: true}, io.Discard, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	writeFile(t, dir, "bad.json", errPatient)
	writeFile(t, dir, "ok.json", cleanPatient)
	r := runFakeKeepEnv(t, dir, "validate", "--no-color", "--server", srv.URL(), "bad.json", "ok.json")
	mustFail(t, r)
	if !strings.Contains(r.stdout, "Fake error found") || !strings.Contains(r.stdout, "Files: 2") {
		t.Errorf("validate --server must report like a normal run:\n%s", r.stdout)
	}

	_ = srv.Stop() // its exit status after the kill differs by platform
	if r := runFakeKeepEnv(t, dir, "validate", "--server", srv.URL(), "ok.json"); r.err == nil {
		t.Error("an unreachable --server must be an error, not a pass")
	}
}

// frame wraps an LSP message in its Content-Length header.
func frame(msg string) string {
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(msg), msg)
}

func TestFakeLSP_PublishesDiagnostics(t *testing.T) {
	prepareFake(t)
	dir := t.TempDir()
	text := strings.ReplaceAll(errPatient, `"`, `\"`)
	session := frame(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{}}}`) +
		frame(`{"jsonrpc":"2.0","method":"initialized","params":{}}`) +
		frame(`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":"file:///work/p.json","languageId":"json","version":1,"text":"`+text+`"}}}`) +
		frame(`{"jsonrpc":"2.0","id":2,"method":"shutdown"}`) +
		frame(`{"jsonrpc":"2.0","method":"exit"}`)
	in := writeFile(t, t.TempDir(), "session.txt", session)
	f, err := os.Open(in) //nolint:gosec // test file
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	old := os.Stdin
	os.Stdin = f
	defer func() { os.Stdin = old }()

	r := runFakeKeepEnv(t, dir, "lsp", "--no-suppress-action")
	if r.err != nil {
		t.Fatalf("lsp: %v\n%s", r.err, r.stderr)
	}
	for _, want := range []string{`"textDocument/publishDiagnostics"`, `"file:///work/p.json"`, `FAKE_ERROR_ID`, `Fake error found`} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("the protocol stream lacks %s:\n%s", want, r.stdout)
		}
	}
	if !strings.Contains(r.stderr, "fhirlint lsp: ready") {
		t.Errorf("human-readable lines go to stderr:\n%s", r.stderr)
	}
}
