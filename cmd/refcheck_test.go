package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

// writeResultFile writes content to a temp file and returns a Result pointing at it.
func writeResultFile(t *testing.T, name, content string) *validator.Result {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return &validator.Result{Filename: p, Label: name, Valid: true}
}

func hasIssueID(res *validator.Result, id string) bool {
	for _, iss := range res.Issues {
		if iss.MessageID == id {
			return true
		}
	}
	return false
}

func TestApplyReferenceCheck_CrossFileResolution(t *testing.T) {
	patient := writeResultFile(t, "patient.json", `{"resourceType":"Patient","id":"p1"}`)
	enc := writeResultFile(t, "enc.json", `{"resourceType":"Encounter","id":"e1","subject":{"reference":"Patient/p1"},"participant":[{"individual":{"reference":"Practitioner/ghost"}}]}`)

	applyReferenceCheck([]*validator.Result{patient, enc}, nil)

	// Patient/p1 resolves across files; Practitioner/ghost does not.
	if !hasIssueID(enc, "ref:unresolved") {
		t.Fatalf("expected unresolved finding on encounter, got %+v", enc.Issues)
	}
	if enc.Valid {
		t.Error("encounter must be marked invalid after an error-severity reference finding")
	}
	if len(patient.Issues) != 0 {
		t.Errorf("patient should have no reference findings, got %+v", patient.Issues)
	}
	// Only the ghost reference should be flagged (Patient/p1 resolved).
	count := 0
	for _, iss := range enc.Issues {
		if iss.MessageID == "ref:unresolved" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 unresolved reference, got %d", count)
	}
}

func TestApplyReferenceCheck_SkipsXML(t *testing.T) {
	xml := writeResultFile(t, "enc.xml", `<Encounter xmlns="http://hl7.org/fhir"><subject><reference value="Patient/nope"/></subject></Encounter>`)
	applyReferenceCheck([]*validator.Result{xml}, nil)
	if len(xml.Issues) != 0 {
		t.Fatalf("XML must be skipped by reference check, got %+v", xml.Issues)
	}
}

func TestApplyReferenceCheck_EmptySet(t *testing.T) {
	// No panics and no findings for an empty result set.
	applyReferenceCheck(nil, nil)
}
