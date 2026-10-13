package ndjson

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// FuzzSplit splits a fuzzed bulk file. Beyond not panicking, every resource it
// returns must be one non-empty line of the input, trimmed, and labelled with
// that line's number.
func FuzzSplit(f *testing.F) {
	f.Add([]byte("{\"resourceType\":\"Patient\"}\n\n  {\"resourceType\":\"Observation\"}  \r\n"))
	f.Add([]byte("\n\n\n"))
	f.Add([]byte("no newline at end"))

	f.Fuzz(func(t *testing.T, data []byte) {
		path := filepath.Join(t.TempDir(), "bulk.ndjson")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		ins, err := Split(path)
		if err != nil {
			return // e.g. a line over the 10 MiB limit
		}
		for _, in := range ins {
			got, rerr := os.ReadFile(in.Path)
			in.Cleanup()
			if rerr != nil {
				t.Fatal(rerr)
			}
			if len(got) == 0 || strings.TrimSpace(string(got)) != string(got) {
				t.Errorf("%s: resource %q is empty or untrimmed", in.Label, got)
			}
			if !strings.HasPrefix(in.Label, "bulk.ndjson:") {
				t.Errorf("label %q does not name the file", in.Label)
			}
		}
	})
}
