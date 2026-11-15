// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"os/exec"
	"strings"
	"testing"
)

// More of validate's preprocessing (#670): XML, NDJSON among other inputs,
// bundle entry labels, and --check-references over files --since left out.

func TestFakeValidate_XMLExtractAndBrokenXML(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "env.xml", `<Envelope><data><Patient xmlns="http://hl7.org/fhir"><id value="p"/><note value="FAKE_ERROR"/></Patient></data></Envelope>`)
	writeFile(t, dir, "broken.xml", `<Patient xmlns="http://hl7.org/fhir"><id value="p"></Patient>`)
	mustFail(t, runFake(t, dir, "validate", "--extract", "$.data", "env.xml"))
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--extract", "$.nothing", "env.xml"}, "nothing"},
		{[]string{"--ignore", "$.note", "broken.xml"}, "--ignore on XML"},
	} {
		r := runFakeKeepEnv(t, dir, append([]string{"validate"}, tc.args...)...)
		if r.err == nil || !strings.Contains(r.err.Error(), tc.want) {
			t.Errorf("%q: want %q, got %v", tc.args, tc.want, r.err)
		}
	}
}

func TestFakeValidate_NDJSONAmongOtherInputs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "bulk.ndjson", cleanPatient+"\n"+errPatient+"\n")
	writeFile(t, dir, "more.ndjson", cleanPatient+"\n")
	writeFile(t, dir, "empty.ndjson", "\n\n")
	writeFile(t, dir, "p.json", cleanPatient)

	r := runFake(t, dir, "validate", "--no-color", "bulk.ndjson", "p.json")
	mustFail(t, r)
	if !strings.Contains(r.stdout, "Files: 3") {
		t.Errorf("each NDJSON line and the JSON file are a file each:\n%s", r.stdout)
	}
	r = runFakeKeepEnv(t, dir, "validate", "--no-color", "bulk.ndjson", "more.ndjson")
	mustFail(t, r)
	if !strings.Contains(r.stdout, "Files: 3") {
		t.Errorf("only NDJSON inputs:\n%s", r.stdout)
	}
	// --ignore applies to every line.
	mustPass(t, runFakeKeepEnv(t, dir, "validate", "--ignore", "$.note", "bulk.ndjson"))
	r = runFakeKeepEnv(t, dir, "validate", "--no-color", "empty.ndjson")
	mustPass(t, r)
	if !strings.Contains(r.stdout, "Files: 0") {
		t.Errorf("an NDJSON file without lines validates nothing:\n%s", r.stdout)
	}
}

func TestFakeValidate_BundleEntryLabels(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "b.json", `{"resourceType":"Bundle","type":"collection","entry":[
	  {"resource":{"resourceType":"Patient","id":"p1"}},
	  {"resource":{"resourceType":"Patient"}},
	  {"resource":{"note":"no type"}}]}`)
	r := runFake(t, dir, "validate", "--no-color", "--bundle-entries", "b.json")
	for _, want := range []string{"b.json → entry[0] (Patient/p1)", "b.json → entry[1] (Patient)", "b.json → entry[2]"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("want the label %q:\n%s", want, r.stdout)
		}
	}
}

// Files --since leaves out are still indexed for --check-references, so a
// reference into them resolves.
func TestFakeValidate_CheckReferencesIndexesFilesSinceLeftOut(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("needs git")
	}
	dir := t.TempDir()
	writeFile(t, dir, "fhir/p.json", `{"resourceType":"Patient","id":"p1"}`)
	writeFile(t, dir, "fhir/o.json", `{"resourceType":"Observation","id":"o1","status":"final","code":{"text":"x"},"subject":{"reference":"Patient/p1"}}`)
	git := func(args ...string) {
		t.Helper()
		c := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.org", "-c", "commit.gpgsign=false"}, args...)...) //nolint:gosec // git with the test's own fixed arguments
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("add", ".")
	git("commit", "-q", "-m", "init")
	writeFile(t, dir, "fhir/o.json", `{"resourceType":"Observation","id":"o1","status":"amended","code":{"text":"x"},"subject":{"reference":"Patient/p1"}}`)

	r := runFake(t, dir, "validate", "--no-color", "--since", "HEAD", "--check-references", "fhir")
	if r.err != nil {
		t.Fatalf("%v\n%s\n%s", r.err, r.stdout, r.stderr)
	}
	if !strings.Contains(r.stdout, "Files: 1") || strings.Contains(r.stdout, "Patient/p1") {
		t.Errorf("only o.json is validated, and its reference to the unchanged p.json resolves:\n%s", r.stdout)
	}
}
