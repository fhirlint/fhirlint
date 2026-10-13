package lint

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzEvaluate runs every built-in rule against one resource. The resource
// comes from a repository the user did not write, and a panic here aborts the
// whole run, so only a panic or a hang fails: findings on garbage are fine.
func FuzzEvaluate(f *testing.F) {
	seeds, _ := filepath.Glob("../../testdata/*/*.json")
	for _, p := range append(seeds, "../../testdata/patient.json") {
		if data, err := os.ReadFile(p); err == nil { //nolint:gosec // fixed test fixtures
			f.Add(data)
		}
	}
	f.Add([]byte(`{"resourceType":"StructureDefinition","id":"Not_Kebab","url":"http://other.org/x","name":"lower"}`))
	f.Add([]byte(`{"resourceType":"Bundle","entry":[{"resource":{"resourceType":"Patient","id":"A B"}}]}`))

	cfg := Config{}
	for _, r := range Rules() {
		params := map[string]string{}
		for _, p := range r.RequiredParams {
			params[p] = "http://example.org/fhir"
		}
		cfg[r.Name] = RuleSetting{Severity: r.DefaultSev, Params: params}
	}
	e, err := NewEngine(cfg)
	if err != nil {
		f.Fatal(err)
	}

	f.Fuzz(func(_ *testing.T, data []byte) {
		e.Evaluate(data)
	})
}
