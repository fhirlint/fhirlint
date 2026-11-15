// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package txreplay

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// The validator's per-run ids don't reach the key (#652). The requests are
// shaped like the ones validator 6.10.4 sent while validating the German
// packages, where every replay of them missed.

const mimeQuery = "inferSystem=true&code=image%2Fjpeg&displayLanguage=en&default-to-latest-version=true" +
	"&url=urn%3Auuid%3AUUID&valueSetVersion=1" +
	"&profile-url=http%3A%2F%2Fhl7.org%2Ffhir%2FExpansionProfile%2Fdc8fd4bc-091a-424a-8a3b-6198ef146891&diagnostics=true"

// mime fills in the run's uuid.
func mime(uuid string) string { return strings.Replace(mimeQuery, "UUID", uuid, 1) }

func languageBody(vsURL, vsID, system string) string {
	return `{"resourceType":"Parameters","parameter":[
	  {"name":"code","valueCode":"de-DE"},
	  {"name":"valueSet","resource":{"resourceType":"ValueSet","id":"` + vsID + `","url":"` + vsURL + `",
	    "compose":{"include":[{"system":"` + system + `"}]}}},
	  {"name":"profile-url","valueString":"http://hl7.org/fhir/ExpansionProfile/dc8fd4bc-091a-424a-8a3b-6198ef146891"}]}`
}

func TestKey_IgnoresPerRunIDs(t *testing.T) {
	// A generated value set named only by its urn:uuid, in the query.
	a := Key("GET", "/ValueSet/$validate-code", mime("2168cdcd-53d5-4cdb-b308-17dda8278ce9"), nil)
	b := Key("GET", "/ValueSet/$validate-code", mime("3E55D58C-BDC8-4944-9BB2-D3C3900BCEA8"), nil)
	if a != b {
		t.Error("two runs' urn:uuid in the query must give the same key")
	}
	if c := Key("GET", "/ValueSet/$validate-code", strings.Replace(mime("2168cdcd-53d5-4cdb-b308-17dda8278ce9"), "image%2Fjpeg", "application%2Fpdf", 1), nil); c == a {
		t.Error("another code is another request")
	}

	// An inline value set with a urn:uuid url, or a fresh uuid id.
	x := Key("POST", "/ValueSet/$validate-code", "", []byte(languageBody("urn:uuid:c1b3909f-9b74-4957-a767-c4a0bf1e0655", "all-languages", "urn:ietf:bcp:47")))
	y := Key("POST", "/ValueSet/$validate-code", "", []byte(languageBody("urn:uuid:9cd3a74e-36bd-3f17-90c9-df7c9eeb62cf", "all-languages", "urn:ietf:bcp:47")))
	if x != y {
		t.Error("two runs' urn:uuid in the body must give the same key")
	}
	p := Key("POST", "/ValueSet/$validate-code", "", []byte(languageBody("https://fhir.kbv.de/ValueSet/V", "fff2328f-9f63-4048-a39d-1499d0e036f3", "http://loinc.org")))
	q := Key("POST", "/ValueSet/$validate-code", "", []byte(languageBody("https://fhir.kbv.de/ValueSet/V", "25de1baf-e9f6-40ae-b3f7-8927b47918d4", "http://loinc.org")))
	if p != q {
		t.Error("an inline copy's fresh uuid id must not change the key")
	}
	// What the request is about still tells requests apart.
	if r := Key("POST", "/ValueSet/$validate-code", "", []byte(languageBody("https://fhir.kbv.de/ValueSet/V", "fff2328f-9f63-4048-a39d-1499d0e036f3", "http://snomed.info/sct"))); r == p {
		t.Error("another compose is another request")
	}
	if r := Key("POST", "/ValueSet/$validate-code", "", []byte(languageBody("https://fhir.kbv.de/ValueSet/W", "fff2328f-9f63-4048-a39d-1499d0e036f3", "http://loinc.org"))); r == p {
		t.Error("another canonical url is another request")
	}
	// A uuid that isn't a resource id, an element id say, is content.
	e1 := Key("POST", "/x", "", []byte(`{"resourceType":"Parameters","parameter":[{"name":"coding","valueCoding":{"id":"fff2328f-9f63-4048-a39d-1499d0e036f3","code":"1"}}]}`))
	e2 := Key("POST", "/x", "", []byte(`{"resourceType":"Parameters","parameter":[{"name":"coding","valueCoding":{"id":"25de1baf-e9f6-40ae-b3f7-8927b47918d4","code":"1"}}]}`))
	if e1 == e2 {
		t.Error("only a resource's own id is masked")
	}
}

// A request without per-run ids keys exactly as before, so a recording made
// by an earlier fhirlint still replays.
func TestKey_UnchangedWithoutPerRunIDs(t *testing.T) {
	body := []byte(validateCodeBody)
	query := "system=http://loinc.org&code=29463-7"
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s\n%s\n%s\n", "POST", "/r4/ValueSet/$validate-code", canonicalQuery(query))
	h.Write(canonicalJSON(body))
	if want := hex.EncodeToString(h.Sum(nil))[:32]; Key("post", "/r4/ValueSet/$validate-code", query, body) != want {
		t.Error("the key of a request without a uuid must not change")
	}
}

// End to end: recorded with one run's uuid, replayed with the next run's.
func TestReplay_MatchesTheNextRunsUUID(t *testing.T) {
	calls := 0
	up := fakeTx(t, &calls)
	defer up.Close()
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rec := NewRecorder(store, up.URL, nil)
	base, err := rec.Start()
	if err != nil {
		t.Fatal(err)
	}
	if status, _ := get(t, base, "/ValueSet/$validate-code?"+mime("2168cdcd-53d5-4cdb-b308-17dda8278ce9")); status != http.StatusOK {
		t.Fatalf("recording: %d", status)
	}
	_ = rec.Stop()

	player := NewPlayer(store)
	pbase, err := player.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = player.Stop() }()
	if status, _ := get(t, pbase, "/ValueSet/$validate-code?"+mime("3e55d58c-bdc8-4944-9bb2-d3c3900bcea8")); status != http.StatusOK || len(player.Misses()) != 0 {
		t.Errorf("the next run's request must replay: status %d, misses %v", status, player.Misses())
	}
}
