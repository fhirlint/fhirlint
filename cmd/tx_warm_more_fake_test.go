// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tx warm's setup and failure paths against the fake validator (#686). None
// of them reaches a terminology server: the inputs carry no FAKE_TX marker.

// Without --terminology-server and with a validator that has registry routing
// (the fake is 7.0.5), the recording keeps the routing a live run has.
func TestFakeTxWarm_RecordsWithTheRegistry(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "fhir/p.json", cleanPatient)
	log := filepath.Join(t.TempDir(), "calls.jsonl")
	prepareFake(t)
	t.Setenv(fakeJavaLogEnv, log)
	r := runFakeKeepEnv(t, dir, "tx", "warm", "fhir")
	if r.err != nil {
		t.Fatalf("tx warm: %v\n%s", r.err, r.stderr)
	}
	if !strings.Contains(r.stderr, "Recording with the tx registry") {
		t.Errorf("say the recording keeps the registry routing:\n%s", r.stderr)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".fhirlint-tx", "manifest.json")) //nolint:gosec // test output
	if err != nil {
		t.Fatal(err)
	}
	var m struct{ Upstream, Routing, Ecosystem string }
	if err := json.Unmarshal(data, &m); err != nil || m.Routing != "registry" || m.Ecosystem == "" || !strings.HasPrefix(m.Upstream, "https://tx.fhir.org") {
		t.Errorf("manifest: %+v %v", m, err)
	}
	call := fakeJavaCalls(t, log)[0]
	tx := argAfter(call, "-tx")
	if len(tx) == 0 || !strings.HasPrefix(tx[0], "http://127.0.0.1:") || !strings.HasSuffix(tx[0], "/r4") {
		t.Errorf("-tx is the recorder plus the upstream's path: %v", call)
	}
	if eco := argAfter(call, "-ecosystem"); len(eco) == 0 || !strings.HasSuffix(eco[0], "/tx-reg") {
		t.Errorf("the registry goes through the recorder too: %v", call)
	}
}

// Credentials go on the recorder's own requests to an explicit https server;
// two at once are refused.
func TestFakeTxWarm_Credentials(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "fhir/p.json", cleanPatient)
	prepareFake(t)
	t.Setenv("FHIRLINT_TX_TOKEN", "s3cret")
	r := runFakeKeepEnv(t, dir, "tx", "warm", "--terminology-server", "https://tx.example.org/r4", "fhir")
	if r.err != nil || !strings.Contains(r.stderr, "Authenticating to https://tx.example.org/r4") || strings.Contains(r.stderr, "s3cret") {
		t.Errorf("say it authenticates, without the secret: %v\n%s", r.err, r.stderr)
	}
	t.Setenv("FHIRLINT_TX_APIKEY", "k3y")
	if r := runFakeKeepEnv(t, dir, "tx", "warm", "--terminology-server", "https://tx.example.org/r4", "fhir"); r.err == nil || !strings.Contains(r.err.Error(), "pick one") {
		t.Errorf("two credentials at once: %v", r.err)
	}
}

func TestFakeTxWarm_Failures(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "fhir/p.json", `{"resourceType":"Patient","note":"FAKE_CRASH_LOADING"}`)
	prepareFake(t)
	if r := runFakeKeepEnv(t, dir, "tx", "warm", "--terminology-server", "https://tx.example.org/r4", "fhir"); r.err == nil || !strings.Contains(r.err.Error(), "recording run failed") {
		t.Errorf("a validator that fails fails the recording: %v", r.err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".fhirlint-tx", "manifest.json")); !os.IsNotExist(err) {
		t.Errorf("a failed recording writes no manifest: %v", err)
	}
	if r := runFakeKeepEnv(t, dir, "tx", "warm", "missing"); r.err == nil {
		t.Error("an input that doesn't exist must fail")
	}
	t.Setenv("PATH", t.TempDir())
	if r := runFakeKeepEnv(t, dir, "tx", "warm", "fhir"); r.err == nil || !strings.Contains(r.err.Error(), "java not found") {
		t.Errorf("no java: %v", r.err)
	}
}
