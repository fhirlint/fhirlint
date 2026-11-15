// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package txreplay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
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

// Which answers are kept (#661): every answer the server states as FHIR or
// registry JSON, whatever its status, and none that is about the way to the
// server (a gateway error) or isn't an answer at all (an HTML error page).
func TestRecorder_KeepsAnswersNotFailures(t *testing.T) {
	type answer struct {
		status int
		ct     string
		body   string
	}
	answers := map[string]answer{
		"/r4/CodeSystem/$lookup": {404, "application/fhir+json", `{"resourceType":"OperationOutcome","issue":[{"severity":"error","code":"not-found","details":{"text":"Unknown code 'LP7386-4'"}}]}`},
		"/r4/ValueSet/$expand":   {500, "application/fhir+json", `{"resourceType":"OperationOutcome","issue":[{"severity":"error","code":"exception","details":{"text":"ops|2026 is not held on this server"}}]}`},
		"/tx-reg/resolve":        {400, "application/json", `{"error":"Invalid value set URL format"}`},
		"/r4/ValueSet/x":         {422, "application/fhir+json", `{"resourceType":"OperationOutcome","id":"invalid"}`},
		"/r4/busy":               {503, "application/fhir+json", `{"resourceType":"OperationOutcome"}`},
		"/r4/misconfigured":      {404, "text/html", `<html><body>Not Found</body></html>`},
		"/r4/broken":             {500, "text/plain", `oops`},
	}
	base, store := recordingThrough(t, func(w http.ResponseWriter, r *http.Request) {
		a := answers[r.URL.Path]
		w.Header().Set("Content-Type", a.ct)
		w.WriteHeader(a.status)
		_, _ = io.WriteString(w, a.body)
	})
	for path, a := range answers {
		if status, _ := get(t, base, path); status != a.status {
			t.Errorf("%s: recording passes %d through, got %d", path, a.status, status)
		}
	}
	if store.Len() != 4 {
		t.Errorf("want the 404, 500, 400 and 422 answers kept, and the 503, HTML and text ones not: %d recorded", store.Len())
	}

	player := NewPlayer(store)
	pbase, err := player.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = player.Stop() }()
	for _, path := range []string{"/r4/CodeSystem/$lookup", "/r4/ValueSet/$expand", "/tx-reg/resolve", "/r4/ValueSet/x"} {
		if status, body := get(t, pbase, path); status != answers[path].status || !strings.Contains(body, strings.Split(answers[path].body, `"`)[1]) {
			t.Errorf("%s: replayed %d %s, want %d", path, status, body, answers[path].status)
		}
	}
	for _, path := range []string{"/r4/busy", "/r4/misconfigured", "/r4/broken"} {
		_, _ = get(t, pbase, path)
	}
	if n := len(player.Misses()); n != 3 {
		t.Errorf("the unstored answers are misses on replay, want 3, got %d", n)
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

func TestManifestCountsErrorAnswers(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i, status := range []int{200, 404, 500, 422} {
		in := &Interaction{Method: "GET", Path: "/r4/x", Query: strconv.Itoa(i), Status: status, Response: []byte(`{"resourceType":"OperationOutcome"}`)}
		if err := store.Put(in, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.WriteManifest(Manifest{Upstream: "https://tx.example.org"}); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m := reopened.ReadManifest(); m == nil || m.Entries != 4 || m.ErrorAnswers != 3 {
		t.Errorf("manifest: %+v", m)
	}
}
