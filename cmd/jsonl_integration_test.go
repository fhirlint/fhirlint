//go:build integration

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

// TestIntegration_JSONLMatchesNDJSON is the regression test for #340: the same
// two resources under two names must produce the same report.
//
// Before the extension table, .jsonl went to the JAR whole. It still validated
// — the JAR sniffs the content — but as one input rather than two resources, so
// the counts, the file:line labels and even the finding count differed purely
// by extension.
func TestIntegration_JSONLMatchesNDJSON(t *testing.T) {
	const body = `{"resourceType":"Patient","id":"a","gender":"male"}
{"resourceType":"Patient","id":"b","gender":"nope"}
`
	dir := t.TempDir()
	opts := validator.Options{FHIRVersion: "4.0.1", NoTerminologyServer: true}

	run := func(name string) []*validator.Result {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		results, err := validateNDJSON(path, opts, nil, nil)
		if err != nil {
			t.Fatalf("validateNDJSON(%s): %v", name, err)
		}
		return results
	}

	nd := run("export.ndjson")
	jl := run("export.jsonl")

	if len(nd) != 2 {
		t.Fatalf("got %d results for .ndjson, want one per line", len(nd))
	}
	if len(jl) != len(nd) {
		t.Fatalf("got %d results for .jsonl and %d for .ndjson", len(jl), len(nd))
	}

	summarise := func(results []*validator.Result) string {
		out := ""
		for _, r := range results {
			out += fmt.Sprintf("valid=%v issues=%d;", r.Valid, len(r.Issues))
		}
		return out
	}
	if summarise(jl) != summarise(nd) {
		t.Errorf(".jsonl summary %q, .ndjson summary %q — the extension changed the report",
			summarise(jl), summarise(nd))
	}
}
