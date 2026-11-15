// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"strings"
	"testing"
)

// The corners of --ignore, --extract-each and --bundle-entries, end to end
// against the fake validator (#616).

func TestFakeIgnore_NestedPathsAndMissingOnes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "p.json", `{"resourceType":"Patient","name":[{"family":"FAKE_ERROR"}],"contact":[{"note":"FAKE_WARNING"}]}`)
	mustFail(t, runFake(t, dir, "validate", "p.json"))

	// Both markers removed by their nested paths; a path that isn't there is
	// left alone rather than failing the run.
	r := runFake(t, dir, "validate", "--no-color", "--fail-on", "warning",
		"--ignore", "$.name[0].family", "--ignore", "$.contact[0].note", "--ignore", "$.nothing.here", "p.json")
	mustPass(t, r)
	if strings.Contains(r.stdout, "Fake") {
		t.Errorf("--ignore must remove nested fields:\n%s", r.stdout)
	}
}

func TestFakeIgnore_XMLAndExtractOnXML(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "p.xml", `<Bundle xmlns="http://hl7.org/fhir"><entry><resource><Patient><note value="FAKE_ERROR"/></Patient></resource></entry></Bundle>`)
	mustFail(t, runFake(t, dir, "validate", "--extract", "$.entry[0].resource", "p.xml"))
	mustPass(t, runFake(t, dir, "validate", "--ignore", "$.entry[0].resource.Patient.note", "p.xml"))
}

func TestFakeExtractEach_Errors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "list.json", `{"items":[],"one":{"resourceType":"Patient"}}`)
	for path, want := range map[string]string{
		"$.missing": "not found in input",
		"$.one":     "is not an array",
		"$.items":   "is empty",
	} {
		r := runFake(t, dir, "validate", "--extract-each", path, "list.json")
		if r.err == nil || !strings.Contains(r.err.Error(), want) {
			t.Errorf("--extract-each %s: want %q, got %v", path, want, r.err)
		}
	}
}

func TestFakeExtractEach_LabelsElementsByTypeAndID(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "list.json", `{"items":[{"resourceType":"Patient","id":"a1","note":"FAKE_ERROR"},{"resourceType":"Observation"}]}`)
	r := runFake(t, dir, "validate", "--no-color", "--extract-each", "$.items", "list.json")
	mustFail(t, r)
	if !strings.Contains(r.stdout, "Patient/a1") {
		t.Errorf("an element is labelled by its type and id:\n%s", r.stdout)
	}
}

func TestFakeBundleEntries_SkipsEmptyEntriesAndNonBundles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "b.json", `{"resourceType":"Bundle","type":"collection","entry":[{"fullUrl":"urn:x"},{"resource":null},{"resource":{"resourceType":"Patient","id":"p9","note":"FAKE_ERROR"}}]}`)
	writeFile(t, dir, "p.json", cleanPatient)
	r := runFake(t, dir, "validate", "--no-color", "--bundle-entries", "b.json", "p.json")
	mustFail(t, r)
	// The Bundle, its one real entry and the Patient: entries without a
	// resource are skipped, and a non-Bundle file is validated as itself.
	if !strings.Contains(r.stdout, "Files: 3") || !strings.Contains(r.stdout, "Patient/p9") {
		t.Errorf("want the Bundle, its entry and the Patient:\n%s", r.stdout)
	}
}
