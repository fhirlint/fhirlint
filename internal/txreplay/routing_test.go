package txreplay

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeEcosystem is one TLS server playing the registry and a routed server:
// the registry names it as authoritative, and tx.fhir.org as a candidate.
func fakeEcosystem(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var routedCalls atomic.Int32
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/tx-reg/resolve"):
			if r.Header.Get("Authorization") != "" {
				t.Error("the registry got the upstream credential")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"formatVersion": "1",
				"authoritative": []any{map[string]any{"server-name": "DE", "url": srv.URL + "/fhir"}},
				"candidates":    []any{map[string]any{"url": "http://tx.fhir.org/r4"}},
			})
		case strings.HasPrefix(r.URL.Path, "/fhir/"):
			routedCalls.Add(1)
			if r.Header.Get("Authorization") != "" {
				t.Error("a routed server got the upstream credential")
			}
			w.Header().Set("Content-Type", "application/fhir+json")
			_, _ = io.WriteString(w, `{"resourceType":"Parameters","parameter":[{"name":"result","valueBoolean":true}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &routedCalls
}

func getURL(t *testing.T, u string) (int, string) {
	t.Helper()
	resp, err := http.Get(u) //nolint:gosec,noctx // test against a local server
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestRegistryRouting_RecordAndReplay(t *testing.T) {
	eco, routedCalls := fakeEcosystem(t)
	ecoHost := strings.TrimPrefix(eco.URL, "https://")
	dir := t.TempDir()

	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	rec := NewRecorder(store, "https://tx.example.invalid", eco.Client()).
		WithUpstreamHeaders(map[string]string{"Authorization": "Bearer secret"}).
		WithRegistry(eco.URL + "/tx-reg")
	base, err := rec.Start()
	if err != nil {
		t.Fatal(err)
	}

	// The registry answer comes back pointing at the recorder.
	status, body := getURL(t, base+"/tx-reg/resolve?system="+url.QueryEscape("http://snomed.info/sct")+"&nocache=111")
	if status != 200 || !strings.Contains(body, base+"/route/"+ecoHost+"/fhir") {
		t.Fatalf("registry answer %d: %s", status, body)
	}
	if !strings.Contains(body, "http://tx.fhir.org/r4") {
		t.Errorf("tx.fhir.org was rewritten, but the validator substitutes it itself: %s", body)
	}
	// The routed server is reached through /route, without the credential.
	if status, _ := getURL(t, base+"/route/"+ecoHost+"/fhir/CodeSystem/$validate-code?code=1"); status != 200 || routedCalls.Load() != 1 {
		t.Fatalf("route: %d, calls %d", status, routedCalls.Load())
	}
	// Anything the registry did not name is refused: no open proxy.
	if status, _ := getURL(t, base+"/route/example.com/fhir/metadata"); status != http.StatusForbidden {
		t.Errorf("unnamed host: %d", status)
	}
	if got := rec.RoutedHosts(); len(got) != 1 || got[0] != ecoHost {
		t.Errorf("RoutedHosts = %v", got)
	}
	_ = rec.Stop()

	// Replay: a different nocache, a different port, no network.
	store, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	player := NewPlayer(store).WithRegistry("")
	pbase, err := player.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = player.Stop() }()
	status, body = getURL(t, pbase+"/tx-reg/resolve?system="+url.QueryEscape("http://snomed.info/sct")+"&nocache=999")
	if status != 200 || !strings.Contains(body, pbase+"/route/"+ecoHost+"/fhir") {
		t.Fatalf("replayed registry answer %d: %s", status, body)
	}
	eco.Close()
	if status, body := getURL(t, pbase+"/route/"+ecoHost+"/fhir/CodeSystem/$validate-code?code=1"); status != 200 || !strings.Contains(body, "valueBoolean") {
		t.Errorf("replayed route %d: %s", status, body)
	}
	if len(player.Misses()) != 0 {
		t.Errorf("misses: %v", player.Misses())
	}
}

func TestRegistryQuery_DropsNocache(t *testing.T) {
	if registryQuery("system=a&nocache=1") != registryQuery("nocache=2&system=a") {
		t.Error("nocache changes the key")
	}
	if registryQuery("system=a") == registryQuery("system=b") {
		t.Error("system no longer keys")
	}
}

func TestRewriteRegistryAnswer(t *testing.T) {
	in := `{"authoritative":[{"url":"https://tx.fhir.de/fhir"},{"url":"http://plain.example/fhir"}],` +
		`"candidates":[{"url":"https://tx.fhir.org/r4"},{"url":"https://user@evil.example/x"},{"url":5}],"other":"https://x.example"}`
	out, hosts := rewriteRegistryAnswer([]byte(in), "http://127.0.0.1:9")
	s := string(out)
	if !strings.Contains(s, `"http://127.0.0.1:9/route/tx.fhir.de/fhir"`) || len(hosts) != 1 || hosts[0] != "tx.fhir.de" {
		t.Errorf("rewrite: %s %v", s, hosts)
	}
	for _, kept := range []string{"http://plain.example/fhir", "https://tx.fhir.org/r4", "https://user@evil.example/x", `"other":"https://x.example"`} {
		if !strings.Contains(s, kept) {
			t.Errorf("%s was changed: %s", kept, s)
		}
	}
	if out, hosts := rewriteRegistryAnswer([]byte("not json"), "b"); string(out) != "not json" || hosts != nil {
		t.Error("unreadable answer changed")
	}
}

func TestRouteTarget(t *testing.T) {
	host, target, ok := routeTarget("/route/tx.fhir.de/fhir/CodeSystem/$validate-code", "a=b")
	if !ok || host != "tx.fhir.de" || target != "https://tx.fhir.de/fhir/CodeSystem/$validate-code?a=b" {
		t.Errorf("%q %q %v", host, target, ok)
	}
	if _, _, ok := routeTarget("/route/", ""); ok {
		t.Error("empty host accepted")
	}
}

// FuzzRewriteRegistryAnswer: registry answers come from the network. Any body
// is handled without panicking, and a rewritten URL only ever points at base.
func FuzzRewriteRegistryAnswer(f *testing.F) {
	f.Add([]byte(`{"authoritative":[{"url":"https://tx.fhir.de/fhir"}]}`))
	f.Add([]byte(`{"candidates":[{"url":"https://a:1/%zz"}]}`))
	f.Fuzz(func(t *testing.T, body []byte) {
		out, hosts := rewriteRegistryAnswer(body, "http://127.0.0.1:1")
		if len(hosts) > 0 && !strings.Contains(string(out), "http://127.0.0.1:1/route/") {
			t.Fatalf("hosts %v but no route in %s", hosts, out)
		}
	})
}

func TestSplitUpstream(t *testing.T) {
	for in, want := range map[string][2]string{
		"https://tx.fhir.org/r4":  {"https://tx.fhir.org", "/r4"},
		"https://tx.fhir.org/r5/": {"https://tx.fhir.org", "/r5"},
		"https://tx.example.org":  {"https://tx.example.org", ""},
		"not a url":               {"not a url", ""},
	} {
		if o, p := SplitUpstream(in); o != want[0] || p != want[1] {
			t.Errorf("SplitUpstream(%q) = %q, %q", in, o, p)
		}
	}
}
