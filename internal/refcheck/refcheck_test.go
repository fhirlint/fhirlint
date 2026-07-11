package refcheck

import (
	"strings"
	"testing"
)

// index builds an Index from a set of resource JSON documents.
func index(docs ...string) *Index {
	ix := NewIndex()
	for _, d := range docs {
		ix.Add([]byte(d))
	}
	return ix
}

func TestResolvedRelativeReference(t *testing.T) {
	ix := index(
		`{"resourceType":"Patient","id":"p1"}`,
		`{"resourceType":"Encounter","id":"e1","subject":{"reference":"Patient/p1"}}`,
	)
	issues := Check([]byte(`{"resourceType":"Encounter","id":"e1","subject":{"reference":"Patient/p1"}}`), ix)
	if len(issues) != 0 {
		t.Fatalf("expected no findings for resolved reference, got %+v", issues)
	}
}

func TestUnresolvedRelativeReference(t *testing.T) {
	ix := index(`{"resourceType":"Encounter","id":"e1","subject":{"reference":"Patient/missing"}}`)
	issues := Check([]byte(`{"resourceType":"Encounter","id":"e1","subject":{"reference":"Patient/missing"}}`), ix)
	if len(issues) != 1 {
		t.Fatalf("expected 1 finding, got %+v", issues)
	}
	if issues[0].MessageID != MsgUnresolved || issues[0].Severity != "error" {
		t.Fatalf("unexpected finding: %+v", issues[0])
	}
	if issues[0].Location != "Encounter.subject.reference" {
		t.Errorf("unexpected location %q", issues[0].Location)
	}
}

func TestHistoryVersionStripped(t *testing.T) {
	ix := index(`{"resourceType":"Patient","id":"p1"}`)
	issues := Check([]byte(`{"resourceType":"Observation","id":"o1","subject":{"reference":"Patient/p1/_history/3"}}`), ix)
	if len(issues) != 0 {
		t.Fatalf("history-versioned reference should resolve, got %+v", issues)
	}
}

func TestContainedReference(t *testing.T) {
	resolved := `{"resourceType":"Observation","id":"o1","contained":[{"resourceType":"Patient","id":"pat"}],"subject":{"reference":"#pat"}}`
	if issues := Check([]byte(resolved), NewIndex()); len(issues) != 0 {
		t.Fatalf("resolved contained ref should have no findings, got %+v", issues)
	}

	broken := `{"resourceType":"Observation","id":"o1","contained":[{"resourceType":"Patient","id":"pat"}],"subject":{"reference":"#nope"}}`
	issues := Check([]byte(broken), NewIndex())
	if len(issues) != 1 || issues[0].MessageID != MsgUnresolved || issues[0].Severity != "error" {
		t.Fatalf("expected one unresolved contained finding, got %+v", issues)
	}
}

func TestSelfContainedHashSkipped(t *testing.T) {
	if issues := Check([]byte(`{"resourceType":"Patient","id":"p1","link":[{"other":{"reference":"#"}}]}`), NewIndex()); len(issues) != 0 {
		t.Fatalf(`"#" self reference must be skipped, got %+v`, issues)
	}
}

func TestBundleUrnReference(t *testing.T) {
	bundle := `{
	  "resourceType":"Bundle","type":"transaction",
	  "entry":[
	    {"fullUrl":"urn:uuid:aaa","resource":{"resourceType":"Patient"}},
	    {"fullUrl":"urn:uuid:bbb","resource":{"resourceType":"Encounter","subject":{"reference":"urn:uuid:aaa"}}}
	  ]
	}`
	ix := index(bundle)
	if issues := Check([]byte(bundle), ix); len(issues) != 0 {
		t.Fatalf("resolved urn reference should have no findings, got %+v", issues)
	}

	broken := strings.Replace(bundle, "urn:uuid:aaa\"}", "urn:uuid:zzz\"}", 1)
	ix2 := index(broken)
	issues := Check([]byte(broken), ix2)
	if len(issues) != 1 || issues[0].MessageID != MsgUnresolved {
		t.Fatalf("expected one unresolved urn finding, got %+v", issues)
	}
}

func TestAbsoluteURLResolvesByFullURL(t *testing.T) {
	bundle := `{
	  "resourceType":"Bundle","type":"collection",
	  "entry":[
	    {"fullUrl":"http://ex.org/fhir/Patient/1","resource":{"resourceType":"Patient","id":"1"}},
	    {"fullUrl":"http://ex.org/fhir/Encounter/2","resource":{"resourceType":"Encounter","id":"2","subject":{"reference":"http://ex.org/fhir/Patient/1"}}}
	  ]
	}`
	ix := index(bundle)
	if issues := Check([]byte(bundle), ix); len(issues) != 0 {
		t.Fatalf("absolute reference matching a fullUrl should resolve, got %+v", issues)
	}
}

func TestAbsoluteURLResolvesByTrailingTypeID(t *testing.T) {
	ix := index(`{"resourceType":"Patient","id":"p1"}`)
	// No matching fullUrl, but the trailing Type/id matches an indexed resource.
	issues := Check([]byte(`{"resourceType":"Observation","id":"o1","subject":{"reference":"https://server.example/fhir/Patient/p1"}}`), ix)
	if len(issues) != 0 {
		t.Fatalf("absolute reference with known trailing Type/id should resolve, got %+v", issues)
	}
}

func TestExternalAbsoluteReferenceIsInformation(t *testing.T) {
	ix := index(`{"resourceType":"Observation","id":"o1"}`)
	issues := Check([]byte(`{"resourceType":"Observation","id":"o1","subject":{"reference":"https://other.example/fhir/Patient/999"}}`), ix)
	if len(issues) != 1 {
		t.Fatalf("expected one external finding, got %+v", issues)
	}
	if issues[0].MessageID != MsgExternal || issues[0].Severity != "information" {
		t.Fatalf("expected ref:external information, got %+v", issues[0])
	}
}

func TestLogicalReferenceSkipped(t *testing.T) {
	// A reference by identifier only (no literal .reference) must not be flagged;
	// and a non-Type/id string is not treated as a literal reference.
	docs := []string{
		`{"resourceType":"Observation","id":"o1","subject":{"identifier":{"system":"http://x","value":"1"}}}`,
		`{"resourceType":"Observation","id":"o2","subject":{"reference":"not-a-type-id"}}`,
	}
	for _, d := range docs {
		if issues := Check([]byte(d), NewIndex()); len(issues) != 0 {
			t.Errorf("expected no findings for %q, got %+v", d, issues)
		}
	}
}

func TestCanonicalURLNotChecked(t *testing.T) {
	// baseDefinition is a canonical (url), not a Reference (.reference) — must be ignored.
	sd := `{"resourceType":"StructureDefinition","id":"sd1","url":"http://ex.org/sd","baseDefinition":"http://hl7.org/fhir/StructureDefinition/Patient"}`
	if issues := Check([]byte(sd), NewIndex()); len(issues) != 0 {
		t.Fatalf("canonical url must not be treated as a reference, got %+v", issues)
	}
}

func TestMultipleReferencesDeterministicOrder(t *testing.T) {
	ix := index(`{"resourceType":"Encounter","id":"e1"}`)
	doc := `{"resourceType":"Encounter","id":"e1","subject":{"reference":"Patient/x"},"serviceProvider":{"reference":"Organization/y"}}`
	first := Check([]byte(doc), ix)
	second := Check([]byte(doc), ix)
	if len(first) != 2 {
		t.Fatalf("expected 2 findings, got %+v", first)
	}
	for i := range first {
		if first[i].Location != second[i].Location {
			t.Fatalf("non-deterministic order: %v vs %v", first, second)
		}
	}
	// Sorted by location: Encounter.serviceProvider before Encounter.subject.
	if first[0].Location != "Encounter.serviceProvider.reference" {
		t.Errorf("unexpected first location %q", first[0].Location)
	}
}
