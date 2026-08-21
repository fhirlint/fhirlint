package validator

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestServerArgs(t *testing.T) {
	cfg := ServerConfig{
		Port:                8080,
		FHIRVersion:         "4.0.1",
		IGs:                 []string{"hl7.fhir.us.core#9.0.0"},
		Profiles:            []string{"http://example.org/sd/Foo"},
		NoTerminologyServer: true,
	}
	args := serverArgs("/path/validator.jar", cfg)
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"-jar /path/validator.jar server 8080",
		"-version 4.0.1",
		"-tx n/a",
		"-ig hl7.fhir.us.core#9.0.0",
		"-profile http://example.org/sd/Foo",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q; got %q", want, joined)
		}
	}
}

func TestServerArgsTerminologyServer(t *testing.T) {
	args := serverArgs("j", ServerConfig{Port: 1, FHIRVersion: "4.0.1", TerminologyServer: "https://tx.example", TxCache: "/cache"})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-tx https://tx.example") {
		t.Errorf("expected tx server arg, got %q", joined)
	}
	if !strings.Contains(joined, "-txCache /cache") {
		t.Errorf("expected txCache arg, got %q", joined)
	}
}

const okOperationOutcome = `{"resourceType":"OperationOutcome","issue":[{"severity":"error","code":"invariant","details":{"text":"boom"},"expression":["Patient.name"]}]}`

func TestValidateBytesViaServer(t *testing.T) {
	var gotPath, gotQuery, gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotContentType = r.Header.Get("Content-Type")
		_, _ = io.WriteString(w, okOperationOutcome)
	}))
	defer srv.Close()

	res, err := validateBytesViaServer(srv.Client(), srv.URL, []byte(`{"resourceType":"Patient"}`), "patient.json", []string{"http://example.org/Foo"})
	if err != nil {
		t.Fatalf("validateBytesViaServer: %v", err)
	}
	if gotPath != "/validateResource" {
		t.Errorf("expected /validateResource, got %q", gotPath)
	}
	if !strings.Contains(gotQuery, "profile=http") {
		t.Errorf("expected profile query, got %q", gotQuery)
	}
	if gotContentType != "application/fhir+json" {
		t.Errorf("expected json content type, got %q", gotContentType)
	}
	if res.Valid {
		t.Error("expected invalid result (error issue present)")
	}
	if len(res.Issues) != 1 || res.Issues[0].Message != "boom" {
		t.Fatalf("unexpected issues: %+v", res.Issues)
	}
	if res.Filename != "patient.json" {
		t.Errorf("expected filename patient.json, got %q", res.Filename)
	}
}

func TestValidateBytesViaServerXMLContentType(t *testing.T) {
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		_, _ = io.WriteString(w, `{"resourceType":"OperationOutcome","issue":[]}`)
	}))
	defer srv.Close()

	if _, err := validateBytesViaServer(srv.Client(), srv.URL, []byte(`  <Patient/>`), "p.xml", nil); err != nil {
		t.Fatalf("validateBytesViaServer: %v", err)
	}
	if gotContentType != "application/fhir+xml" {
		t.Errorf("expected xml content type, got %q", gotContentType)
	}
}

func TestValidateBytesViaServerHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	if _, err := validateBytesViaServer(srv.Client(), srv.URL, []byte(`{}`), "x.json", nil); err == nil {
		t.Fatal("expected error on HTTP 500")
	}
}

func TestReadyWriter(t *testing.T) {
	var sink strings.Builder
	rw := &readyWriter{w: &sink, marker: serverReadyMarker, ready: make(chan struct{})}
	_, _ = rw.Write([]byte("loading packages...\n"))
	select {
	case <-rw.ready:
		t.Fatal("ready closed too early")
	default:
	}
	_, _ = rw.Write([]byte("FHIR Validator HTTP Service started on 127.0.0.1:8080\n"))
	select {
	case <-rw.ready:
	case <-time.After(time.Second):
		t.Fatal("ready not closed after marker written")
	}
	if !strings.Contains(sink.String(), "loading packages") {
		t.Error("writer did not tee output")
	}
}

func TestLooksLikeXML(t *testing.T) {
	cases := map[string]bool{
		`{"a":1}`:        false,
		"  \n<Patient/>": true,
		"":               false,
		"  {}":           false,
	}
	for in, want := range cases {
		if got := looksLikeXML([]byte(in)); got != want {
			t.Errorf("looksLikeXML(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestServerArgs_NoHTTPAccess(t *testing.T) {
	args := serverArgs("jar", ServerConfig{Port: 8080, FHIRVersion: "4.0.1", Offline: true, NoTerminologyServer: true})
	if !containsArg(args, "-no-http-access") {
		t.Errorf("offline server did not block the JAR's network: %v", args)
	}

	// The replay server lives on loopback, which the block would cut off too.
	args = serverArgs("jar", ServerConfig{Port: 8080, FHIRVersion: "4.0.1", Offline: true, TerminologyServer: "http://127.0.0.1:8081"})
	if containsArg(args, "-no-http-access") {
		t.Errorf("replay server passed -no-http-access, blocking its own terminology: %v", args)
	}

	args = serverArgs("jar", ServerConfig{Port: 8080, FHIRVersion: "4.0.1"})
	if containsArg(args, "-no-http-access") {
		t.Errorf("ordinary server blocked the JAR's network: %v", args)
	}
}
