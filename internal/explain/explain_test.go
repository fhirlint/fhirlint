package explain

import (
	"strings"
	"testing"
)

func TestLookup_KnownCaseInsensitive(t *testing.T) {
	for _, id := range []string{"dom-6", "DOM-6", " Dom-6 "} {
		r, ok := Lookup(id)
		if !ok {
			t.Fatalf("expected %q to be known", id)
		}
		if r.ID != "dom-6" {
			t.Errorf("expected canonical ID dom-6, got %q", r.ID)
		}
	}
}

func TestLookup_Unknown(t *testing.T) {
	if _, ok := Lookup("not-a-real-id"); ok {
		t.Error("expected unknown ID to return ok=false")
	}
}

func TestKnown(t *testing.T) {
	if !Known("ele-1") {
		t.Error("ele-1 should be known")
	}
	if Known("zzz-1") {
		t.Error("zzz-1 should not be known")
	}
}

func TestIDs_SortedAndCanonical(t *testing.T) {
	ids := IDs()
	if len(ids) == 0 {
		t.Fatal("expected some IDs")
	}
	for i := 1; i < len(ids); i++ {
		if ids[i-1] > ids[i] {
			t.Errorf("IDs not sorted: %q before %q", ids[i-1], ids[i])
		}
	}
	// Every listed ID resolves and round-trips to itself.
	for _, id := range ids {
		r, ok := Lookup(id)
		if !ok || r.ID != id {
			t.Errorf("ID %q does not resolve to a canonical rule (got %q, ok=%v)", id, r.ID, ok)
		}
	}
}

func TestFormat_ContainsKeySections(t *testing.T) {
	r, _ := Lookup("dom-6")
	out := Format(r)
	for _, want := range []string{
		"dom-6 — A resource should have narrative",
		"Defined in: FHIR R4 Core",
		"How to fix:",
		"Suppress if intentional:",
		"--suppress messageId:dom-6",
		"- messageId: dom-6",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Format output missing %q\n---\n%s", want, out)
		}
	}
}

func TestRulesWellFormed(t *testing.T) {
	for key, r := range rules {
		if key != strings.ToLower(key) {
			t.Errorf("rule key %q must be lowercase", key)
		}
		if strings.ToLower(r.ID) != key {
			t.Errorf("rule %q: ID field %q must match its map key once lowercased", key, r.ID)
		}
		if r.Title == "" || r.DefinedIn == "" || r.Description == "" || r.HowToFix == "" {
			t.Errorf("rule %q has an empty field", key)
		}
	}
}
