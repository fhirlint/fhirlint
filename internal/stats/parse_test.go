package stats

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParseFile_JSON(t *testing.T) {
	p := write(t, t.TempDir(), "p.json",
		`{"resourceType":"Patient","meta":{"profile":["http://a","http://b"]}}`)
	rs, err := ParseFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 || rs[0].Type != "Patient" {
		t.Fatalf("expected one Patient, got %+v", rs)
	}
	if len(rs[0].Profiles) != 2 {
		t.Errorf("expected 2 profiles, got %v", rs[0].Profiles)
	}
}

func TestParseFile_JSONNoProfile(t *testing.T) {
	p := write(t, t.TempDir(), "p.json", `{"resourceType":"Observation"}`)
	rs, _ := ParseFile(p)
	if rs[0].Type != "Observation" || len(rs[0].Profiles) != 0 {
		t.Errorf("unexpected: %+v", rs[0])
	}
}

func TestParseFile_NDJSON(t *testing.T) {
	p := write(t, t.TempDir(), "bulk.ndjson",
		"{\"resourceType\":\"Patient\"}\n\n{\"resourceType\":\"Medication\"}\n")
	rs, err := ParseFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 {
		t.Fatalf("expected 2 resources (blank line skipped), got %d", len(rs))
	}
	if rs[0].Type != "Patient" || rs[1].Type != "Medication" {
		t.Errorf("unexpected types: %+v", rs)
	}
}

func TestParseFile_XML(t *testing.T) {
	p := write(t, t.TempDir(), "p.xml",
		`<Patient xmlns="http://hl7.org/fhir"><meta><profile value="http://x"/></meta></Patient>`)
	rs, err := ParseFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if rs[0].Type != "Patient" {
		t.Errorf("expected Patient from XML root, got %q", rs[0].Type)
	}
	if len(rs[0].Profiles) != 1 || rs[0].Profiles[0] != "http://x" {
		t.Errorf("expected one profile http://x, got %v", rs[0].Profiles)
	}
}

func TestParseFile_GarbageJSONYieldsEmptyType(t *testing.T) {
	p := write(t, t.TempDir(), "bad.json", `not json at all`)
	rs, _ := ParseFile(p)
	if len(rs) != 1 || rs[0].Type != "" {
		t.Errorf("expected one resource with empty type, got %+v", rs)
	}
}

// .jsonl is NDJSON under the name the surrounding data tooling produces, so it
// must count per line. Before #340 it fell through to the JSON parser and
// counted the whole file as one resource with no type.
func TestParseFile_JSONL(t *testing.T) {
	p := write(t, t.TempDir(), "bulk.jsonl",
		"{\"resourceType\":\"Patient\"}\n{\"resourceType\":\"Medication\"}\n")
	rs, err := ParseFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 {
		t.Fatalf("expected 2 resources, got %d: %+v", len(rs), rs)
	}
	if rs[0].Type != "Patient" || rs[1].Type != "Medication" {
		t.Errorf("unexpected types: %+v", rs)
	}
}

// A FHIR Mapping Language file is valid input for the validator but not
// something stats can read. It must count as nothing rather than as one
// malformed resource (#341).
func TestParseFile_ValidatorOnlyFormatCountsNothing(t *testing.T) {
	p := write(t, t.TempDir(), "transform.fml",
		"map \"http://example.org/StructureMap/Demo\" = \"Demo\"\n")
	rs, err := ParseFile(p)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(rs) != 0 {
		t.Errorf("got %d resources from a mapping file, want none: %+v", len(rs), rs)
	}
}
