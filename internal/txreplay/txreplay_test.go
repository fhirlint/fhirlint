package txreplay

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const capabilityStatement = `{"resourceType":"CapabilityStatement","id":"tx"}`

const validateCodeBody = `{"resourceType":"Parameters","parameter":[
  {"name":"system","valueUri":"http://loinc.org"},
  {"name":"code","valueCode":"29463-7"}
]}`

// sameJSON compares two JSON documents by value. Recordings are stored
// pretty-printed so they stay reviewable when committed, so replayed bodies are
// re-indented rather than byte-identical to what the server originally sent.
func sameJSON(a, b string) bool {
	var av, bv interface{}
	if err := json.Unmarshal([]byte(a), &av); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(b), &bv); err != nil {
		return false
	}
	x, _ := json.Marshal(av)
	y, _ := json.Marshal(bv)
	return string(x) == string(y)
}

// fakeTx stands in for tx.fhir.org and counts what it was asked.
func fakeTx(t *testing.T, calls *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		w.Header().Set("Content-Type", "application/fhir+json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/metadata"):
			_, _ = io.WriteString(w, capabilityStatement)
		default:
			_, _ = io.WriteString(w, `{"resourceType":"Parameters","parameter":[{"name":"result","valueBoolean":true}]}`)
		}
	}))
}

func get(t *testing.T, base, path string) (int, string) {
	t.Helper()
	resp, err := http.Get(base + path) //nolint:gosec // loopback test server
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func post(t *testing.T, base, path, body string) (int, string) {
	t.Helper()
	resp, err := http.Post(base+path, "application/fhir+json", strings.NewReader(body)) //nolint:gosec // loopback test server
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestRecordThenReplay(t *testing.T) {
	calls := 0
	upstream := fakeTx(t, &calls)
	defer upstream.Close()
	dir := t.TempDir()

	// Record.
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	rec := NewRecorder(store, upstream.URL, nil)
	base, err := rec.Start()
	if err != nil {
		t.Fatal(err)
	}
	if status, body := get(t, base, "/r4/metadata"); status != 200 || !sameJSON(body, capabilityStatement) {
		t.Fatalf("record: metadata = %d %q", status, body)
	}
	if status, _ := post(t, base, "/r4/ValueSet/$validate-code", validateCodeBody); status != 200 {
		t.Fatalf("record: validate-code status %d", status)
	}
	if err := rec.Stop(); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("upstream should have been called twice, got %d", calls)
	}

	// Replay from a freshly opened store — this is what a later run does.
	upstream.Close()
	replayStore, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if replayStore.Len() != 2 {
		t.Fatalf("expected 2 recorded interactions, got %d", replayStore.Len())
	}
	player := NewPlayer(replayStore)
	pbase, err := player.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = player.Stop() }()

	if status, body := get(t, pbase, "/r4/metadata"); status != 200 || !sameJSON(body, capabilityStatement) {
		t.Errorf("replay: metadata = %d %q", status, body)
	}
	if status, _ := post(t, pbase, "/r4/ValueSet/$validate-code", validateCodeBody); status != 200 {
		t.Errorf("replay: validate-code status %d", status)
	}
	if calls != 2 {
		t.Errorf("replay must not touch the upstream; call count rose to %d", calls)
	}
	if len(player.Misses()) != 0 {
		t.Errorf("expected no misses, got %v", player.Misses())
	}
}

func TestReplayMissIsReportedWithDetail(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	player := NewPlayer(store)
	base, err := player.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = player.Stop() }()

	status, body := post(t, base, "/r4/ValueSet/$validate-code", validateCodeBody)
	if status != http.StatusNotFound {
		t.Errorf("a miss should not look like a successful validation, got status %d", status)
	}
	if !strings.Contains(body, "OperationOutcome") {
		t.Errorf("miss response should be an OperationOutcome, got %q", body)
	}

	misses := player.Misses()
	if len(misses) != 1 {
		t.Fatalf("expected exactly one miss, got %v", misses)
	}
	// The message has to name the code, otherwise the user cannot tell which
	// resource needs re-recording.
	if !strings.Contains(misses[0].Detail, "http://loinc.org") || !strings.Contains(misses[0].Detail, "29463-7") {
		t.Errorf("miss detail should name system and code, got %q", misses[0].Detail)
	}
}

func TestKeyIgnoresBodyKeyOrderAndQueryOrder(t *testing.T) {
	a := Key("POST", "/r4/ValueSet/$validate-code", "b=2&a=1", []byte(`{"x":1,"y":2}`))
	b := Key("post", "/r4/ValueSet/$validate-code", "a=1&b=2", []byte(`{"y":2,"x":1}`))
	if a != b {
		t.Errorf("logically identical requests must share a key: %s vs %s", a, b)
	}
}

func TestKeyDistinguishesDifferentCodes(t *testing.T) {
	a := Key("POST", "/r4/ValueSet/$validate-code", "", []byte(`{"code":"1234-5"}`))
	b := Key("POST", "/r4/ValueSet/$validate-code", "", []byte(`{"code":"6789-0"}`))
	if a == b {
		t.Error("different request bodies must not collide")
	}
}

func TestOpenMissingDirectoryIsEmptyNotAnError(t *testing.T) {
	store, err := Open(t.TempDir() + "/does-not-exist")
	if err != nil {
		t.Fatalf("opening a fresh recording path should succeed: %v", err)
	}
	if store.Len() != 0 {
		t.Errorf("expected an empty store, got %d entries", store.Len())
	}
}

func TestOpenCorruptRecordingFails(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(&Interaction{Method: "GET", Path: "/r4/metadata", Status: 200}, nil); err != nil {
		t.Fatal(err)
	}
	// Corrupt the file that was just written.
	names, _ := os.ReadDir(dir)
	if len(names) != 1 {
		t.Fatalf("expected one recording file, got %d", len(names))
	}
	if err := os.WriteFile(filepath.Join(dir, names[0].Name()), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Open(dir); err == nil {
		t.Error("a corrupt recording should be reported, not silently ignored")
	}
}

func TestManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(&Interaction{Method: "GET", Path: "/r4/metadata", Status: 200}, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteManifest(Manifest{Upstream: "https://tx.fhir.org", FHIRVersion: "4.0.1", Recorded: "2026-08-03"}); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Len() != 1 {
		t.Errorf("manifest must not be loaded as an interaction, got %d entries", reopened.Len())
	}
	m := reopened.ReadManifest()
	if m == nil || m.Upstream != "https://tx.fhir.org" || m.Entries != 1 {
		t.Errorf("manifest round-trip failed: %+v", m)
	}
}

func TestRecorderSurvivesUpstreamFailure(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Nothing listening on this port.
	rec := NewRecorder(store, "http://127.0.0.1:1", nil)
	base, err := rec.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rec.Stop() }()

	status, body := get(t, base, "/r4/metadata")
	if status != http.StatusBadGateway {
		t.Errorf("expected 502 when the terminology server is unreachable, got %d", status)
	}
	if !strings.Contains(body, "unreachable") {
		t.Errorf("error should say the server was unreachable, got %q", body)
	}
	if store.Len() != 0 {
		t.Error("a failed call must not be recorded")
	}
}

func TestReplayPreservesStatusAndContentType(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	in := &Interaction{
		Method:      "POST",
		Path:        "/r4/CodeSystem/$validate-code",
		Status:      422,
		ContentType: "application/fhir+json; charset=utf-8",
		Response:    json.RawMessage(`{"resourceType":"OperationOutcome"}`),
	}
	if err := store.Put(in, []byte(`{"a":1}`)); err != nil {
		t.Fatal(err)
	}

	player := NewPlayer(store)
	base, err := player.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = player.Stop() }()

	resp, err := http.Post(base+"/r4/CodeSystem/$validate-code", "application/fhir+json", strings.NewReader(`{"a":1}`)) //nolint:gosec // loopback test server
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 422 {
		t.Errorf("recorded status should be replayed, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != in.ContentType {
		t.Errorf("recorded content type should be replayed, got %q", ct)
	}
}

func TestWriteJARSettings_ExemptsExactURLIncludingPort(t *testing.T) {
	path, cleanup, err := WriteJARSettings("http://127.0.0.1:35119")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	data, err := os.ReadFile(path) //nolint:gosec // path returned by the function under test
	if err != nil {
		t.Fatal(err)
	}
	var got jarSettings
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("settings file is not valid JSON: %v\n%s", err, data)
	}
	if len(got.Servers) != 1 {
		t.Fatalf("expected one server entry, got %d", len(got.Servers))
	}
	s := got.Servers[0]
	// The port has to be in there: prefix matching was tightened for
	// CVE-2026-34361, so a host-only entry no longer covers a port-bearing URL.
	if s.URL != "http://127.0.0.1:35119" {
		t.Errorf("url = %q, want the full loopback URL with its port", s.URL)
	}
	if !s.AllowHTTP {
		t.Error("allowHttp must be set, or validator 6.10.0+ refuses the plain-HTTP endpoint")
	}
	if !s.AllowPrivateNetwork {
		t.Error("allowPrivateNetwork must be set for a loopback destination")
	}
}

func TestWriteJARSettings_CleanupRemovesTheFile(t *testing.T) {
	path, cleanup, err := WriteJARSettings("http://127.0.0.1:1234")
	if err != nil {
		t.Fatal(err)
	}
	cleanup()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("settings file should be gone after cleanup, stat gave %v", err)
	}
}
