// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package refcheck

import (
	"testing"
)

// The reference shapes the main tests leave out (#691). Each row is what a
// realistic resource can carry; want is the message id reported, "" for none.
func TestReferenceShapes(t *testing.T) {
	ix := index(`{"resourceType":"Patient","id":"p1"}`)
	for _, tc := range []struct{ ref, want string }{
		// An absolute URL resolves by its trailing Type/id, whatever follows it.
		{"https://s.example/fhir/Patient/p1?_format=json", ""},
		{"https://s.example/fhir/Patient/p1#frag", ""},
		{"https://s.example/fhir/Patient/p1/", ""},
		{"https://s.example/fhir/Patient/p1/_history/2", ""},
		// One that doesn't end in a Type/id pair is external, not unresolved.
		{"https://s.example/", MsgExternal},
		{"https://s.example/fhir/patient/p1", MsgExternal},
		{"https://s.example/fhir/Patient/p2", MsgExternal},
		// Relative values that are no Type/id literal aren't checked: there is
		// nothing to resolve them against.
		{"patient/p1", ""},
		{"Patient/", ""},
		{"Patient", ""},
		{"   ", ""},
		// A relative literal with a version resolves without it.
		{"Patient/p1/_history/1", ""},
		{"Patient/p2/_history/1", MsgUnresolved},
	} {
		doc := `{"resourceType":"Observation","id":"o1","subject":{"reference":"` + tc.ref + `"}}`
		issues := check([]byte(doc), ix)
		got := ""
		if len(issues) > 0 {
			got = issues[0].MessageID
		}
		if got != tc.want || len(issues) > 1 {
			t.Errorf("%q: got %+v, want %q", tc.ref, issues, tc.want)
		}
	}
}

// Malformed input is ignored on both sides rather than read as "nothing
// resolves", and odd Bundle shapes don't stop the rest from being indexed.
func TestMalformedInputIsIgnored(t *testing.T) {
	ix := index(
		`{"resourceType":"Patient",`,
		`{"resourceType":"Bundle","entry":{"not":"a list"}}`,
		`{"resourceType":"Bundle","entry":["not an object",{"fullUrl":"urn:uuid:1","resource":{"resourceType":"Patient","id":"p9"}}]}`,
	)
	if issues, refs := CheckWithRefs([]byte(`{"resourceType":`), ix); issues != nil || refs != nil {
		t.Errorf("malformed JSON yields nothing: %+v %v", issues, refs)
	}
	doc := `{"resourceType":"Observation","subject":{"reference":"Patient/p9"},"focus":[{"reference":"urn:uuid:1"}]}`
	if issues := check([]byte(doc), ix); len(issues) != 0 {
		t.Errorf("entries after an odd one are still indexed: %+v", issues)
	}
}

// A resource without a resourceType still gets locations to point at.
func TestLocationWithoutResourceType(t *testing.T) {
	issues := check([]byte(`{"subject":{"reference":"Patient/zz"}}`), index())
	if len(issues) != 1 || issues[0].Location != "(resource).subject.reference" {
		t.Errorf("got %+v", issues)
	}
}
