package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
	"go.yaml.in/yaml/v3"
)

const vitalSigns = "http://hl7.org/fhir/StructureDefinition/vitalsigns"

// The example from #524, written with `or` because the subset has no union.
const vitalSignsWhen = "code.coding.where(system='http://loinc.org' and (code='8867-4' or code='8310-5')).exists()"

func profileMapFromYAML(t *testing.T, src string) (profileMapping, error) {
	t.Helper()
	var raw map[string]any
	if err := yaml.Unmarshal([]byte(src), &raw); err != nil {
		t.Fatal(err)
	}
	return parseProfileMap(raw["profile-map"])
}

func writeMapFixture(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

const vitalObsConfig = `
profile-map:
  Observation:
    - profile: ` + vitalSigns + `
      when: "` + vitalSignsWhen + `"
    - https://example.org/StructureDefinition/my-observation
  Patient: https://example.org/StructureDefinition/my-patient
`

func TestParseProfileMap_Forms(t *testing.T) {
	pm, err := profileMapFromYAML(t, vitalObsConfig)
	if err != nil {
		t.Fatal(err)
	}
	obs := pm["Observation"]
	if len(obs) != 2 || obs[0].Profile != vitalSigns || obs[0].When == nil || obs[1].When != nil {
		t.Errorf("Observation = %+v", obs)
	}
	if p := pm["Patient"]; len(p) != 1 || p[0].When != nil {
		t.Errorf("Patient = %+v", p)
	}
	// A single map, not in a list.
	pm, err = profileMapFromYAML(t, "profile-map:\n  Observation: {profile: x, when: \"status = 'final'\"}\n")
	if err != nil || len(pm["Observation"]) != 1 || pm["Observation"][0].When == nil {
		t.Errorf("single map: %+v, %v", pm, err)
	}
}

func TestParseProfileMap_Errors(t *testing.T) {
	for name, src := range map[string]string{
		"union is unsupported": "profile-map:\n  Observation:\n    - {profile: x, when: \"code in ('a' | 'b')\"}\n",
		"no profile":           "profile-map:\n  Observation:\n    - {when: \"status = 'final'\"}\n",
		"unknown key":          "profile-map:\n  Observation:\n    - {profile: x, if: \"true\"}\n",
		"empty when":           "profile-map:\n  Observation:\n    - {profile: x, when: \"\"}\n",
	} {
		if _, err := profileMapFromYAML(t, src); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestResolveProfileEntries_Condition(t *testing.T) {
	profileMapApplied.reset()
	pm, err := profileMapFromYAML(t, vitalObsConfig)
	if err != nil {
		t.Fatal(err)
	}
	vital := writeMapFixture(t, "hr.json", `{"resourceType":"Observation","status":"final","code":{"coding":[{"system":"http://loinc.org","code":"8867-4"}]}}`)
	other := writeMapFixture(t, "glucose.json", `{"resourceType":"Observation","status":"final","code":{"coding":[{"system":"http://loinc.org","code":"2339-0"}]}}`)

	got := resolveProfileEntries(vital, pm)
	if len(got) != 2 || got[0].Profile != vitalSigns || !strings.Contains(got[0].Reason, "when code.coding") ||
		got[1].Reason != "profile-map: Observation" {
		t.Errorf("vital sign: %+v", got)
	}
	if got := resolveProfilesForPath(other, pm); !reflect.DeepEqual(got, []string{"https://example.org/StructureDefinition/my-observation"}) {
		t.Errorf("other observation: %v", got)
	}

	results := []*validator.Result{{Filename: vital, SourcePath: vital}, {Filename: other, SourcePath: other}}
	profileMapApplied.attach(results)
	if len(results[0].AppliedProfiles) != 2 || len(results[1].AppliedProfiles) != 1 {
		t.Errorf("attached: %+v / %+v", results[0].AppliedProfiles, results[1].AppliedProfiles)
	}
}

// XML cannot be evaluated: the conditional entry is skipped with a warning,
// the unconditional one still applies.
func TestResolveProfileEntries_XMLFallsBack(t *testing.T) {
	profileMapApplied.reset()
	pm, err := profileMapFromYAML(t, "profile-map:\n  \"*.xml\":\n    - {profile: a, when: \"status = 'final'\"}\n    - b\n")
	if err != nil {
		t.Fatal(err)
	}
	x := writeMapFixture(t, "obs.xml", `<Observation xmlns="http://hl7.org/fhir"><status value="final"/></Observation>`)
	if got := resolveProfilesForPath(x, pm); !reflect.DeepEqual(got, []string{"b"}) {
		t.Errorf("xml: %v", got)
	}
}

// A condition that is valid but not a predicate fails at evaluation: the
// profile does not apply, and the run says why instead of guessing.
func TestResolveProfileEntries_EvalErrorDoesNotApply(t *testing.T) {
	profileMapApplied.reset()
	pm, err := profileMapFromYAML(t, "profile-map:\n  Observation:\n    - {profile: a, when: \"code.coding\"}\n")
	if err != nil {
		t.Fatal(err)
	}
	p := writeMapFixture(t, "o.json", `{"resourceType":"Observation","code":{"coding":[{"code":"1"},{"code":"2"}]}}`)
	if got := resolveProfilesForPath(p, pm); len(got) != 0 {
		t.Errorf("non-boolean condition applied: %v", got)
	}
}
