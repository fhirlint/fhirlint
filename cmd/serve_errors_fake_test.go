// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// serve's recording, port, proxy and credential paths (#690).

// warmServeRecording records one terminology answer (FAKE_TX:known) into
// dir's default recording and shuts the terminology server down.
func warmServeRecording(t *testing.T, dir string) {
	t.Helper()
	tx, _ := fakeTxServer(t)
	writeFile(t, dir, "fhir/p.json", `{"resourceType":"Patient","id":"p1","a":"FAKE_TX:known"}`)
	prepareFake(t)
	if r := runFakeKeepEnv(t, dir, "tx", "warm", "--terminology-server", tx.URL+"/r4", "fhir"); r.err != nil {
		t.Fatalf("tx warm: %v\n%s", r.err, r.stderr)
	}
	tx.Close()
}

// The shutdown report lists the first ten misses and counts the rest.
func TestFakeServe_TxOfflineCapsTheMissList(t *testing.T) {
	dir := t.TempDir()
	warmServeRecording(t, dir)
	base, done := startFakeServe(t, dir, "--tx-offline")
	for i := range 12 {
		_ = post(t, base+"/validateResource", fmt.Sprintf(`{"resourceType":"Patient","a":"FAKE_TX:miss%02d"}`, i))
	}
	_ = post(t, base+"/stop", "")
	r := waitServe(t, done)
	if r.err != nil {
		t.Fatalf("serve: %v\n%s", r.err, r.stderr)
	}
	if !strings.Contains(r.stderr, "warn: 12 terminology request(s) were not in the recording") || !strings.Contains(r.stderr, "  … and 2 more\n") {
		t.Errorf("twelve misses: ten listed, two counted:\n%s", r.stderr)
	}
	listed := 0
	for _, line := range strings.Split(r.stderr, "\n") {
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "  …") {
			listed++
		}
	}
	if listed != maxReportedMisses {
		t.Errorf("want %d listed misses, got %d:\n%s", maxReportedMisses, listed, r.stderr)
	}
}

func TestFakeServe_TxOfflineCorruptRecording(t *testing.T) {
	prepareFake(t)
	dir := t.TempDir()
	writeFile(t, dir, "rec/broken.json", `{not json`)
	r := runFakeKeepEnv(t, dir, "serve", "--port", strconv.Itoa(freeServerPort(t)), "--tx-offline", "--tx-dir", "rec")
	if r.err == nil || !strings.Contains(r.err.Error(), "broken.json is corrupt") {
		t.Errorf("a corrupt recording must stop serve before it starts: %v", r.err)
	}
}

// Under --tx-offline the requested port belongs to the proxy, so it has to be
// a real port, and one that is taken fails the start instead of leaving the
// validator running without its proxy.
func TestFakeServe_TxOfflinePortProblems(t *testing.T) {
	dir := t.TempDir()
	warmServeRecording(t, dir)

	r := runFakeKeepEnv(t, dir, "serve", "--port", "0", "--tx-offline")
	var ee *exitErr
	if !errors.As(r.err, &ee) || ee.code != 2 || !strings.Contains(r.err.Error(), "server port must be positive") {
		t.Errorf("--port 0 with --tx-offline: want exit 2, got %v", r.err)
	}

	// All interfaces, as the proxy binds, so the clash holds on every OS.
	taken, err := net.Listen("tcp", ":0") //nolint:gosec // G102: must clash with the proxy, which binds every interface
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = taken.Close() }()
	port := taken.Addr().(*net.TCPAddr).Port
	r = runFakeKeepEnv(t, dir, "serve", "--port", strconv.Itoa(port), "--tx-offline")
	if !errors.As(r.err, &ee) || ee.code != 2 {
		t.Errorf("a taken proxy port: want exit 2, got %v\n%s", r.err, r.stderr)
	}
	if strings.Contains(r.stdout, "server ready") {
		t.Errorf("a server without its proxy is not ready:\n%s", r.stdout)
	}
}

// Credentials for a named https server reach the validator as a settings file,
// which is gone once serve returns.
func TestFakeServe_CredentialsBecomeASettingsFile(t *testing.T) {
	prepareFake(t)
	t.Setenv("FHIRLINT_TX_TOKEN", "s3cret")
	log := filepath.Join(t.TempDir(), "calls.jsonl")
	t.Setenv(fakeJavaLogEnv, log)
	base, done := startFakeServe(t, t.TempDir(), "--terminology-server", "https://tx.example.org/r4")

	calls := fakeJavaCalls(t, log)
	vals := argAfter(calls[len(calls)-1], "-fhir-settings")
	if len(vals) != 1 {
		t.Fatalf("want one -fhir-settings, got %v", calls[len(calls)-1])
	}
	settings := vals[0]
	data, err := os.ReadFile(settings) //nolint:gosec // the path serve passed to the fake
	if err != nil || !strings.Contains(string(data), "https://tx.example.org/r4") || !strings.Contains(string(data), "s3cret") {
		t.Errorf("the settings file must name the server and carry the token: %v\n%s", err, data)
	}

	_ = post(t, base+"/stop", "")
	if r := waitServe(t, done); r.err != nil {
		t.Fatalf("serve: %v\n%s", r.err, r.stderr)
	}
	if _, err := os.Stat(settings); !os.IsNotExist(err) {
		t.Errorf("the settings file holding the token must be removed at shutdown: %v", err)
	}
}
