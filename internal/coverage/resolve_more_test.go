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

// Slices whose definition lives elsewhere (#631): in a datatype profile the
// element's type points at, or in the profile's own base.

// delegatingPatient constrains Patient.name through a HumanName profile and
// marks a slice of it that only that profile defines.
const delegatingPatient = `{
  "resourceType": "StructureDefinition", "url": "https://example.org/sd/DelegatingPatient",
  "name": "DelegatingPatient", "type": "Patient", "kind": "resource", "derivation": "constraint",
  "baseDefinition": "http://hl7.org/fhir/StructureDefinition/Patient",
  "differential": {"element": [
    {"id": "Patient.name", "path": "Patient.name", "mustSupport": true,
     "type": [{"code": "HumanName", "profile": ["https://example.org/sd/humanname-de"]}]},
    {"id": "Patient.name.family.extension:zusatz", "path": "Patient.name.family.extension",
     "sliceName": "zusatz", "mustSupport": true}
  ]}
}`

const humanNameDE = `{
  "resourceType": "StructureDefinition", "url": "https://example.org/sd/humanname-de",
  "name": "HumanNameDE", "type": "HumanName", "kind": "complex-type", "derivation": "constraint",
  "baseDefinition": "http://hl7.org/fhir/StructureDefinition/HumanName",
  "differential": {"element": [
    {"id": "HumanName.family.extension", "path": "HumanName.family.extension",
     "slicing": {"discriminator": [{"type": "value", "path": "url"}], "rules": "open"}},
    {"id": "HumanName.family.extension:zusatz", "path": "HumanName.family.extension", "sliceName": "zusatz",
     "type": [{"code": "Extension", "profile": ["http://fhir.de/StructureDefinition/humanname-namenszusatz"]}]}
  ]}
}`

const zusatzPatient = `{"resourceType": "Patient",
  "meta": {"profile": ["https://example.org/sd/DelegatingPatient"]},
  "name": [{"family": "Musterfrau",
    "_family": {"extension": [{"url": "http://fhir.de/StructureDefinition/humanname-namenszusatz", "valueString": "Gräfin"}]}}]}`

// loadPackage loads docs as one package and returns the registry and the
// profile with the given URL.
func loadPackage(t *testing.T, url string, docs ...string) (*coverage.Registry, *coverage.StructureDefinition) {
	t.Helper()
	dir := t.TempDir()
	for i, doc := range docs {
		if err := os.WriteFile(filepath.Join(dir, "sd"+string(rune('a'+i))+".json"), []byte(doc), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	reg := coverage.NewRegistry()
	if _, err := reg.LoadPackage(dir, "test#1.0.0"); err != nil {
		t.Fatal(err)
	}
	sd, ok := reg.Get(url)
	if !ok {
		t.Fatalf("%s not loaded", url)
	}
	return reg, sd
}

func element(t *testing.T, rep coverage.Report, id string) coverage.ElementReport {
	t.Helper()
	for _, p := range rep.Profiles {
		for _, e := range p.Elements {
			if e.ID == id {
				return e
			}
		}
	}
	t.Fatalf("no element %s in %+v", id, rep)
	return coverage.ElementReport{}
}

func TestRun_SliceDefinedInADatatypeProfile(t *testing.T) {
	const id = "Patient.name.family.extension:zusatz"
	res := coverage.Resource{Type: "Patient", Profiles: []string{"https://example.org/sd/DelegatingPatient"}, Body: decode(t, zusatzPatient)}

	reg, sd := loadPackage(t, "https://example.org/sd/DelegatingPatient", delegatingPatient, humanNameDE)
	e := element(t, coverage.Run(reg, []*coverage.StructureDefinition{sd}, []coverage.Resource{res}, coverage.Options{}), id)
	if e.Unresolved || !e.Populated {
		t.Errorf("with the HumanName profile loaded the slice is decided, and populated: %+v", e)
	}

	// Without it the slice can't be decided, and the report says which
	// package to install rather than a generic reason.
	reg, sd = loadPackage(t, "https://example.org/sd/DelegatingPatient", delegatingPatient)
	e = element(t, coverage.Run(reg, []*coverage.StructureDefinition{sd}, []coverage.Resource{res}, coverage.Options{}), id)
	if !e.Unresolved || !strings.Contains(e.Reason, "https://example.org/sd/humanname-de, which is not loaded") {
		t.Errorf("a missing delegate must be named: %+v", e)
	}
}

func TestRun_SliceWithoutADiscriminatorValue(t *testing.T) {
	sd := strings.Replace(delegatingPatient, `,
     "type": [{"code": "HumanName", "profile": ["https://example.org/sd/humanname-de"]}]`, "", 1)
	reg, p := loadPackage(t, "https://example.org/sd/DelegatingPatient", sd)
	res := coverage.Resource{Type: "Patient", Profiles: []string{"https://example.org/sd/DelegatingPatient"}, Body: decode(t, zusatzPatient)}
	e := element(t, coverage.Run(reg, []*coverage.StructureDefinition{p}, []coverage.Resource{res}, coverage.Options{}), "Patient.name.family.extension:zusatz")
	if !e.Unresolved || e.Reason != "no discriminator value found for this slice" {
		t.Errorf("%+v", e)
	}
}

// A derived profile marks a slice its base constraint profile defines.
func TestRun_SliceDeclaredInTheBaseProfile(t *testing.T) {
	derived := `{
  "resourceType": "StructureDefinition", "url": "https://example.org/sd/DerivedPatient",
  "name": "DerivedPatient", "type": "Patient", "kind": "resource", "derivation": "constraint",
  "baseDefinition": "https://example.org/StructureDefinition/TestPatient",
  "differential": {"element": [
    {"id": "Patient.identifier:kvid", "path": "Patient.identifier", "sliceName": "kvid", "mustSupport": true}
  ]}
}`
	reg, sd := loadPackage(t, "https://example.org/sd/DerivedPatient", derived, patientProfile)
	res := coverage.Resource{Type: "Patient", Profiles: []string{"https://example.org/sd/DerivedPatient"}, Body: decode(t, richPatient)}
	e := element(t, coverage.Run(reg, []*coverage.StructureDefinition{sd}, []coverage.Resource{res}, coverage.Options{}), "Patient.identifier:kvid")
	if e.Unresolved || !e.Populated {
		t.Errorf("the base profile's pattern decides the slice: %+v", e)
	}
}
