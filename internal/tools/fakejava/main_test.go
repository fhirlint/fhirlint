// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestJavaVersion(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"-version"}, &out, &errb, env(nil)); code != 0 || !strings.Contains(errb.String(), `version "21`) {
		t.Fatalf("java -version: code %d, stderr %q", code, errb.String())
	}
}

func TestSingleInputGetsAnOperationOutcome(t *testing.T) {
	dir := t.TempDir()
	in := write(t, dir, "p.json", `{"note":"FAKE_ERROR FAKE_WARNING"}`)
	out := filepath.Join(dir, "out.json")
	var so, se bytes.Buffer
	if code := run([]string{"-Duser.home=/x", "-jar", "v.jar", in, "-version", "4.0.1", "-output", out}, &so, &se, env(nil)); code != 0 {
		t.Fatalf("code %d: %s", code, se.String())
	}
	var oo struct {
		ResourceType string `json:"resourceType"`
		Issue        []struct {
			Severity string `json:"severity"`
		} `json:"issue"`
	}
	data, _ := os.ReadFile(out) //nolint:gosec // test output
	if err := json.Unmarshal(data, &oo); err != nil || oo.ResourceType != "OperationOutcome" || len(oo.Issue) != 2 {
		t.Fatalf("output: %s (%v)", data, err)
	}
	if oo.Issue[0].Severity != "error" || oo.Issue[1].Severity != "warning" {
		t.Errorf("severities: %+v", oo.Issue)
	}
}

func TestSeveralInputsGetABundleInOrder(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "a.json", `{}`)
	b := write(t, dir, "b.json", `{"FAKE_ERROR":1}`)
	out := filepath.Join(dir, "out.json")
	log := filepath.Join(dir, "calls.jsonl")
	var so, se bytes.Buffer
	args := []string{"-jar", "v.jar", a, b, "-version", "4.0.1", "-output", out}
	if code := run(args, &so, &se, env(map[string]string{LogEnv: log})); code != 0 {
		t.Fatalf("code %d: %s", code, se.String())
	}
	data, _ := os.ReadFile(out) //nolint:gosec // test output
	if !strings.Contains(string(data), `"resourceType":"Bundle"`) || strings.Index(string(data), "All OK") > strings.Index(string(data), "Fake error found") {
		t.Errorf("want a Bundle with a's outcome before b's: %s", data)
	}
	logged, _ := os.ReadFile(log) //nolint:gosec // test output
	if !strings.Contains(string(logged), `"-output"`) {
		t.Errorf("the call must be logged: %s", logged)
	}
}

func TestCrashRefuseAndBadCalls(t *testing.T) {
	dir := t.TempDir()
	crash := write(t, dir, "c.json", `FAKE_CRASH`)
	out := filepath.Join(dir, "out.json")
	var so, se bytes.Buffer
	if code := run([]string{"-jar", "v.jar", crash, "-version", "4.0.1", "-output", out}, &so, &se, env(nil)); code != 1 || !strings.Contains(so.String(), "Exception") {
		t.Errorf("crash: code %d, stdout %q", code, so.String())
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("a crash must not write output")
	}
	ok := write(t, dir, "ok.json", `{}`)
	if code := run([]string{"-jar", "v.jar", ok, "-version", "4.0.1", "-output", out}, &so, &se, env(map[string]string{RefuseEnv: "1"})); code != 3 {
		t.Errorf("refuse: code %d", code)
	}
	for _, args := range [][]string{
		{"-jar", "v.jar", ok, "-version", "4.0.1"},
		{"-jar", "v.jar", "-version", "4.0.1", "-output", out},
		{"-jar", "v.jar", filepath.Join(dir, "missing.json"), "-version", "4.0.1", "-output", out},
		{"-jar", "v.jar", ok, "-version", "4.0.1", "-output", filepath.Join(dir, "no", "such", "dir", "o.json")},
	} {
		if code := run(args, &so, &se, env(nil)); code != 2 {
			t.Errorf("%q: code %d, want 2", args, code)
		}
	}
	if code := run([]string{"-jar", "v.jar", ok, "-version", "4.0.1", "-output", out}, &so, &se, env(map[string]string{LogEnv: filepath.Join(dir, "no", "log")})); code != 2 {
		t.Errorf("an unwritable log must fail, got %d", code)
	}
}

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	return strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
}

func TestServerMode(t *testing.T) {
	port := freePort(t)
	pr, pw := io.Pipe()
	done := make(chan int, 1)
	go func() {
		done <- run([]string{"-jar", "v.jar", "server", port, "-version", "4.0.1"}, pw, io.Discard, env(nil))
		_ = pw.Close()
	}()
	ready := make(chan string, 1)
	go func() {
		buf := make([]byte, 256)
		n, _ := pr.Read(buf)
		ready <- string(buf[:n])
		_, _ = io.Copy(io.Discard, pr)
	}()
	select {
	case line := <-ready:
		if !strings.Contains(line, serverReady) {
			t.Fatalf("first output %q, want the ready banner", line)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no ready banner")
	}
	base := "http://127.0.0.1:" + port
	resp, err := http.Post(base+"/validateResource", "application/fhir+json", strings.NewReader(`{"x":"FAKE_ERROR"}`)) //nolint:gosec // loopback test server
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), "FAKE_ERROR_ID") {
		t.Errorf("validateResource: %s", body)
	}
	resp, err = http.Post(base+"/validateResource", "application/fhir+json", strings.NewReader(`FAKE_CRASH`)) //nolint:gosec // loopback test server
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("a crash answers 500, got %d", resp.StatusCode)
	}
	resp, err = http.Post(base+"/stop", "", nil) //nolint:gosec // loopback test server
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("exit %d after /stop", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not exit after /stop")
	}
}

func TestServerModeFailures(t *testing.T) {
	var so, se bytes.Buffer
	if code := run([]string{"-jar", "v.jar", "server", "1", "-version", "4.0.1"}, &so, &se, env(map[string]string{ServerFailEnv: "1"})); code != 1 {
		t.Errorf("FAKEJAVA_SERVER_FAIL: exit %d", code)
	}
	if code := run([]string{"-jar", "v.jar", "server", "not-a-port", "-version", "4.0.1"}, &so, &se, env(nil)); code != 2 {
		t.Errorf("a bad port: exit %d", code)
	}
}

func TestTerminologyMarker(t *testing.T) {
	var asked []string
	tx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		asked = append(asked, r.URL.Path+"?code="+code)
		switch code {
		case "down":
			http.Error(w, "no", http.StatusServiceUnavailable)
		default:
			known := code == "known"
			_, _ = fmt.Fprintf(w, `{"resourceType":"Parameters","parameter":[{"name":"result","valueBoolean":%v}]}`, known) //nolint:gosec // only a boolean is written, nothing from the request
		}
	}))
	defer tx.Close()
	dir := t.TempDir()
	in := write(t, dir, "p.json", `{"a":"FAKE_TX:known","b":"FAKE_TX:unknown","c":"FAKE_TX:down"}`)
	out := filepath.Join(dir, "out.json")
	var so, se bytes.Buffer
	if code := run([]string{"-jar", "v.jar", in, "-version", "4.0.1", "-output", out, "-tx", tx.URL + "/r4/"}, &so, &se, env(nil)); code != 0 {
		t.Fatalf("code %d: %s", code, se.String())
	}
	data, _ := os.ReadFile(out) //nolint:gosec // test output
	s := string(data)
	if !strings.Contains(s, "FAKE_TX_UNKNOWN") || !strings.Contains(s, "FAKE_TX_UNAVAILABLE") || strings.Contains(s, "All OK") || strings.Count(s, `"severity"`) != 2 {
		t.Errorf("want one unknown and one unavailable code, nothing for the known one: %s", s)
	}
	if len(asked) != 3 || asked[0] != "/r4/CodeSystem/$validate-code?code=known" {
		t.Errorf("requests: %v", asked)
	}
	// -tx n/a, as with --no-terminology-server: nothing is asked.
	asked = nil
	if code := run([]string{"-jar", "v.jar", in, "-version", "4.0.1", "-output", out, "-tx", "n/a"}, &so, &se, env(nil)); code != 0 || len(asked) != 0 {
		t.Errorf("-tx n/a must not reach a server: code %d, %v", code, asked)
	}
}
