package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
