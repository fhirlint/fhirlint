package stats

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzParse feeds one input to all three readers stats has: JSON, NDJSON and
// XML. Unparseable input is expected to come back as an untyped resource, so
// only a panic or a hang fails.
func FuzzParse(f *testing.F) {
	seeds, _ := filepath.Glob("../../testdata/*/*.json")
	for _, p := range append(seeds, "../../testdata/patient.json") {
		if data, err := os.ReadFile(p); err == nil { //nolint:gosec // fixed test fixtures
			f.Add(data)
		}
	}
	f.Add([]byte("{\"resourceType\":\"Patient\",\"meta\":{\"profile\":[\"http://a\",\" \"]}}\n\n{\"resourceType\":\"Observation\"}\n"))
	f.Add([]byte(`<Patient xmlns="http://hl7.org/fhir"><meta><profile value="http://a"/></meta></Patient>`))
	f.Add([]byte(`{"meta":{"profile":"not-an-array"}}`))

	f.Fuzz(func(_ *testing.T, data []byte) {
		parseJSON(data)
		parseNDJSON(data)
		parseXML(data)
	})
}
