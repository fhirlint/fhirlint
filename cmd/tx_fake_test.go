// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// Recording terminology with tx warm and replaying it with --tx-offline, end
// to end against the fake validator, which asks the terminology server about
// every FAKE_TX:<code> in its input (#612).

// fakeTx stands in for a terminology server that knows the code "known".
func fakeTxServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/fhir+json")
		known := r.URL.Query().Get("code") == "known"
		_, _ = fmt.Fprintf(w, `{"resourceType":"Parameters","parameter":[{"name":"result","valueBoolean":%v}]}`, known) //nolint:gosec // only a boolean is written, nothing from the request
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

const txPatient = `{"resourceType":"Patient","id":"p1","a":"FAKE_TX:known","b":"FAKE_TX:unknown"}`

func TestFakeTx_RecordThenReplayOffline(t *testing.T) {
	srv, calls := fakeTxServer(t)
	dir := t.TempDir()
	writeFile(t, dir, "fhir/p.json", txPatient)

	r := runFake(t, dir, "tx", "warm", "--terminology-server", srv.URL+"/r4", "fhir")
	if r.err != nil {
		t.Fatalf("tx warm: %v\n%s", r.err, r.stderr)
	}
	if calls.Load() != 2 {
		t.Errorf("the recording run must reach the server once per code, got %d calls", calls.Load())
	}
	entries, _ := os.ReadDir(filepath.Join(dir, ".fhirlint-tx"))
	if len(entries) == 0 {
		t.Fatalf("no recording written:\n%s", r.stderr)
	}

	// The server is gone; the replay answers alone, with the same findings.
	srv.Close()
	r = runFakeKeepEnv(t, dir, "validate", "--no-color", "--tx-offline", "fhir")
	mustFail(t, r)
	if !strings.Contains(r.stdout, "Unknown code unknown") || strings.Contains(r.stdout, "unavailable") {
		t.Errorf("the replay must give the recorded answers:\n%s\n%s", r.stdout, r.stderr)
	}
	if strings.Contains(r.stdout, "Unknown code known") {
		t.Errorf("the known code is valid in the recording too:\n%s", r.stdout)
	}
}

func TestFakeTx_AMissFailsTheRun(t *testing.T) {
	srv, _ := fakeTxServer(t)
	dir := t.TempDir()
	writeFile(t, dir, "fhir/p.json", `{"resourceType":"Patient","id":"p1","a":"FAKE_TX:known"}`)
	if r := runFake(t, dir, "tx", "warm", "--terminology-server", srv.URL+"/r4", "fhir"); r.err != nil {
		t.Fatalf("tx warm: %v\n%s", r.err, r.stderr)
	}
	mustPass(t, runFakeKeepEnv(t, dir, "validate", "--tx-offline", "fhir"))

	// A code the recording never saw: the replay can't answer, and the run
	// fails instead of passing with the check skipped.
	writeFile(t, dir, "fhir/p.json", `{"resourceType":"Patient","id":"p1","a":"FAKE_TX:other"}`)
	r := runFakeKeepEnv(t, dir, "validate", "--tx-offline", "fhir")
	if r.err == nil || !strings.Contains(r.err.Error(), "not in the recording") || !strings.Contains(r.err.Error(), "tx warm") {
		t.Fatalf("a replay miss must fail and say how to re-record, got %v", r.err)
	}
}

func TestFakeTx_UsageErrors(t *testing.T) {
	srv, _ := fakeTxServer(t)
	dir := t.TempDir()
	writeFile(t, dir, "empty/notes.txt", "no FHIR here")
	if r := runFake(t, dir, "tx", "warm", "--terminology-server", srv.URL, "empty"); r.err == nil || !strings.Contains(r.err.Error(), "no FHIR resources") {
		t.Errorf("tx warm over nothing: %v", r.err)
	}

	writeFile(t, dir, "fhir/p.json", txPatient)
	t.Setenv("FHIRLINT_TX_TOKEN", "s3cret")
	r := runFakeKeepEnv(t, dir, "tx", "warm", "--terminology-server", srv.URL, "fhir")
	if r.err == nil || !strings.Contains(r.err.Error(), "over plain HTTP") {
		t.Errorf("credentials must not go to an http:// server, got %v", r.err)
	}
	t.Setenv("FHIRLINT_TX_TOKEN", "")

	if r := runFakeKeepEnv(t, dir, "validate", "--tx-offline", "fhir"); r.err == nil || !strings.Contains(r.err.Error(), "tx warm") {
		t.Errorf("--tx-offline without a recording must say how to make one, got %v", r.err)
	}
}

// A miss still fails the run, but after every report is written (#659): one
// file's missing terminology answer must not cost the findings of the others.
func TestFakeTx_AMissKeepsTheReport(t *testing.T) {
	srv, _ := fakeTxServer(t)
	dir := t.TempDir()
	writeFile(t, dir, "fhir/a.json", `{"resourceType":"Patient","id":"a","x":"FAKE_TX:known"}`)
	writeFile(t, dir, "fhir/b.json", errPatient)
	if r := runFake(t, dir, "tx", "warm", "--terminology-server", srv.URL+"/r4", "fhir"); r.err != nil {
		t.Fatalf("tx warm: %v\n%s", r.err, r.stderr)
	}
	writeFile(t, dir, "fhir/a.json", `{"resourceType":"Patient","id":"a","x":"FAKE_TX:other"}`)

	r := runFakeKeepEnv(t, dir, "validate", "--tx-offline", "--format", "json", "--output", "rep.json", "--generate-baseline", "base.json", "fhir")
	if r.err == nil || !strings.Contains(r.err.Error(), "not in the recording") {
		t.Fatalf("the miss must still fail the run, got %v", r.err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "rep.json")) //nolint:gosec // test output
	if err != nil || !strings.Contains(string(data), "a.json") || !strings.Contains(string(data), "FAKE_ERROR_ID") {
		t.Errorf("the report must be written with every file's findings: %v\n%s", err, data)
	}
	if _, err := os.Stat(filepath.Join(dir, "base.json")); !os.IsNotExist(err) {
		t.Errorf("no baseline from an incomplete recording: %v", err)
	}
	if !strings.Contains(r.stderr, "Not writing the baseline") {
		t.Errorf("say why the baseline is missing:\n%s", r.stderr)
	}
}

// A replay's findings name the server the recording stands for, not the
// local replay server, and read the same in every replay (#663).
func TestFakeTx_FindingsNameTheRecordedServer(t *testing.T) {
	srv, _ := fakeTxServer(t)
	dir := t.TempDir()
	writeFile(t, dir, "fhir/p.json", `{"resourceType":"Patient","id":"p1","a":"FAKE_TX:known"}`)
	if r := runFake(t, dir, "tx", "warm", "--terminology-server", srv.URL+"/r4", "fhir"); r.err != nil {
		t.Fatalf("tx warm: %v\n%s", r.err, r.stderr)
	}
	// A code the recording lacks: the replay answers 404 and the validator
	// quotes the server it asked.
	writeFile(t, dir, "fhir/p.json", `{"resourceType":"Patient","id":"p1","a":"FAKE_TX:other"}`)
	var reports []string
	for _, out := range []string{"a.json", "b.json"} {
		_ = runFakeKeepEnv(t, dir, "validate", "--tx-offline", "--format", "json", "--output", out, "fhir")
		data, err := os.ReadFile(filepath.Join(dir, out)) //nolint:gosec // test output
		if err != nil {
			t.Fatal(err)
		}
		reports = append(reports, string(data))
	}
	if !strings.Contains(reports[0], "Error from "+srv.URL+"/r4") {
		t.Errorf("the finding must name the recorded server %s:\n%s", srv.URL+"/r4", reports[0])
	}
	if strip := func(s string) string { return s[strings.Index(s, `"files"`):] }; strip(reports[0]) != strip(reports[1]) {
		t.Errorf("two replays must give the same findings:\n%s\n%s", reports[0], reports[1])
	}
}
