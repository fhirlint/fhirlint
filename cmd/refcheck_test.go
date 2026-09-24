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

// danglingBundle is a Bundle whose second entry points at a urn: no entry
// carries. The validator reports that itself, as Bundle_BUNDLE_Not_Local.
const danglingBundle = `{"resourceType":"Bundle","id":"b1","type":"transaction","entry":[
  {"fullUrl":"urn:uuid:11111111-1111-1111-1111-111111111111",
   "resource":{"resourceType":"Patient","id":"p1"}},
  {"fullUrl":"urn:uuid:22222222-2222-2222-2222-222222222222",
   "resource":{"resourceType":"Observation","id":"o1","status":"final",
               "subject":{"reference":"urn:uuid:99999999-9999-9999-9999-999999999999"}}}]}`

const danglingRef = "urn:uuid:99999999-9999-9999-9999-999999999999"

// notLocalIssue is the validator's warning for danglingRef, as it arrives from
// the JAR: the message ID comes from the operationoutcome-message-id extension
// and the reference is the message's {0} parameter.
func notLocalIssue() validator.Issue {
	return validator.Issue{
		Severity:  "warning",
		MessageID: bundleNotLocalMessageID,
		Message:   "URN reference is not locally contained within the bundle " + danglingRef,
		Location:  "Bundle.entry[1].resource/*Observation/o1*/.subject",
	}
}

func countID(res *validator.Result, id string) int {
	n := 0
	for _, iss := range res.Issues {
		if iss.MessageID == id {
			n++
		}
	}
	return n
}

// One dangling reference produced two findings: the validator's warning and the
// reference check's error, under two severities and two spellings of the path.
// --group cannot fold them together, because the messages differ (#436).
func TestApplyReferenceCheck_DropsDuplicateBundleWarning(t *testing.T) {
	res := writeResultFile(t, "bundle.json", danglingBundle)
	res.Issues = []validator.Issue{notLocalIssue()}

	applyReferenceCheck([]*validator.Result{res}, nil)

	if got := countID(res, bundleNotLocalMessageID); got != 0 {
		t.Errorf("validator warning for the same reference survived (%d), issues: %+v", got, res.Issues)
	}
	if got := countID(res, "ref:unresolved"); got != 1 {
		t.Errorf("ref:unresolved count = %d, want 1 — the reference check's finding is the one to keep", got)
	}
	if res.Valid {
		t.Error("an unresolved reference is an error, so the result must be invalid")
	}
}

// The warning is dropped only for the reference the check itself flagged. A
// second, unrelated Bundle_BUNDLE_Not_Local must survive untouched.
func TestApplyReferenceCheck_KeepsUnrelatedBundleWarnings(t *testing.T) {
	res := writeResultFile(t, "bundle.json", danglingBundle)
	other := validator.Issue{
		Severity:  "warning",
		MessageID: bundleNotLocalMessageID,
		Message:   "URN reference is not locally contained within the bundle urn:uuid:aaaaaaaa-0000-0000-0000-000000000000",
		Location:  "Bundle.entry[0].resource/*Patient/p1*/.managingOrganization",
	}
	res.Issues = []validator.Issue{notLocalIssue(), other}

	applyReferenceCheck([]*validator.Result{res}, nil)

	if got := countID(res, bundleNotLocalMessageID); got != 1 {
		t.Fatalf("Bundle_BUNDLE_Not_Local count = %d, want 1, issues: %+v", got, res.Issues)
	}
	for _, iss := range res.Issues {
		if iss.MessageID == bundleNotLocalMessageID && iss.Message != other.Message {
			t.Errorf("the wrong warning survived: %q", iss.Message)
		}
	}
}

// The reference check spans the whole validated set, the validator sees one
// Bundle. When the urn resolves against another file, the check reports nothing
// and the validator's warning is the only account of "not local to this bundle"
// — so it must stay.
func TestApplyReferenceCheck_KeepsWarningWhenRefResolvesElsewhere(t *testing.T) {
	res := writeResultFile(t, "bundle.json", danglingBundle)
	res.Issues = []validator.Issue{notLocalIssue()}
	elsewhere := writeResultFile(t, "other.json",
		`{"resourceType":"Bundle","id":"b2","type":"collection","entry":[
		   {"fullUrl":"`+danglingRef+`","resource":{"resourceType":"Patient","id":"p9"}}]}`)

	applyReferenceCheck([]*validator.Result{res, elsewhere}, nil)

	if got := countID(res, bundleNotLocalMessageID); got != 1 {
		t.Errorf("validator warning dropped although the reference resolved elsewhere: %+v", res.Issues)
	}
	if got := countID(res, "ref:unresolved"); got != 0 {
		t.Errorf("ref:unresolved count = %d, want 0 — the reference resolves across the set", got)
	}
}

// Without a reference-check finding there is nothing to reconcile against, so a
// run that produced none must leave every validator issue in place.
func TestDropDuplicateBundleRefWarnings_NoFindingsIsANoOp(t *testing.T) {
	issues := []validator.Issue{notLocalIssue()}

	if got := dropDuplicateBundleRefWarnings(issues, nil); len(got) != 1 {
		t.Errorf("len = %d, want 1 with no unresolved references", len(got))
	}
	if got := dropDuplicateBundleRefWarnings(issues, map[string]struct{}{}); len(got) != 1 {
		t.Errorf("len = %d, want 1 with an empty reference set", len(got))
	}
}

// Only Bundle_BUNDLE_Not_Local is reconciled. Another validator issue that
// happens to quote the same reference is a different statement and stays.
func TestDropDuplicateBundleRefWarnings_OnlyTheOneMessageID(t *testing.T) {
	issues := []validator.Issue{
		notLocalIssue(),
		{Severity: "error", MessageID: "Type_Specific_Checks_DT_Reference_Bad", Message: "bad reference " + danglingRef},
	}

	got := dropDuplicateBundleRefWarnings(issues, map[string]struct{}{danglingRef: {}})

	if len(got) != 1 || got[0].MessageID != "Type_Specific_Checks_DT_Reference_Bad" {
		t.Errorf("got %+v, want only the non-Bundle_BUNDLE_Not_Local issue", got)
	}
}
