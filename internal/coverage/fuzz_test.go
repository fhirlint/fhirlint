package coverage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// fuzzProfile is a constraint on Patient with slices, a pattern, a typed
// extension slice and a choice element, so that the seed reaches every
// branch of slice matching.
const fuzzProfile = `{
  "resourceType": "StructureDefinition",
  "url": "https://example.org/StructureDefinition/P",
  "name": "P", "type": "Patient", "kind": "resource", "derivation": "constraint",
  "baseDefinition": "https://example.org/StructureDefinition/P",
  "differential": {"element": [
    {"id": "Patient.identifier", "path": "Patient.identifier", "mustSupport": true,
     "slicing": {"discriminator": [{"type": "pattern", "path": "type"}, {"type": "value", "path": "$this"}], "rules": "open"}},
    {"id": "Patient.identifier:kvid", "path": "Patient.identifier", "sliceName": "kvid", "mustSupport": true,
     "patternIdentifier": {"type": {"coding": [{"code": "KVZ10"}]}}},
    {"id": "Patient.identifier:kvid.value", "path": "Patient.identifier.value", "mustSupport": true, "fixedString": "A"},
    {"id": "Patient.name.family.extension", "path": "Patient.name.family.extension",
     "slicing": {"discriminator": [{"type": "value", "path": "url"}]}},
    {"id": "Patient.name.family.extension:z", "path": "Patient.name.family.extension", "sliceName": "z", "mustSupport": true,
     "type": [{"code": "Extension", "profile": ["https://example.org/StructureDefinition/P"]}]},
    {"id": "Patient.deceased[x]", "path": "Patient.deceased[x]", "mustSupport": true}
  ]}
}`

const fuzzResource = `{
  "resourceType": "Patient",
  "meta": {"profile": ["https://example.org/StructureDefinition/P"]},
  "identifier": [{"type": {"coding": [{"code": "KVZ10"}]}, "value": "A"}, {"value": 1}],
  "name": [{"family": "M", "_family": {"extension": [{"url": "u", "valueString": "x"}]}}],
  "deceasedBoolean": false
}`

// FuzzRun measures a fuzzed resource against a fuzzed profile. Profiles come
// from registry packages published by third parties and resources from a
// repository the user did not write. A profile is its own base in the seed, so
// the base chain and delegation guards are exercised too. Nothing may panic,
// and every element the report lists must be one the profile declares.
func FuzzRun(f *testing.F) {
	f.Add([]byte(fuzzProfile), []byte(fuzzResource))
	profiles, _ := filepath.Glob("../../testdata/profiles/*.json")
	resources, _ := filepath.Glob("../../testdata/*/*.json")
	for i, p := range profiles {
		sd, err1 := os.ReadFile(p) //nolint:gosec // fixed test fixtures
		res, err2 := os.ReadFile(resources[i%len(resources)])
		if err1 == nil && err2 == nil {
			f.Add(sd, res)
		}
	}

	f.Fuzz(func(t *testing.T, sdJSON, resJSON []byte) {
		var sd StructureDefinition
		if err := json.Unmarshal(sdJSON, &sd); err != nil || sd.URL == "" {
			return
		}
		res, err := decodeResource("r.json", resJSON)
		if err != nil || res == nil {
			return
		}
		reg := NewRegistry()
		reg.byURL[sd.URL] = &sd

		declared := map[string]bool{}
		for _, id := range reg.Resolve(&sd).MustSupport {
			declared[id] = true
		}
		rep := Run(reg, []*StructureDefinition{&sd}, []Resource{*res}, Options{AttributeByType: true})
		for _, p := range rep.Profiles {
			for _, e := range p.Elements {
				if !declared[e.ID] {
					t.Errorf("report lists %q, which the profile does not declare mustSupport", e.ID)
				}
			}
			_ = p.Percent()
		}
	})
}
