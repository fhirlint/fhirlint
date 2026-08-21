package coverage_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/coverage"
)

// patientProfile exercises the shapes that actually occur in German IGs:
// slices discriminated by a pattern on $this, an extension slice identified by
// its type.profile, a choice element, and a slice whose only distinguishing
// feature is a value set binding.
const patientProfile = `{
  "resourceType": "StructureDefinition",
  "url": "https://example.org/StructureDefinition/TestPatient",
  "name": "TestPatient",
  "id": "TestPatient",
  "type": "Patient",
  "kind": "resource",
  "derivation": "constraint",
  "baseDefinition": "http://hl7.org/fhir/StructureDefinition/Patient",
  "differential": {
    "element": [
      {"id": "Patient.identifier", "path": "Patient.identifier", "mustSupport": true,
       "slicing": {"discriminator": [{"type": "pattern", "path": "$this"}], "rules": "open"}},
      {"id": "Patient.identifier:kvid", "path": "Patient.identifier", "sliceName": "kvid", "mustSupport": true,
       "patternIdentifier": {"type": {"coding": [{"code": "KVZ10", "system": "http://fhir.de/CodeSystem/identifier-type-de-basis"}]}}},
      {"id": "Patient.identifier:kvid.value", "path": "Patient.identifier.value", "mustSupport": true},
      {"id": "Patient.identifier:pkv", "path": "Patient.identifier", "sliceName": "pkv", "mustSupport": true,
       "patternIdentifier": {"type": {"coding": [{"code": "PKV", "system": "http://fhir.de/CodeSystem/identifier-type-de-basis"}]}}},
      {"id": "Patient.name", "path": "Patient.name", "mustSupport": true,
       "slicing": {"discriminator": [{"type": "pattern", "path": "$this"}], "rules": "open"}},
      {"id": "Patient.name:official", "path": "Patient.name", "sliceName": "official", "mustSupport": true,
       "patternHumanName": {"use": "official"}},
      {"id": "Patient.name:official.family", "path": "Patient.name.family", "mustSupport": true},
      {"id": "Patient.name:official.family.extension:namenszusatz", "path": "Patient.name.family.extension",
       "sliceName": "namenszusatz", "mustSupport": true,
       "type": [{"code": "Extension", "profile": ["http://fhir.de/StructureDefinition/humanname-namenszusatz"]}]},
      {"id": "Patient.name:maiden", "path": "Patient.name", "sliceName": "maiden", "mustSupport": true,
       "patternHumanName": {"use": "maiden"}},
      {"id": "Patient.deceased[x]", "path": "Patient.deceased[x]", "mustSupport": true},
      {"id": "Patient.type", "path": "Patient.type", "mustSupport": true,
       "slicing": {"discriminator": [{"type": "pattern", "path": "$this"}], "rules": "open"}},
      {"id": "Patient.type:bound", "path": "Patient.type", "sliceName": "bound", "mustSupport": true,
       "binding": {"strength": "required", "valueSet": "https://example.org/ValueSet/kinds"}}
    ]
  }
}`

// richPatient populates the kvid identifier, an official name whose family
// carries the namenszusatz extension, and a deceased choice.
const richPatient = `{
  "resourceType": "Patient",
  "meta": {"profile": ["https://example.org/StructureDefinition/TestPatient"]},
  "identifier": [
    {"type": {"coding": [{"code": "KVZ10", "system": "http://fhir.de/CodeSystem/identifier-type-de-basis"}]},
     "system": "http://fhir.de/sid/gkv/kvid-10", "value": "A123456789"}
  ],
  "name": [
    {"use": "official", "family": "Musterfrau",
     "_family": {"extension": [{"url": "http://fhir.de/StructureDefinition/humanname-namenszusatz", "valueString": "Gräfin"}]}}
  ],
  "deceasedBoolean": false,
  "type": [{"coding": [{"code": "anything"}]}]
}`

func loadRegistry(t *testing.T, docs ...string) (*coverage.Registry, *coverage.StructureDefinition) {
	t.Helper()
	dir := t.TempDir()
	for i, doc := range docs {
		path := filepath.Join(dir, "sd"+string(rune('a'+i))+".json")
		if err := os.WriteFile(path, []byte(doc), 0600); err != nil {
			t.Fatal(err)
		}
	}
	reg := coverage.NewRegistry()
	if _, err := reg.LoadPackage(dir, "test#1.0.0"); err != nil {
		t.Fatal(err)
	}
	profiles := reg.ProfilesFrom("test#1.0.0")
	if len(profiles) == 0 {
		t.Fatal("no profiles loaded")
	}
	return reg, profiles[0]
}

func decode(t *testing.T, doc string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(doc), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestPopulated(t *testing.T) {
	reg, sd := loadRegistry(t, patientProfile)
	res := decode(t, richPatient)

	cases := []struct {
		id         string
		want       bool
		unresolved bool
		why        string
	}{
		{id: "Patient.identifier", want: true, why: "the resource has an identifier"},
		{id: "Patient.identifier:kvid", want: true, why: "its type coding matches the kvid pattern"},
		{id: "Patient.identifier:kvid.value", want: true, why: "the kvid slice carries a value"},
		{id: "Patient.identifier:pkv", want: false, why: "no identifier matches the PKV pattern"},
		{id: "Patient.name:official", want: true, why: "the name declares use=official"},
		{id: "Patient.name:official.family", want: true, why: "that name has a family"},
		// The extension lives under "_family", not "family" — the one place a
		// naive walk would report a false negative.
		{id: "Patient.name:official.family.extension:namenszusatz", want: true,
			why: "the extension sits on the primitive's companion object"},
		{id: "Patient.name:maiden", want: false, why: "no name declares use=maiden"},
		// The profile writes deceased[x]; the instance writes deceasedBoolean.
		{id: "Patient.deceased[x]", want: true, why: "the choice element resolves to deceasedBoolean"},
		// A binding cannot decide slice membership without expanding it.
		{id: "Patient.type:bound", unresolved: true, why: "the slice is identified only by a value set binding"},
	}

	for _, tc := range cases {
		got, reason := reg.Populated(sd, res, tc.id)
		if tc.unresolved {
			if reason == "" {
				t.Errorf("%s: expected unresolved (%s), got populated=%v", tc.id, tc.why, got)
			}
			continue
		}
		if reason != "" {
			t.Errorf("%s: unexpectedly unresolved: %s", tc.id, reason)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: populated = %v, want %v — %s", tc.id, got, tc.want, tc.why)
		}
	}
}

func TestPopulatedIgnoresEmptyValues(t *testing.T) {
	reg, sd := loadRegistry(t, patientProfile)

	// An empty string, an empty array and an absent value all populate nothing.
	// Counting them would let a placeholder example pass for a real one.
	res := decode(t, `{
	  "resourceType": "Patient",
	  "identifier": [],
	  "name": [{"use": "official", "family": "  "}]
	}`)

	if got, _ := reg.Populated(sd, res, "Patient.identifier"); got {
		t.Error("an empty identifier array must not count as populated")
	}
	if got, _ := reg.Populated(sd, res, "Patient.name:official.family"); got {
		t.Error("a blank family string must not count as populated")
	}
	if got, _ := reg.Populated(sd, res, "Patient.name:official"); !got {
		t.Error("the name itself is present and should count")
	}
}

const basePatientProfile = `{
  "resourceType": "StructureDefinition",
  "url": "https://example.org/StructureDefinition/BasePatient",
  "name": "BasePatient",
  "type": "Patient",
  "kind": "resource",
  "derivation": "constraint",
  "baseDefinition": "http://hl7.org/fhir/StructureDefinition/Patient",
  "differential": {"element": [
    {"id": "Patient.birthDate", "path": "Patient.birthDate", "mustSupport": true},
    {"id": "Patient.gender", "path": "Patient.gender", "mustSupport": true}
  ]}
}`

const derivedPatientProfile = `{
  "resourceType": "StructureDefinition",
  "url": "https://example.org/StructureDefinition/DerivedPatient",
  "name": "DerivedPatient",
  "type": "Patient",
  "kind": "resource",
  "derivation": "constraint",
  "baseDefinition": "https://example.org/StructureDefinition/BasePatient",
  "differential": {"element": [
    {"id": "Patient.active", "path": "Patient.active", "mustSupport": true}
  ]}
}`

func TestResolveInheritsFromBaseWhenNoSnapshot(t *testing.T) {
	reg, _ := loadRegistry(t, derivedPatientProfile, basePatientProfile)

	sd, ok := reg.Get("https://example.org/StructureDefinition/DerivedPatient")
	if !ok {
		t.Fatal("derived profile not loaded")
	}
	p := reg.Resolve(sd)

	// A differential holds only what the profile itself constrains. Without the
	// base chain walk, the two inherited mustSupport elements would silently
	// vanish from the denominator.
	want := map[string]bool{"Patient.active": true, "Patient.birthDate": true, "Patient.gender": true}
	if len(p.MustSupport) != len(want) {
		t.Fatalf("mustSupport = %v, want the 3 elements %v", p.MustSupport, want)
	}
	for _, id := range p.MustSupport {
		if !want[id] {
			t.Errorf("unexpected element %q", id)
		}
	}
	if len(p.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", p.Warnings)
	}
}

func TestResolveWarnsWhenBaseIsMissing(t *testing.T) {
	// The base is not loaded, so inherited elements cannot be seen. The count
	// is then a lower bound and the report has to say so rather than present it
	// as complete.
	reg, sd := loadRegistry(t, derivedPatientProfile)

	p := reg.Resolve(sd)

	if len(p.Warnings) == 0 {
		t.Fatal("want a warning about the unresolvable base profile")
	}
	if !strings.Contains(p.Warnings[0], "BasePatient") {
		t.Errorf("warning should name the missing base, got %q", p.Warnings[0])
	}
}

func TestRunAttribution(t *testing.T) {
	reg, sd := loadRegistry(t, patientProfile)

	declared := coverage.Resource{Type: "Patient", Profiles: []string{
		"https://example.org/StructureDefinition/TestPatient"}, Body: decode(t, richPatient)}
	bare := coverage.Resource{Type: "Patient", Body: decode(t,
		`{"resourceType": "Patient", "name": [{"use": "maiden", "family": "Quelle"}]}`)}

	t.Run("by meta.profile only", func(t *testing.T) {
		rep := coverage.Run(reg, []*coverage.StructureDefinition{sd},
			[]coverage.Resource{declared, bare}, coverage.Options{})

		if len(rep.Profiles) != 1 {
			t.Fatalf("got %d profile reports, want 1", len(rep.Profiles))
		}
		if rep.Profiles[0].Resources != 1 || rep.Profiles[0].ByType != 0 {
			t.Errorf("resources = %d (byType %d), want 1 (0)", rep.Profiles[0].Resources, rep.Profiles[0].ByType)
		}
		if rep.Unattributed != 1 {
			t.Errorf("unattributed = %d, want 1", rep.Unattributed)
		}
	})

	t.Run("with attribution by type", func(t *testing.T) {
		rep := coverage.Run(reg, []*coverage.StructureDefinition{sd},
			[]coverage.Resource{declared, bare}, coverage.Options{AttributeByType: true})

		p := rep.Profiles[0]
		if p.Resources != 2 || p.ByType != 1 {
			t.Errorf("resources = %d (byType %d), want 2 (1)", p.Resources, p.ByType)
		}
		// The bare resource contributes the maiden name, which the declared one
		// lacks — coverage is the union across every attributed resource.
		if !elementPopulated(p, "Patient.name:maiden") {
			t.Error("the by-type resource should have covered the maiden name slice")
		}
	})
}

func TestRunExcludesUnresolvedFromThePercentage(t *testing.T) {
	reg, sd := loadRegistry(t, patientProfile)
	res := coverage.Resource{Type: "Patient", Profiles: []string{
		"https://example.org/StructureDefinition/TestPatient"}, Body: decode(t, richPatient)}

	rep := coverage.Run(reg, []*coverage.StructureDefinition{sd}, []coverage.Resource{res}, coverage.Options{})
	p := rep.Profiles[0]

	if p.Unresolved() == 0 {
		t.Fatal("expected the binding-only slice to be unresolved")
	}
	if p.Measurable() != len(p.Elements)-p.Unresolved() {
		t.Errorf("measurable = %d, want %d", p.Measurable(), len(p.Elements)-p.Unresolved())
	}
	// An element that could not be measured must not be counted as a miss: that
	// would report a limitation of the tool as a gap in the user's data.
	for _, e := range p.Elements {
		if e.Unresolved {
			for _, id := range p.Missing() {
				if id == e.ID {
					t.Errorf("%s is unresolved but listed as missing", e.ID)
				}
			}
		}
	}
}

func TestRunSkipsProfilesWithNoResources(t *testing.T) {
	reg, sd := loadRegistry(t, patientProfile)

	rep := coverage.Run(reg, []*coverage.StructureDefinition{sd},
		[]coverage.Resource{{Type: "Observation", Body: decode(t, `{"resourceType": "Observation"}`)}},
		coverage.Options{})

	// Reporting 0% for a profile nothing was measured against would be a
	// verdict on data that does not exist.
	if len(rep.Profiles) != 0 {
		t.Errorf("got %d profile reports, want none", len(rep.Profiles))
	}
	if rep.ProfilesWithoutResources != 1 {
		t.Errorf("profilesWithoutResources = %d, want 1", rep.ProfilesWithoutResources)
	}
}

func elementPopulated(p coverage.ProfileReport, id string) bool {
	for _, e := range p.Elements {
		if e.ID == id {
			return e.Populated
		}
	}
	return false
}

func TestLoadResources(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("patient.json", `{"resourceType":"Patient","meta":{"profile":["https://example.org/p"]}}`)
	write("bulk.ndjson", "{\"resourceType\":\"Observation\"}\n\n{\"resourceType\":\"Encounter\"}\n")
	write("notfhir.json", `{"hello":"world"}`)
	write("legacy.xml", `<Patient/>`)

	resources, skipped, err := coverage.LoadResources(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(resources) != 3 {
		t.Errorf("got %d resources, want 3 (one JSON, two NDJSON lines)", len(resources))
	}
	// XML is reported rather than passed over: a silently ignored input reads
	// as a measured one.
	if len(skipped) != 1 || !strings.Contains(skipped[0].Reason, "XML") {
		t.Errorf("skipped = %+v, want one XML entry", skipped)
	}
	for _, r := range resources {
		if r.Type == "" {
			t.Errorf("resource %s has no type", r.Path)
		}
	}
}

func TestFilterProfiles(t *testing.T) {
	reg, _ := loadRegistry(t, patientProfile, basePatientProfile)
	all := reg.ProfilesFrom("test#1.0.0")

	for _, selector := range []string{
		"TestPatient",
		"testpatient",
		"https://example.org/StructureDefinition/TestPatient",
	} {
		got := coverage.FilterProfiles(all, []string{selector})
		if len(got) != 1 || got[0].Name != "TestPatient" {
			t.Errorf("selector %q matched %d profiles, want just TestPatient", selector, len(got))
		}
	}

	if got := coverage.FilterProfiles(all, nil); len(got) != len(all) {
		t.Errorf("no selector should keep everything, got %d of %d", len(got), len(all))
	}
	if got := coverage.FilterProfiles(all, []string{"NoSuchProfile"}); len(got) != 0 {
		t.Errorf("unknown selector matched %d profiles, want 0", len(got))
	}
}

func TestProfilesFromExcludesOtherPackages(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.json"), []byte(patientProfile), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "b.json"), []byte(basePatientProfile), 0600); err != nil {
		t.Fatal(err)
	}

	reg := coverage.NewRegistry()
	if _, err := reg.LoadPackage(dir, "wanted#1.0.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.LoadPackage(other, "support#1.0.0"); err != nil {
		t.Fatal(err)
	}

	// Supporting packages are loaded so slices can be resolved across them, but
	// measuring their profiles against this dataset would be noise.
	got := reg.ProfilesFrom("wanted#1.0.0")
	if len(got) != 1 || got[0].Name != "TestPatient" {
		t.Fatalf("got %d profiles, want only TestPatient", len(got))
	}
	if _, ok := reg.Get("https://example.org/StructureDefinition/BasePatient"); !ok {
		t.Error("the supporting profile should still be resolvable by URL")
	}
}

// coverage reads files itself, so a format it cannot parse is reported as
// skipped — the same treatment XML already gets. Counting a mapping file as an
// unmeasured resource would understate coverage without saying why (#341).
func TestLoadResources_UnreadableFormatsAreReported(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("patient.json", `{"resourceType":"Patient"}`)
	write("bulk.jsonl", "{\"resourceType\":\"Observation\"}\n{\"resourceType\":\"Encounter\"}\n")
	write("transform.fml", `map "http://example.org/StructureMap/Demo" = "Demo"`)

	resources, skipped, err := coverage.LoadResources(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	// .jsonl counts per line, exactly like .ndjson.
	if len(resources) != 3 {
		t.Errorf("got %d resources, want 3 (one JSON, two JSONL lines)", len(resources))
	}
	if len(skipped) != 1 || !strings.Contains(skipped[0].Reason, "FHIR Mapping Language") {
		t.Errorf("skipped = %+v, want one entry naming the format", skipped)
	}
}
