// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package coverage_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/coverage"
)

// What LoadResources reads, skips with a reason, and passes over (#631).
func TestLoadResources_WhatIsReadAndWhatIsSkipped(t *testing.T) {
	root := t.TempDir()
	put := func(name, body string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	put("b.json", `{"resourceType":"Patient","id":"b","meta":{"profile":["https://example.org/p|1.0",""]}}`)
	put("a.json", `{"resourceType":"Observation","id":"a"}`)
	put("bulk.ndjson", "{\"resourceType\":\"Patient\",\"id\":\"n1\"}\n\nnot json\n{\"id\":\"no-type\"}\n{\"resourceType\":\"Patient\",\"id\":\"n2\"}\n")
	put("broken.json", `{"resourceType":`)
	put("p.xml", `<Patient xmlns="http://hl7.org/fhir"/>`)
	put("package.json", `{"name":"not-fhir"}`) // no resourceType: not reported
	put("notes.txt", "not an input")
	put(".git/c.json", `{"resourceType":"Patient","id":"hidden"}`)

	rs, skipped, err := coverage.LoadResources(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, r := range rs {
		ids = append(ids, filepath.Base(r.Path)+":"+r.Body["id"].(string))
	}
	// Sorted by path; NDJSON lines that aren't resources are passed over.
	if got := strings.Join(ids, " "); got != "a.json:a b.json:b bulk.ndjson:n1 bulk.ndjson:n2" {
		t.Errorf("resources: %s", got)
	}
	if p := rs[1].Profiles; len(p) != 1 || p[0] != "https://example.org/p|1.0" {
		t.Errorf("meta.profile, empty entries dropped: %q", p)
	}

	reasons := map[string]string{}
	for _, s := range skipped {
		reasons[filepath.Base(s.Path)] = s.Reason
	}
	if len(reasons) != 2 || reasons["broken.json"] == "" || !strings.Contains(reasons["p.xml"], "XML input is not supported") {
		t.Errorf("skipped: %v", reasons)
	}

	// One file as the input.
	rs, _, err = coverage.LoadResources(filepath.Join(root, "a.json"), nil)
	if err != nil || len(rs) != 1 {
		t.Errorf("a single file: %d resources, %v", len(rs), err)
	}
	if _, _, err := coverage.LoadResources(filepath.Join(root, "missing"), nil); err == nil {
		t.Error("a missing input must be an error")
	}
}

// A meta.profile with a |version suffix claims the profile, and the report is
// named after the profile's name, id or URL, in that order.
func TestRun_VersionedProfileAndReportName(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{`"name":"Named","id":"the-id",`, "Named"},
		{`"id":"the-id",`, "the-id"},
		{``, "TestPatient"}, // the URL's last segment
	} {
		sd := strings.Replace(patientProfile, `"name": "TestPatient",
  "id": "TestPatient",`, tc.name, 1)
		reg, p := loadRegistry(t, sd)
		res := coverage.Resource{Type: "Patient", Profiles: []string{
			"https://example.org/StructureDefinition/TestPatient|1.2.0"}, Body: decode(t, richPatient)}
		rep := coverage.Run(reg, []*coverage.StructureDefinition{p}, []coverage.Resource{res}, coverage.Options{})
		if len(rep.Profiles) != 1 || rep.Profiles[0].Resources != 1 || rep.Unattributed != 0 {
			t.Fatalf("a versioned meta.profile must claim the profile: %+v", rep)
		}
		if rep.Profiles[0].Name != tc.want {
			t.Errorf("name = %q, want %q", rep.Profiles[0].Name, tc.want)
		}
	}
}

// Nothing measurable is no evidence of a gap: 100, not 0.
func TestPercent_NothingMeasurable(t *testing.T) {
	p := coverage.ProfileReport{Elements: []coverage.ElementReport{{ID: "Patient.x", Unresolved: true}}}
	if p.Percent() != 100 {
		t.Errorf("Percent = %v", p.Percent())
	}
}
