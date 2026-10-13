package refcheck

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzCheck indexes a resource and checks its references against that index,
// the way validate does for each file in the input set. Only a panic or a hang
// fails.
func FuzzCheck(f *testing.F) {
	seeds, _ := filepath.Glob("../../testdata/*/*.json")
	for _, p := range append(seeds, "../../testdata/patient.json") {
		if data, err := os.ReadFile(p); err == nil { //nolint:gosec // fixed test fixtures
			f.Add(data)
		}
	}
	f.Add([]byte(`{"resourceType":"Bundle","entry":[{"fullUrl":"urn:uuid:1","resource":{"resourceType":"Patient","id":"p"}},` +
		`{"resource":{"resourceType":"Observation","subject":{"reference":"Patient/p"},"performer":[{"reference":"#c"}]}}]}`))
	f.Add([]byte(`{"resourceType":"Observation","contained":[{"resourceType":"Practitioner","id":"c"}],` +
		`"subject":{"reference":"http://x/fhir/Patient/1/_history/2"},"focus":[{"reference":"urn:uuid:"}]}`))

	f.Fuzz(func(_ *testing.T, data []byte) {
		ix := NewIndex()
		ix.Add(data)
		CheckWithRefs(data, ix)
	})
}
