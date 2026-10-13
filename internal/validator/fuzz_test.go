package validator

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fuzzExcerptCatalog loads the real 7.0.1 templates kept in testdata.
func fuzzExcerptCatalog(f *testing.F) *messageCatalog {
	f.Helper()
	data, err := os.ReadFile("testdata/messages-7.0.1-excerpt.properties")
	if err != nil {
		f.Fatal(err)
	}
	c, err := parseMessageCatalog(bytes.NewReader(data))
	if err != nil {
		f.Fatal(err)
	}
	return c
}

// FuzzMessageCatalog builds a catalog from fuzzed .properties content and
// infers an id for a fuzzed text, against both that catalog and the real 7.0.1
// templates. The texts come from the validator and, through it, from
// terminology servers (#449), so their shape is not ours to rely on. An
// inferred id must be one the catalog actually holds.
func FuzzMessageCatalog(f *testing.F) {
	excerpt := fuzzExcerptCatalog(f)
	props, err := os.ReadFile("testdata/messages-7.0.1-excerpt.properties")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(props, "A definition for CodeSystem 'http://fhir.de/CodeSystem/bfarm/atc' version '2026' could not be found, so the code cannot be validated. Valid versions: [2026]")
	f.Add([]byte("A_KEY = a ''quoted'' value with {0} and '{literal}' and {1,number}\\\n  continued\nB_one=x\\u0041\\"), "a 'quoted' value with x and {literal} and 1 continued")
	f.Add([]byte("UNBALANCED = text {0 that never closes long enough"), "text")
	f.Add([]byte("! comment\n# comment\nK:{0}{1}{2}"), "")

	ids := func(c *messageCatalog) map[string]bool {
		m := map[string]bool{}
		for _, t := range c.templates {
			m[t.id] = true
		}
		return m
	}
	excerptIDs := ids(excerpt)

	f.Fuzz(func(t *testing.T, props []byte, text string) {
		if id := excerpt.infer(text); id != "" && !excerptIDs[id] {
			t.Errorf("inferred %q, which the 7.0.1 catalog does not hold", id)
		}
		c, err := parseMessageCatalog(bytes.NewReader(props))
		if err != nil {
			return // a line over the scanner's 1 MiB limit
		}
		if id := c.infer(text); id != "" && !ids(c)[id] {
			t.Errorf("inferred %q, which the catalog does not hold", id)
		}
	})
}

// FuzzParseOutput parses a fuzzed validator output, with the 7.0.1 catalog so
// that id recovery runs as well. A malformed document is an error, never a
// panic.
func FuzzParseOutput(f *testing.F) {
	c := fuzzExcerptCatalog(f)
	seeds, _ := filepath.Glob("../../testdata/fixtures/oo-*.json")
	seeds = append(seeds, "testdata/oo-7.0.1-missing-message-ids.json", "testdata/profile-fetch-errors.json")
	for _, p := range seeds {
		if data, err := os.ReadFile(p); err == nil { //nolint:gosec // fixed test fixtures
			f.Add(data)
		}
	}
	f.Add([]byte(`{"resourceType":"Bundle","entry":[{"fullUrl":"file:///a.json","resource":{"resourceType":"OperationOutcome"}},{}]}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		results, err := parseOutputWith(data, []string{"a.json"}, "", c)
		if err != nil {
			return
		}
		for _, r := range results {
			if r == nil {
				t.Fatal("a parsed output must not contain a nil result")
			}
			for _, is := range r.Issues {
				if is.MessageIDInferred && strings.TrimSpace(is.MessageID) == "" {
					t.Errorf("issue marked as inferred without an id: %+v", is)
				}
			}
		}
	})
}
