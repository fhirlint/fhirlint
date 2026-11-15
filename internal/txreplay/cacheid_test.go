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

// cacheSessionServer behaves like tx.fhir.org's terminology session (#673):
// $cache-control?mode=start issues a cache id, a POST under it holds a value
// set, and a GET that names the value set only by URL finds it under the same
// id, and gets 422 "not found" without it.
func cacheSessionServer(t *testing.T) *httptest.Server {
	t.Helper()
	const id = "tx2.test.1"
	held := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/fhir+json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/$cache-control"):
			_, _ = io.WriteString(w, `{"resourceType":"Parameters","parameter":[{"name":"cache-id","valueId":"`+id+`"}]}`)
		case r.Method == http.MethodPost && r.Header.Get("X-Cache-Id") == id:
			held = true
			_, _ = io.WriteString(w, `{"resourceType":"Parameters","parameter":[{"name":"result","valueBoolean":true}]}`)
		case r.Method == http.MethodGet && held && r.Header.Get("X-Cache-Id") == id:
			_, _ = io.WriteString(w, `{"resourceType":"Parameters","parameter":[{"name":"result","valueBoolean":true}]}`)
		default:
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = io.WriteString(w, `{"resourceType":"OperationOutcome","issue":[{"severity":"error","code":"not-found","details":{"text":"A definition for the value Set 'http://fhir.de/ValueSet/UcumVitalsCommonDE' could not be found"}}]}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func send(t *testing.T, method, url, cacheID, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/fhir+json")
	if cacheID != "" {
		req.Header.Set("X-Cache-Id", cacheID)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestRecorder_ForwardsTheValidatorsCacheSession(t *testing.T) {
	up := cacheSessionServer(t)
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rec := NewRecorder(store, up.URL, nil)
	base, err := rec.Start()
	if err != nil {
		t.Fatal(err)
	}
	const get = "/ValueSet/$validate-code?url=http%3A%2F%2Ffhir.de%2FValueSet%2FUcumVitalsCommonDE&code=Cel"
	_, _ = send(t, http.MethodPost, base+"/$cache-control?mode=start", "", `{"resourceType":"Parameters"}`)
	_, _ = send(t, http.MethodPost, base+"/ValueSet/$validate-code", "tx2.test.1", `{"resourceType":"Parameters","parameter":[{"name":"tx-resource","resource":{"resourceType":"ValueSet","url":"http://fhir.de/ValueSet/UcumVitalsCommonDE"}}]}`)
	status, body := send(t, http.MethodGet, base+get, "tx2.test.1", "")
	if status != http.StatusOK || !strings.Contains(body, `"valueBoolean":true`) {
		t.Fatalf("the GET in the validator's session must reach the server with its cache id: %d %s", status, body)
	}
	_ = rec.Stop()

	// The replay answers the same GET from the recording, under the next run's
	// cache id, which the key ignores.
	player := NewPlayer(store)
	pbase, err := player.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = player.Stop() }()
	if status, body := send(t, http.MethodGet, pbase+get, "tx1.other.2", ""); status != http.StatusOK || !strings.Contains(body, `"valueBoolean":true`) {
		t.Errorf("replay: %d %s", status, body)
	}
}
