// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package input

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Stdin is written to a temp file whose extension tells the validator the
// format (#634): XML with .xml, anything else with .json.
func TestResolve_StdinKeepsTheFormat(t *testing.T) {
	for _, tc := range []struct{ arg, body, ext string }{
		{"-", `{"resourceType":"Patient"}`, ".json"},
		{"", "  \n<Patient xmlns=\"http://hl7.org/fhir\"/>", ".xml"},
	} {
		f, err := os.CreateTemp(t.TempDir(), "stdin")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString(tc.body); err != nil {
			t.Fatal(err)
		}
		if _, err := f.Seek(0, 0); err != nil {
			t.Fatal(err)
		}
		old := os.Stdin
		os.Stdin = f
		in, err := Resolve(tc.arg, "", 0)
		os.Stdin = old
		_ = f.Close()
		if err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(in.Path)
		if in.Source != SourceStdin || in.Label != "stdin" || filepath.Ext(in.Path) != tc.ext || string(data) != tc.body {
			t.Errorf("%q: %+v, content %q", tc.arg, in, data)
		}
		in.Cleanup()
		if _, err := os.Stat(in.Path); !os.IsNotExist(err) {
			t.Errorf("Cleanup must remove the temp file: %v", err)
		}
	}
}

func TestResolve_URLThatCannotBeFetched(t *testing.T) {
	for _, u := range []string{"http://127.0.0.1:1/p.json", "http://[::1"} {
		if _, err := Resolve("", u, 0); err == nil || !strings.Contains(err.Error(), "fetching") {
			t.Errorf("%s: %v", u, err)
		}
	}
}
