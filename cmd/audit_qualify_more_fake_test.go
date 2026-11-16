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

// audit without an installed JAR and with advisories in JSON; qualify with a
// JAR it cannot prepare and with a named terminology server (#690).

func TestFakeAudit_NoJARInstalled(t *testing.T) {
	prepareFake(t)
	fakeGitHubAPI(t, "7.0.6", "[]", 0)
	p, err := cache.ValidatorVersionPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	r := runFakeKeepEnv(t, t.TempDir(), "audit")
	if code := auditExit(t, r); code != exitJARIssue || !strings.Contains(r.stderr, "JAR not installed — run: fhirlint update") {
		t.Errorf("no JAR is a JAR issue: exit %d\n%s", code, r.stderr)
	}
}

func TestFakeAudit_AffectingAdvisoriesInJSON(t *testing.T) {
	prepareFake(t)
	fakeGitHubAPI(t, "7.0.6", "["+affecting+","+notAffecting+"]", 0)
	r := runFakeKeepEnv(t, t.TempDir(), "audit", "--format", "json")
	if r.err != nil {
		t.Fatalf("--format json exits 0: %v", r.err)
	}
	var got struct {
		AdvisoryCount int `json:"advisoryCount"`
		Affecting     []struct {
			GHSAID, Severity, URL string
		} `json:"affecting"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &got); err != nil {
		t.Fatalf("%v\n%s", err, r.stdout)
	}
	if got.AdvisoryCount != 2 || len(got.Affecting) != 1 || got.Affecting[0].GHSAID != "GHSA-aaaa-bbbb-cccc" || got.Affecting[0].Severity != "high" {
		t.Errorf("only the affecting advisory is listed, out of two: %+v\n%s", got, r.stdout)
	}
}

func TestFakeQualify_JARAndTerminology(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "suite/ok.json", cleanPatient)
	writeFile(t, dir, "suite/ok.expected.json", `{"valid": true}`)

	prepareFake(t)
	t.Setenv("FHIRLINT_JAR", filepath.Join(t.TempDir(), "absent.jar"))
	r := runFakeKeepEnv(t, dir, "qualify", "--test-suite", "suite")
	var ee *exitErr
	if !errors.As(r.err, &ee) || ee.code != 2 || !strings.Contains(r.err.Error(), "preparing validator JAR") {
		t.Errorf("a JAR that cannot be prepared exits 2: %v", r.err)
	}

	prepareFake(t)
	r = runFakeKeepEnv(t, dir, "qualify", "--test-suite", "suite", "--format", "json", "--terminology-server", "https://tx.example.org/r4")
	if !strings.Contains(r.stdout, `"terminology": "https://tx.example.org/r4"`) {
		t.Errorf("the report names the terminology server it ran against:\n%s", r.stdout)
	}
}
