package refcheck

import (
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
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
	issues := check([]byte(`{"resourceType":"Encounter","id":"e1","subject":{"reference":"Patient/p1"}}`), ix)
	if len(issues) != 0 {
		t.Fatalf("expected no findings for resolved reference, got %+v", issues)
	}
}

func TestUnresolvedRelativeReference(t *testing.T) {
	ix := index(`{"resourceType":"Encounter","id":"e1","subject":{"reference":"Patient/missing"}}`)
	issues := check([]byte(`{"resourceType":"Encounter","id":"e1","subject":{"reference":"Patient/missing"}}`), ix)
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
	issues := check([]byte(`{"resourceType":"Observation","id":"o1","subject":{"reference":"Patient/p1/_history/3"}}`), ix)
	if len(issues) != 0 {
		t.Fatalf("history-versioned reference should resolve, got %+v", issues)
	}
}

func TestContainedReference(t *testing.T) {
	resolved := `{"resourceType":"Observation","id":"o1","contained":[{"resourceType":"Patient","id":"pat"}],"subject":{"reference":"#pat"}}`
	if issues := check([]byte(resolved), NewIndex()); len(issues) != 0 {
		t.Fatalf("resolved contained ref should have no findings, got %+v", issues)
	}

	broken := `{"resourceType":"Observation","id":"o1","contained":[{"resourceType":"Patient","id":"pat"}],"subject":{"reference":"#nope"}}`
	issues := check([]byte(broken), NewIndex())
	if len(issues) != 1 || issues[0].MessageID != MsgUnresolved || issues[0].Severity != "error" {
		t.Fatalf("expected one unresolved contained finding, got %+v", issues)
	}
}

func TestSelfContainedHashSkipped(t *testing.T) {
	if issues := check([]byte(`{"resourceType":"Patient","id":"p1","link":[{"other":{"reference":"#"}}]}`), NewIndex()); len(issues) != 0 {
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
	if issues := check([]byte(bundle), ix); len(issues) != 0 {
		t.Fatalf("resolved urn reference should have no findings, got %+v", issues)
	}

	broken := strings.Replace(bundle, "urn:uuid:aaa\"}", "urn:uuid:zzz\"}", 1)
	ix2 := index(broken)
	issues := check([]byte(broken), ix2)
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
	if issues := check([]byte(bundle), ix); len(issues) != 0 {
		t.Fatalf("absolute reference matching a fullUrl should resolve, got %+v", issues)
	}
}

func TestAbsoluteURLResolvesByTrailingTypeID(t *testing.T) {
	ix := index(`{"resourceType":"Patient","id":"p1"}`)
	// No matching fullUrl, but the trailing Type/id matches an indexed resource.
	issues := check([]byte(`{"resourceType":"Observation","id":"o1","subject":{"reference":"https://server.example/fhir/Patient/p1"}}`), ix)
	if len(issues) != 0 {
		t.Fatalf("absolute reference with known trailing Type/id should resolve, got %+v", issues)
	}
}

func TestExternalAbsoluteReferenceIsInformation(t *testing.T) {
	ix := index(`{"resourceType":"Observation","id":"o1"}`)
	issues := check([]byte(`{"resourceType":"Observation","id":"o1","subject":{"reference":"https://other.example/fhir/Patient/999"}}`), ix)
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
		if issues := check([]byte(d), NewIndex()); len(issues) != 0 {
			t.Errorf("expected no findings for %q, got %+v", d, issues)
		}
	}
}

func TestCanonicalURLNotChecked(t *testing.T) {
	// baseDefinition is a canonical (url), not a Reference (.reference) — must be ignored.
	sd := `{"resourceType":"StructureDefinition","id":"sd1","url":"http://ex.org/sd","baseDefinition":"http://hl7.org/fhir/StructureDefinition/Patient"}`
	if issues := check([]byte(sd), NewIndex()); len(issues) != 0 {
		t.Fatalf("canonical url must not be treated as a reference, got %+v", issues)
	}
}

func TestMultipleReferencesDeterministicOrder(t *testing.T) {
	ix := index(`{"resourceType":"Encounter","id":"e1"}`)
	doc := `{"resourceType":"Encounter","id":"e1","subject":{"reference":"Patient/x"},"serviceProvider":{"reference":"Organization/y"}}`
	first := check([]byte(doc), ix)
	second := check([]byte(doc), ix)
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

// CheckWithRefs hands back the reference values behind its unresolved findings,
// so a caller can reconcile them with what the validator said about the same
// reference. Only unresolved ones: an external reference is a different
// statement and nothing duplicates it (#436).
func TestCheckWithRefsCollectsUnresolvedOnly(t *testing.T) {
	ix := NewIndex()
	ix.Add([]byte(`{"resourceType":"Patient","id":"p1"}`))

	doc := `{"resourceType":"Encounter","id":"e1",
	  "subject":{"reference":"Patient/p1"},
	  "partOf":{"reference":"Encounter/missing"},
	  "serviceProvider":{"reference":"https://other.example/fhir/Organization/9"}}`

	issues, refs := CheckWithRefs([]byte(doc), ix)

	if len(issues) != 2 {
		t.Fatalf("got %d issues, want 2 (one unresolved, one external): %+v", len(issues), issues)
	}
	if len(refs) != 1 {
		t.Fatalf("got refs %v, want just the unresolved one", refs)
	}
	if _, ok := refs["Encounter/missing"]; !ok {
		t.Errorf("refs = %v, want Encounter/missing", refs)
	}
}

// The returned value is the trimmed reference, so it matches the text the
// validator's message carries rather than the raw JSON string.
func TestCheckWithRefsTrimsTheReference(t *testing.T) {
	_, refs := CheckWithRefs([]byte(`{"resourceType":"Observation","id":"o1","subject":{"reference":"  Patient/gone  "}}`), NewIndex())

	if _, ok := refs["Patient/gone"]; !ok {
		t.Errorf("refs = %v, want the trimmed Patient/gone", refs)
	}
}

// Check stays the thin wrapper it was, so existing callers are unaffected.
func TestCheckMatchesCheckWithRefs(t *testing.T) {
	doc := []byte(`{"resourceType":"Observation","id":"o1","subject":{"reference":"Patient/gone"}}`)

	plain := check(doc, NewIndex())
	withRefs, _ := CheckWithRefs(doc, NewIndex())

	if len(plain) != len(withRefs) || len(plain) != 1 {
		t.Fatalf("Check = %+v, CheckWithRefs = %+v", plain, withRefs)
	}
	if plain[0] != withRefs[0] {
		t.Errorf("Check and CheckWithRefs disagree:\n%+v\n%+v", plain[0], withRefs[0])
	}
}

// check is CheckWithRefs without the reference values.
func check(resourceJSON []byte, ix *Index) []validator.Issue {
	issues, _ := CheckWithRefs(resourceJSON, ix)
	return issues
}

// A "#id" inside a contained resource points at a sibling in the container;
// contained resources have no contained resources of their own (#577, found on
// the core examples, e.g. PlanDefinition-example-cardiology-os).
func TestContainedSiblingReference(t *testing.T) {
	res := `{"resourceType":"PlanDefinition","id":"pd","contained":[
		{"resourceType":"ActivityDefinition","id":"ad","productReference":{"reference":"#med"}},
		{"resourceType":"Medication","id":"med"}],
		"action":[{"definitionCanonical":"#ad"}]}`
	if issues := check([]byte(res), NewIndex()); len(issues) != 0 {
		t.Fatalf("sibling contained reference should resolve, got %+v", issues)
	}
	broken := strings.Replace(res, `"#med"`, `"#nope"`, 1)
	issues := check([]byte(broken), NewIndex())
	if len(issues) != 1 || issues[0].Location != "PlanDefinition.contained[0].productReference.reference" {
		t.Fatalf("missing sibling should still be reported once, got %+v", issues)
	}
}

// The entries of a Bundle nested in a Bundle entry, or contained in another
// resource, resolve against each other (#577, found on the core examples'
// Bundle of Bundles and SDC's extraction templates).
func TestNestedBundleEntriesAreIndexed(t *testing.T) {
	outer := `{"resourceType":"Bundle","id":"outer","type":"collection","entry":[
		{"fullUrl":"https://example.com/base/Bundle/3","resource":{"resourceType":"Bundle","id":"3","type":"collection","entry":[
			{"fullUrl":"https://example.com/base/DiagnosticReport/3","resource":{"resourceType":"DiagnosticReport","id":"3","result":[{"reference":"Observation/o1"}]}},
			{"fullUrl":"https://example.com/base/Observation/o1","resource":{"resourceType":"Observation","id":"o1"}}]}}]}`
	if issues := check([]byte(outer), index(outer)); len(issues) != 0 {
		t.Fatalf("reference into a nested Bundle should resolve, got %+v", issues)
	}

	template := `{"resourceType":"Questionnaire","id":"q","contained":[{"resourceType":"Bundle","id":"tpl","type":"transaction","entry":[
		{"fullUrl":"urn:uuid:6f6177d2-13ee-4d27-b0e8-3eaf663dd031","resource":{"resourceType":"Patient"}},
		{"fullUrl":"urn:uuid:7a1b","resource":{"resourceType":"Observation","subject":{"reference":"urn:uuid:6f6177d2-13ee-4d27-b0e8-3eaf663dd031"}}}]}]}`
	if issues := check([]byte(template), index(template)); len(issues) != 0 {
		t.Fatalf("urn reference inside a contained Bundle should resolve, got %+v", issues)
	}
}

// DetectedIssue.reference is a uri element of the resource, not the reference
// of a Reference datatype (#577).
func TestResourceLevelReferenceElementSkipped(t *testing.T) {
	res := `{"resourceType":"DetectedIssue","id":"d","reference":"http://www.tmhp.com/guidelines.pdf","patient":{"reference":"Patient/missing"}}`
	issues := check([]byte(res), NewIndex())
	if len(issues) != 1 || issues[0].Location != "DetectedIssue.patient.reference" {
		t.Fatalf("want only the patient reference, got %+v", issues)
	}
}
