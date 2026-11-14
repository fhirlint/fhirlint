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

// fhirlint coverage end to end, against a small IG in the FHIR package cache
// under the temp HOME that runFake sets (#614).

const coverageProfileURL = "https://example.org/StructureDefinition/CovPatient"

// cacheCoverageIG puts example.cov#1.0.0 into the package cache: one Patient
// profile with a snapshot and three must-support elements.
func cacheCoverageIG(t *testing.T) {
	t.Helper()
	dir := filepath.Join(os.Getenv("HOME"), ".fhir", "packages", "example.cov#1.0.0", "package")
	writeFile(t, dir, "package.json", `{"name":"example.cov","version":"1.0.0","fhirVersions":["4.0.1"]}`)
	writeFile(t, dir, "StructureDefinition-CovPatient.json", `{
  "resourceType": "StructureDefinition", "url": "`+coverageProfileURL+`",
  "name": "CovPatient", "id": "CovPatient", "type": "Patient", "kind": "resource",
  "derivation": "constraint", "baseDefinition": "http://hl7.org/fhir/StructureDefinition/Patient",
  "snapshot": {"element": [
    {"id": "Patient", "path": "Patient"},
    {"id": "Patient.identifier", "path": "Patient.identifier", "mustSupport": true},
    {"id": "Patient.birthDate", "path": "Patient.birthDate", "mustSupport": true},
    {"id": "Patient.gender", "path": "Patient.gender", "mustSupport": true}
  ]}
}`)
}

const (
	covClaimed   = `{"resourceType":"Patient","meta":{"profile":["` + coverageProfileURL + `"]},"identifier":[{"value":"1"}],"birthDate":"1990-01-01"}`
	covUnclaimed = `{"resourceType":"Patient","identifier":[{"value":"2"}]}`
)

func TestFakeCoverage_ReportsWhatIsNeverPopulated(t *testing.T) {
	prepareFake(t)
	cacheCoverageIG(t)
	dir := t.TempDir()
	writeFile(t, dir, "data/p1.json", covClaimed)
	writeFile(t, dir, "data/p2.json", covUnclaimed)

	r := runFakeKeepEnv(t, dir, "coverage", "--ig", "example.cov#1.0.0", "data")
	if r.err != nil {
		t.Fatalf("coverage: %v\n%s", r.err, r.stderr)
	}
	for _, want := range []string{"CovPatient  (Patient)", "must-support:   2/3  (67%)", "never populated:", "Patient.gender", "2 resource(s) scanned, 1 not attributed to any profile"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output lacks %q:\n%s", want, r.stdout)
		}
	}

	if r := runFakeKeepEnv(t, dir, "coverage", "--ig", "example.cov#1.0.0", "--min-coverage", "90", "data"); r.err == nil || !strings.Contains(r.err.Error(), "below the required 90%") {
		t.Errorf("--min-coverage 90 over 67%% must fail, got %v", r.err)
	}
	if r := runFakeKeepEnv(t, dir, "coverage", "--ig", "example.cov#1.0.0", "--min-coverage", "60", "data"); r.err != nil {
		t.Errorf("--min-coverage 60 over 67%% passes, got %v", r.err)
	}
}

func TestFakeCoverage_JSONAndAttributionByType(t *testing.T) {
	prepareFake(t)
	cacheCoverageIG(t)
	dir := t.TempDir()
	writeFile(t, dir, "data/p2.json", covUnclaimed)

	// Named with --profile, a resource without meta.profile is measured by type.
	r := runFakeKeepEnv(t, dir, "coverage", "--ig", "example.cov#1.0.0", "--profile", coverageProfileURL, "--format", "json", "--output", "cov.json", "data")
	if r.err != nil {
		t.Fatalf("coverage: %v\n%s", r.err, r.stderr)
	}
	data, err := os.ReadFile(filepath.Join(dir, "cov.json")) //nolint:gosec // test output
	if err != nil {
		t.Fatal(err)
	}
	var rep struct {
		Profiles []struct {
			Name       string  `json:"name"`
			ByType     int     `json:"byType"`
			Covered    int     `json:"covered"`
			Measurable int     `json:"measurable"`
			Percent    float64 `json:"percent"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, data)
	}
	if len(rep.Profiles) != 1 || rep.Profiles[0].ByType != 1 || rep.Profiles[0].Covered != 1 || rep.Profiles[0].Measurable != 3 || rep.Profiles[0].Percent != 33.3 {
		t.Errorf("report: %+v\n%s", rep.Profiles, data)
	}
}

func TestFakeCoverage_UsageErrors(t *testing.T) {
	prepareFake(t)
	cacheCoverageIG(t)
	dir := t.TempDir()
	writeFile(t, dir, "data/p1.json", covClaimed)
	writeFile(t, dir, "empty/notes.txt", "nothing")
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"coverage", "data"}, "--ig is required"},
		{[]string{"coverage", "--ig", "example.cov", "data"}, "must be name#version"},
		{[]string{"coverage", "--ig", "not.cached#9.9.9", "--offline", "data"}, "not.cached#9.9.9"},
		{[]string{"coverage", "--ig", "example.cov#1.0.0", "--profile", "https://example.org/nope", "data"}, "none of the named profiles"},
		{[]string{"coverage", "--ig", "example.cov#1.0.0", "empty"}, "no JSON or NDJSON resources"},
	} {
		r := runFakeKeepEnv(t, dir, tc.args...)
		if r.err == nil || !strings.Contains(r.err.Error(), tc.want) {
			t.Errorf("%q: want an error with %q, got %v", tc.args, tc.want, r.err)
		}
	}
}
