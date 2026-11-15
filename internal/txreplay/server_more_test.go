// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package txreplay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The recorder's and player's edges (#633).

func TestMissString(t *testing.T) {
	for _, tc := range []struct {
		m    Miss
		want string
	}{
		{Miss{Method: "GET", Path: "/r4/metadata"}, "GET /r4/metadata"},
		{Miss{Method: "POST", Path: "/r4/ValueSet/$validate-code", Query: "x=1", Detail: "system http://loinc.org, code 1"},
			"POST /r4/ValueSet/$validate-code?x=1 (system http://loinc.org, code 1)"},
	} {
		if got := tc.m.String(); got != tc.want {
			t.Errorf("%q, want %q", got, tc.want)
		}
	}
}

// describe names what a request asked about, for the miss message.
func TestDescribe(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{`{"resourceType":"Parameters","parameter":[{"name":"codeableConcept","valueCodeableConcept":{"coding":[
			{"system":"http://snomed.info/sct","code":"1"},{"system":"http://loinc.org","code":"2"}]}}]}`,
			"system http://snomed.info/sct, code 1, system http://loinc.org, code 2"},
		{`{"parameter":[{"name":"coding","valueCoding":{"system":"http://loinc.org","code":"3"}}]}`, "system http://loinc.org, code 3"},
		{`{"parameter":[{"name":"url","valueUri":"http://vs"},{"name":"version","valueString":"1.0"},{"name":"display","valueString":"ignored"}]}`, "url http://vs, version 1.0"},
		{`not json`, ""},
		{`[1,2]`, ""},
		{``, ""},
	} {
		if got := describe([]byte(tc.body)); got != tc.want {
			t.Errorf("describe(%s) = %q, want %q", tc.body, got, tc.want)
		}
	}
}

// recordingThrough starts a recorder in front of upstream and returns its URL
// and store.
func recordingThrough(t *testing.T, upstream http.HandlerFunc) (string, *Store) {
	t.Helper()
	up := httptest.NewServer(upstream)
	t.Cleanup(up.Close)
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rec := NewRecorder(store, up.URL+"/", nil) // the trailing slash is dropped
	base, err := rec.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rec.Stop() })
	return base, store
}

// A server error is passed to the validator as it is, and never recorded: it
// would be replayed forever as the server's answer. A 422 is an answer.
func TestRecorder_KeepsAnswersNotFailures(t *testing.T) {
	base, store := recordingThrough(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/r4/broken":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"resourceType":"OperationOutcome"}`)
		default:
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = io.WriteString(w, `{"resourceType":"OperationOutcome","id":"invalid"}`)
		}
	})
	if status, _ := get(t, base, "/r4/broken"); status != http.StatusInternalServerError || store.Len() != 0 {
		t.Errorf("a 500 is passed through, not kept: status %d, %d recorded", status, store.Len())
	}
	if status, _ := get(t, base, "/r4/ValueSet/x"); status != http.StatusUnprocessableEntity || store.Len() != 1 {
		t.Errorf("a 422 is an answer and is kept: status %d, %d recorded", status, store.Len())
	}
}

// The query goes upstream, and an answer that isn't JSON is replayed as the
// text it was, with its content type.
func TestRecorder_QueryAndTextAnswers(t *testing.T) {
	var gotQuery string
	base, store := recordingThrough(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "plain answer")
	})
	if status, body := get(t, base, "/r4/CodeSystem/$lookup?system=http://loinc.org&code=1"); status != http.StatusOK || body != "plain answer" {
		t.Fatalf("recording: %d %q", status, body)
	}
	if gotQuery != "system=http://loinc.org&code=1" {
		t.Errorf("the query must reach the terminology server, got %q", gotQuery)
	}

	player := NewPlayer(store)
	pbase, err := player.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = player.Stop() }()
	resp, err := http.Get(pbase + "/r4/CodeSystem/$lookup?system=http://loinc.org&code=1") //nolint:gosec // loopback test server
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "plain answer" || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/plain") {
		t.Errorf("replayed %q as %s", body, resp.Header.Get("Content-Type"))
	}
}

// A recording without a status or content type, written by hand or by an old
// fhirlint, replays as a 200 FHIR JSON answer.
func TestReplay_DefaultsForSparseRecordings(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(&Interaction{Method: "GET", Path: "/r4/metadata", Response: []byte(capabilityStatement)}, nil); err != nil {
		t.Fatal(err)
	}
	player := NewPlayer(store)
	base, err := player.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = player.Stop() }()
	resp, err := http.Get(base + "/r4/metadata") //nolint:gosec // loopback test server
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/fhir+json" {
		t.Errorf("status %d, content type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
}

func TestStopBeforeStart(t *testing.T) {
	if err := NewPlayer(nil).Stop(); err != nil {
		t.Errorf("stopping a server that never started: %v", err)
	}
}
