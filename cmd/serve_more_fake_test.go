// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// serve with --tx-offline, --offline, credentials and a server that dies
// (#666), against the fake validator's server mode.

// startFakeServe runs serve in the background and returns its base URL and a
// channel with its result.
func startFakeServe(t *testing.T, dir string, args ...string) (string, chan fakeRun) {
	t.Helper()
	port := freeServerPort(t)
	done := make(chan fakeRun, 1)
	go func() {
		done <- runFakeKeepEnv(t, dir, append([]string{"serve", "--port", strconv.Itoa(port)}, args...)...)
	}()
	waitForPort(t, port)
	return fmt.Sprintf("http://127.0.0.1:%d", port), done
}

func post(t *testing.T, url, body string) string {
	t.Helper()
	resp, err := http.Post(url, "application/fhir+json", strings.NewReader(body)) //nolint:gosec // loopback test server
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func waitServe(t *testing.T, done chan fakeRun) fakeRun {
	t.Helper()
	select {
	case r := <-done:
		return r
	case <-time.After(15 * time.Second):
		t.Fatal("serve did not return")
		return fakeRun{}
	}
}

// A long-lived server can't fail on a replay miss, so it reports the misses
// when it stops.
func TestFakeServe_TxOfflineReportsMissesAtShutdown(t *testing.T) {
	tx, _ := fakeTxServer(t)
	dir := t.TempDir()
	writeFile(t, dir, "fhir/p.json", `{"resourceType":"Patient","id":"p1","a":"FAKE_TX:known"}`)
	prepareFake(t)
	if r := runFakeKeepEnv(t, dir, "tx", "warm", "--terminology-server", tx.URL+"/r4", "fhir"); r.err != nil {
		t.Fatalf("tx warm: %v\n%s", r.err, r.stderr)
	}
	tx.Close() // from here on only the recording answers

	base, done := startFakeServe(t, dir, "--tx-offline")
	if out := post(t, base+"/validateResource", `{"resourceType":"Patient","a":"FAKE_TX:known"}`); strings.Contains(out, "FAKE_TX") {
		t.Errorf("a recorded code is answered from the recording: %s", out)
	}
	if out := post(t, base+"/validateResource", `{"resourceType":"Patient","a":"FAKE_TX:other"}`); !strings.Contains(out, "FAKE_TX_UNAVAILABLE") {
		t.Errorf("a code the recording lacks gets no answer: %s", out)
	}
	_ = post(t, base+"/stop", "")
	r := waitServe(t, done)
	if r.err != nil {
		t.Fatalf("serve: %v\n%s", r.err, r.stderr)
	}
	for _, want := range []string{"Replaying 1 recorded terminology interaction(s)", "warn: 1 terminology request(s) were not in the recording", "Re-record with: fhirlint tx warm"} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, r.stderr)
		}
	}
}

// --offline blocks the validator's own HTTP for the server's lifetime.
func TestFakeServe_Offline(t *testing.T) {
	prepareFake(t)
	log := filepath.Join(t.TempDir(), "calls.jsonl")
	t.Setenv(fakeJavaLogEnv, log)
	base, done := startFakeServe(t, t.TempDir(), "--offline")
	_ = post(t, base+"/stop", "")
	if r := waitServe(t, done); r.err != nil {
		t.Fatalf("serve --offline: %v\n%s", r.err, r.stderr)
	}
	calls := fakeJavaCalls(t, log)
	server := calls[len(calls)-1]
	if !slices.Contains(server, "-no-http-access") {
		t.Errorf("--offline must reach the server as -no-http-access: %v", server)
	}
	if r := runFakeKeepEnv(t, t.TempDir(), "serve", "--offline", "--terminology-server", "https://tx.example.org/r4"); r.err == nil || !strings.Contains(r.err.Error(), "--offline forbids network access") {
		t.Errorf("--offline with --terminology-server: %v", r.err)
	}
}

// A credential is never sent over plain HTTP, for serve as for validate.
func TestFakeServe_CredentialsOverPlainHTTPAreRefused(t *testing.T) {
	prepareFake(t)
	t.Setenv("FHIRLINT_TX_TOKEN", "s3cret")
	r := runFakeKeepEnv(t, t.TempDir(), "serve", "--port", strconv.Itoa(freeServerPort(t)), "--terminology-server", "http://tx.example.org/r4")
	if r.err == nil || !strings.Contains(r.err.Error(), "over plain HTTP") {
		t.Errorf("got %v", r.err)
	}
}

// The server dying on its own ends serve with exit 2, not a clean return.
func TestFakeServe_AServerThatDiesExits2(t *testing.T) {
	prepareFake(t)
	base, done := startFakeServe(t, t.TempDir(), "--no-terminology-server")
	_ = post(t, base+"/crash", "")
	r := waitServe(t, done)
	var ee *exitErr
	if !errors.As(r.err, &ee) || ee.code != 2 || !strings.Contains(r.err.Error(), "validator server exited") {
		t.Errorf("want exit 2 saying the server exited, got %v", r.err)
	}
}

// Through serve --tx-offline, a finding names the server the recording stands
// for, not the local replay server and its port (#684).
func TestFakeServe_TxOfflineFindingsNameTheRecordedServer(t *testing.T) {
	tx, _ := fakeTxServer(t)
	upstream := tx.URL + "/r4"
	dir := t.TempDir()
	writeFile(t, dir, "fhir/p.json", `{"resourceType":"Patient","id":"p1","a":"FAKE_TX:known"}`)
	prepareFake(t)
	if r := runFakeKeepEnv(t, dir, "tx", "warm", "--terminology-server", upstream, "fhir"); r.err != nil {
		t.Fatalf("tx warm: %v\n%s", r.err, r.stderr)
	}
	tx.Close()

	base, done := startFakeServe(t, dir, "--tx-offline")
	out := post(t, base+"/validateResource", `{"resourceType":"Patient","a":"FAKE_TX:other"}`)
	if !strings.Contains(out, "Error from "+upstream+":") {
		t.Errorf("the finding must name the recorded server %s:\n%s", upstream, out)
	}
	_ = post(t, base+"/stop", "")
	if r := waitServe(t, done); r.err != nil {
		t.Fatalf("serve: %v\n%s", r.err, r.stderr)
	}
}
