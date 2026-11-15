// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/cache"
)

// upgrade-check end to end (#629). It runs this binary's validate once per
// release: in a test that binary is the test binary, which runCLIEnv turns
// into the CLI. The releases are fake JARs placed where EnsureSideJAR keeps
// them, so nothing is downloaded, and the fake validator's *_IN markers make
// the two disagree.

// prepareUpgradeCheck readies the fake validator and caches side JARs for
// versions, returning the call log.
func prepareUpgradeCheck(t *testing.T, versions ...string) string {
	t.Helper()
	prepareFake(t)
	t.Setenv(runCLIEnv, "1")
	log := filepath.Join(t.TempDir(), "calls.jsonl")
	t.Setenv(fakeJavaLogEnv, log)
	dir, err := cache.Dir()
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range versions {
		side := filepath.Join(dir, "validators", v)
		if err := os.MkdirAll(side, 0o750); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(fakeJAR(t, v)) //nolint:gosec // test file
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, side, "validator_cli.jar", string(data))
	}
	return log
}

const upgradePatient = `{"resourceType":"Patient","id":"p1","a":"FAKE_ONLY_IN:7.0.6","b":"FAKE_GONE_IN:7.0.6","c":"FAKE_RENAMED_IN:7.0.6"}`

func TestFakeUpgradeCheck_ReportsTheDifference(t *testing.T) {
	prepareUpgradeCheck(t, "7.0.6")
	dir := t.TempDir()
	writeFile(t, dir, "p.json", upgradePatient)

	r := runFakeKeepEnv(t, dir, "upgrade-check", "--to", "7.0.6", "--no-terminology-server", "p.json")
	if !errors.Is(r.err, errValidationFailed) {
		t.Fatalf("a new error must fail the check, got %v\n%s\n%s", r.err, r.stdout, r.stderr)
	}
	for _, want := range []string{
		"p.json  +1 new  -1 resolved  ~1 renamed",
		"New (1):", "FAKE_ONLY_ID @ Patient.birthDate",
		"Resolved (1):", "FAKE_GONE_ID @ Patient.active",
		"FAKE_OLD_ID → FAKE_NEW_ID",
		"1 new error(s) with the new validator.",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("the report lacks %q:\n%s", want, r.stdout)
		}
	}
	if strings.Contains(r.stderr, "live terminology server") {
		t.Errorf("no terminology server, so no warning about one:\n%s", r.stderr)
	}
}

func TestFakeUpgradeCheck_JSONAndFrom(t *testing.T) {
	log := prepareUpgradeCheck(t, "7.0.4", "7.0.6")
	dir := t.TempDir()
	writeFile(t, dir, "p.json", upgradePatient)

	r := runFakeKeepEnv(t, dir, "upgrade-check", "--to=7.0.6", "--from", "7.0.4", "--json", "--no-terminology-server", "p.json")
	if !errors.Is(r.err, errValidationFailed) {
		t.Fatalf("got %v\n%s", r.err, r.stderr)
	}
	var rep upgradeReport
	if err := json.Unmarshal([]byte(r.stdout), &rep); err != nil {
		t.Fatalf("--json must print the report as JSON: %v\n%s", err, r.stdout)
	}
	if rep.NewErrors != 1 || len(rep.New) != 1 || len(rep.Resolved) != 1 || len(rep.Renamed) != 1 {
		t.Errorf("unexpected report: %+v", rep)
	}

	// Each side ran with its own release, in order.
	var jars []string
	for _, c := range fakeJavaCalls(t, log) {
		if j := argAfter(c, "-jar"); len(j) > 0 {
			jars = append(jars, j[0])
		}
	}
	if len(jars) != 2 || !strings.Contains(jars[0], filepath.Join("validators", "7.0.4")) || !strings.Contains(jars[1], filepath.Join("validators", "7.0.6")) {
		t.Errorf("from must run 7.0.4 and to 7.0.6, got %q", jars)
	}
}

func TestFakeUpgradeCheck_NoChange(t *testing.T) {
	prepareUpgradeCheck(t, "7.0.6")
	dir := t.TempDir()
	writeFile(t, dir, "p.json", cleanPatient)

	r := runFakeKeepEnv(t, dir, "upgrade-check", "--to", "7.0.6", "p.json")
	if r.err != nil {
		t.Fatalf("no change must pass, got %v\n%s", r.err, r.stderr)
	}
	if !strings.Contains(r.stdout, "No change in the findings.") {
		t.Errorf("report:\n%s", r.stdout)
	}
	if !strings.Contains(r.stderr, "live terminology server") {
		t.Errorf("a run against a live server must warn that it can drift:\n%s", r.stderr)
	}
}

func TestFakeUpgradeCheck_Failures(t *testing.T) {
	prepareUpgradeCheck(t, "7.0.6")
	dir := t.TempDir()
	writeFile(t, dir, "p.json", cleanPatient)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--to", "7.0"}, "not a validator release"},
		{[]string{"--to", "7.0.6", "--from", "latest"}, "not a validator release"},
		{[]string{"--to", "7.0.6", "--format", "json"}, "set by upgrade-check itself"},
		{[]string{"p.json"}, "--to <version> is required"},
		// validate fails before writing a report: the check can't compare.
		{[]string{"--to", "7.0.6", "--no-such-flag", "p.json"}, `"from" validator wrote no report`},
	} {
		r := runFakeKeepEnv(t, dir, append([]string{"upgrade-check"}, tc.args...)...)
		var ee *exitErr
		if !errors.As(r.err, &ee) || ee.code != 2 || !strings.Contains(r.err.Error(), tc.want) {
			t.Errorf("%q: want exit 2 with %q, got %v", tc.args, tc.want, r.err)
		}
	}
	if r := runFakeKeepEnv(t, dir, "upgrade-check", "--help"); r.err != nil || !strings.Contains(r.stdout, "upgrade-check --to <version>") {
		t.Errorf("--help: %v\n%s", r.err, r.stdout)
	}
}
