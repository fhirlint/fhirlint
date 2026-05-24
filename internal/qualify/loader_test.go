package qualify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltinCases_ExtractsAndPairs(t *testing.T) {
	dir := t.TempDir()
	cases, err := BuiltinCases(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) < 4 {
		t.Fatalf("expected several built-in cases, got %d", len(cases))
	}

	var haveValid, haveInvalid bool
	for _, c := range cases {
		// The materialised file must exist on disk and be non-empty.
		info, statErr := os.Stat(c.Path)
		if statErr != nil || info.Size() == 0 {
			t.Errorf("case %s: file not written to disk (%v)", c.Name, statErr)
		}
		if c.Expected.Description == "" {
			t.Errorf("case %s: expectation has no description", c.Name)
		}
		if strings.HasPrefix(c.Name, "valid/") {
			haveValid = true
			if !c.Expected.Valid {
				t.Errorf("case %s under valid/ should expect valid=true", c.Name)
			}
		}
		if strings.HasPrefix(c.Name, "invalid/") {
			haveInvalid = true
			if c.Expected.Valid {
				t.Errorf("case %s under invalid/ should expect valid=false", c.Name)
			}
		}
	}
	if !haveValid || !haveInvalid {
		t.Error("built-in suite must contain both valid/ and invalid/ cases")
	}
}

func TestBuiltinCases_Sorted(t *testing.T) {
	cases, err := BuiltinCases(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(cases); i++ {
		if cases[i-1].Name > cases[i].Name {
			t.Errorf("cases not sorted: %q before %q", cases[i-1].Name, cases[i].Name)
		}
	}
}

func TestLoadSuite(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "case-a.json"), `{"resourceType":"Patient"}`)
	writeFile(t, filepath.Join(dir, "case-a.expected.json"), `{"description":"a","valid":true}`)
	// A FHIR file without an expectation companion is skipped, not loaded.
	writeFile(t, filepath.Join(dir, "orphan.json"), `{"resourceType":"Patient"}`)

	cases, err := LoadSuite(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 1 {
		t.Fatalf("expected 1 paired case, got %d", len(cases))
	}
	if cases[0].Name != "case-a.json" || !cases[0].Expected.Valid {
		t.Errorf("unexpected case loaded: %+v", cases[0])
	}
}

func TestLoadSuite_EmptyDirErrors(t *testing.T) {
	if _, err := LoadSuite(t.TempDir()); err == nil {
		t.Error("expected an error for a directory with no paired cases")
	}
}

func TestExpectedPathAndIsCaseFile(t *testing.T) {
	if got := expectedPath("foo/bar.json"); got != "foo/bar.expected.json" {
		t.Errorf("expectedPath: got %q", got)
	}
	if !isCaseFile("a/b.json") {
		t.Error("b.json should be a case file")
	}
	if isCaseFile("a/b.expected.json") {
		t.Error("expected files must not be treated as case files")
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}
