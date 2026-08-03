package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fhirlint/fhirlint/internal/validator"
)

// fakeBackend returns canned results and counts calls.
type fakeBackend struct {
	mu      sync.Mutex
	calls   int
	result  *validator.Result
	err     error
	lastArg []byte
}

func (f *fakeBackend) ValidateContent(content []byte, _ string) (*validator.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastArg = append([]byte(nil), content...)
	return f.result, f.err
}

func (f *fakeBackend) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type fakeSuppressor struct {
	mu  sync.Mutex
	ids []string
	err error
}

func (f *fakeSuppressor) Suppress(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.ids = append(f.ids, id)
	return nil
}

// session drives a server over in-memory pipes.
type session struct {
	t       *testing.T
	in      *io.PipeWriter
	out     *bufferedReader
	done    chan error
	backend *fakeBackend
}

// bufferedReader reads framed messages from the server's output.
type bufferedReader struct {
	c *conn
}

func newSession(t *testing.T, backend *fakeBackend, sup Suppressor) *session {
	t.Helper()
	srvIn, clientW := io.Pipe()  // test writes  -> server reads
	clientR, srvOut := io.Pipe() // server writes -> test reads

	srv := NewServer(srvIn, srvOut, backend, sup, io.Discard, "test")
	done := make(chan error, 1)
	go func() { done <- srv.Run() }()

	s := &session{
		t:       t,
		in:      clientW,
		out:     &bufferedReader{c: newConn(clientR, io.Discard)},
		done:    done,
		backend: backend,
	}
	t.Cleanup(func() {
		_ = clientW.Close()
		// Unblock any reply still queued on the unbuffered pipe, or the server
		// stays stuck in a write nobody is reading.
		_ = clientR.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("server did not stop after its input closed")
		}
		_ = srvOut.Close()
	})
	return s
}

func (s *session) send(t *testing.T, method string, id interface{}, params interface{}) {
	t.Helper()
	body := map[string]interface{}{"jsonrpc": "2.0", "method": method}
	if id != nil {
		body["id"] = id
	}
	if params != nil {
		body["params"] = params
	}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintf(s.in, "Content-Length: %d\r\n\r\n%s", len(data), data); err != nil {
		t.Fatal(err)
	}
}

// expect reads messages until one matches method, failing on timeout. An empty
// method means "the next response", skipping any server-initiated
// notifications that overtake it — showMessage and publishDiagnostics routinely
// arrive before the reply to a request.
func (s *session) expect(t *testing.T, method string) *message {
	t.Helper()
	type res struct {
		m   *message
		err error
	}
	ch := make(chan res, 1)
	go func() {
		for {
			m, err := s.out.c.read()
			if err != nil {
				ch <- res{nil, err}
				return
			}
			if m.Method == method {
				ch <- res{m, nil}
				return
			}
		}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("reading %q: %v", method, r.err)
		}
		return r.m
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %q", method)
		return nil
	}
}

// tryExpect waits briefly for any message and reports whether one arrived. It
// never fails the test, so it is safe to use for "nothing should be sent"
// assertions where a fatal in a goroutine would outlive the test.
func (s *session) tryExpect(wait time.Duration) (*message, bool) {
	ch := make(chan *message, 1)
	go func() {
		m, err := s.out.c.read()
		if err != nil {
			close(ch)
			return
		}
		ch <- m
	}()
	select {
	case m, ok := <-ch:
		return m, ok
	case <-time.After(wait):
		return nil, false
	}
}

func result(issues ...validator.Issue) *validator.Result {
	return &validator.Result{Filename: "patient.json", Issues: issues}
}

func TestInitializeAdvertisesCapabilities(t *testing.T) {
	s := newSession(t, &fakeBackend{result: result()}, &fakeSuppressor{})
	s.send(t, "initialize", 1, map[string]interface{}{"rootUri": "file:///tmp"})

	m := s.expect(t, "")
	var got initializeResult
	raw, _ := json.Marshal(m.Result)
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Capabilities.TextDocumentSync != textDocumentSyncFull {
		t.Errorf("expected full sync, got %d", got.Capabilities.TextDocumentSync)
	}
	if !got.Capabilities.HoverProvider {
		t.Error("hover should be advertised")
	}
	if !got.Capabilities.CodeActionProvider {
		t.Error("code actions should be advertised when a suppressor is configured")
	}
	if got.ServerInfo.Name != "fhirlint" {
		t.Errorf("unexpected server name %q", got.ServerInfo.Name)
	}
}

func TestDidOpenPublishesDiagnostics(t *testing.T) {
	backend := &fakeBackend{result: result(validator.Issue{
		Severity:  "error",
		Message:   "Patient.gender: value is not a valid code",
		Location:  "Patient.gender (line 2, col 5)",
		MessageID: "Type_Specific_Checks_DT_Code_Bad",
	})}
	s := newSession(t, backend, nil)

	s.send(t, "textDocument/didOpen", nil, didOpenParams{TextDocument: textDocumentItem{
		URI:  "file:///tmp/patient.json",
		Text: "{\n  \"gender\": \"nope\"\n}\n",
	}})

	m := s.expect(t, "textDocument/publishDiagnostics")
	var p publishDiagnosticsParams
	if err := json.Unmarshal(m.Params, &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Diagnostics) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(p.Diagnostics))
	}
	d := p.Diagnostics[0]
	if d.Severity != severityError {
		t.Errorf("severity = %d, want %d", d.Severity, severityError)
	}
	if d.Code != "Type_Specific_Checks_DT_Code_Bad" {
		t.Errorf("code = %q, want the HL7 message id", d.Code)
	}
	if d.Source != "fhirlint" {
		t.Errorf("source = %q", d.Source)
	}
	// line 2, col 5 is zero-based line 1, character 4.
	if d.Range.Start.Line != 1 || d.Range.Start.Character != 4 {
		t.Errorf("range start = %+v, want line 1 char 4", d.Range.Start)
	}
}

func TestDidChangeIsDebouncedToOneRun(t *testing.T) {
	backend := &fakeBackend{result: result()}
	s := newSession(t, backend, nil)

	s.send(t, "textDocument/didOpen", nil, didOpenParams{TextDocument: textDocumentItem{
		URI: "file:///tmp/patient.json", Text: "{}",
	}})
	s.expect(t, "textDocument/publishDiagnostics")
	afterOpen := backend.callCount()

	// Five rapid edits must collapse into a single validation.
	for i := range 5 {
		s.send(t, "textDocument/didChange", nil, didChangeParams{
			TextDocument:   versionedTextDocumentIdentifier{URI: "file:///tmp/patient.json", Version: i + 2},
			ContentChanges: []contentChange{{Text: fmt.Sprintf(`{"id":"%d"}`, i)}},
		})
	}
	s.expect(t, "textDocument/publishDiagnostics")

	if got := backend.callCount() - afterOpen; got != 1 {
		t.Errorf("expected 1 validation for 5 rapid edits, got %d", got)
	}
}

func TestBackendFailureKeepsPreviousDiagnostics(t *testing.T) {
	backend := &fakeBackend{result: result(validator.Issue{
		Severity: "error", Message: "boom", Location: "Patient (line 1, col 1)", MessageID: "x",
	})}
	s := newSession(t, backend, nil)

	s.send(t, "textDocument/didOpen", nil, didOpenParams{TextDocument: textDocumentItem{
		URI: "file:///tmp/patient.json", Text: "{}",
	}})
	m := s.expect(t, "textDocument/publishDiagnostics")
	var first publishDiagnosticsParams
	_ = json.Unmarshal(m.Params, &first)
	if len(first.Diagnostics) != 1 {
		t.Fatalf("setup: expected 1 diagnostic, got %d", len(first.Diagnostics))
	}

	// The backend now fails. Nothing new should be published — clearing the
	// list would look like the file just became clean.
	backend.mu.Lock()
	backend.err = fmt.Errorf("validator server is down")
	backend.mu.Unlock()

	s.send(t, "textDocument/didSave", nil, didSaveParams{
		TextDocument: textDocumentIdentifier{URI: "file:///tmp/patient.json"},
	})

	if m, ok := s.tryExpect(700 * time.Millisecond); ok {
		var p publishDiagnosticsParams
		_ = json.Unmarshal(m.Params, &p)
		t.Errorf("nothing should have been published, got %d diagnostics", len(p.Diagnostics))
	}
}

func TestDidCloseClearsDiagnostics(t *testing.T) {
	s := newSession(t, &fakeBackend{result: result()}, nil)

	s.send(t, "textDocument/didOpen", nil, didOpenParams{TextDocument: textDocumentItem{
		URI: "file:///tmp/patient.json", Text: "{}",
	}})
	s.expect(t, "textDocument/publishDiagnostics")

	s.send(t, "textDocument/didClose", nil, didCloseParams{
		TextDocument: textDocumentIdentifier{URI: "file:///tmp/patient.json"},
	})
	m := s.expect(t, "textDocument/publishDiagnostics")
	var p publishDiagnosticsParams
	_ = json.Unmarshal(m.Params, &p)
	if len(p.Diagnostics) != 0 {
		t.Errorf("closing a file must clear its problems, got %d", len(p.Diagnostics))
	}
}

func TestHoverExplainsAKnownMessageID(t *testing.T) {
	// dom-6 is in the built-in explain corpus.
	backend := &fakeBackend{result: result(validator.Issue{
		Severity:  "warning",
		Message:   "Constraint failed: dom-6",
		Location:  "Patient (line 1, col 1)",
		MessageID: "dom-6",
	})}
	s := newSession(t, backend, nil)

	s.send(t, "textDocument/didOpen", nil, didOpenParams{TextDocument: textDocumentItem{
		URI: "file:///tmp/patient.json", Text: `{"resourceType":"Patient"}`,
	}})
	s.expect(t, "textDocument/publishDiagnostics")

	s.send(t, "textDocument/hover", 7, hoverParams{
		TextDocument: textDocumentIdentifier{URI: "file:///tmp/patient.json"},
		Position:     position{Line: 0, Character: 2},
	})
	m := s.expect(t, "")
	raw, _ := json.Marshal(m.Result)
	var h hover
	if err := json.Unmarshal(raw, &h); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.Contents.Value, "dom-6") {
		t.Errorf("hover should name the message id, got: %s", h.Contents.Value)
	}
	if !strings.Contains(strings.ToLower(h.Contents.Value), "narrative") {
		t.Errorf("hover should carry the offline explanation, got: %s", h.Contents.Value)
	}
}

func TestHoverOutsideAnyDiagnosticReturnsNull(t *testing.T) {
	s := newSession(t, &fakeBackend{result: result()}, nil)
	s.send(t, "textDocument/didOpen", nil, didOpenParams{TextDocument: textDocumentItem{
		URI: "file:///tmp/patient.json", Text: "{}\n",
	}})
	s.expect(t, "textDocument/publishDiagnostics")

	s.send(t, "textDocument/hover", 9, hoverParams{
		TextDocument: textDocumentIdentifier{URI: "file:///tmp/patient.json"},
		Position:     position{Line: 0, Character: 1},
	})
	m := s.expect(t, "")
	if m.Result != nil {
		t.Errorf("expected a null hover, got %v", m.Result)
	}
}

func TestCodeActionOffersSuppressionPerMessageID(t *testing.T) {
	s := newSession(t, &fakeBackend{result: result()}, &fakeSuppressor{})

	diag := diagnostic{Code: "dom-6", Message: "Constraint failed"}
	s.send(t, "textDocument/codeAction", 3, codeActionParams{
		TextDocument: textDocumentIdentifier{URI: "file:///tmp/patient.json"},
		// The same id twice must yield one action, not two.
		Context: codeActionContext{Diagnostics: []diagnostic{diag, diag}},
	})

	m := s.expect(t, "")
	raw, _ := json.Marshal(m.Result)
	var actions []codeAction
	if err := json.Unmarshal(raw, &actions); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if !strings.Contains(actions[0].Title, "dom-6") {
		t.Errorf("action title should name the id, got %q", actions[0].Title)
	}
	if actions[0].Command == nil || actions[0].Command.Command != commandSuppress {
		t.Errorf("action should invoke %s", commandSuppress)
	}
}

func TestExecuteCommandWritesSuppression(t *testing.T) {
	sup := &fakeSuppressor{}
	s := newSession(t, &fakeBackend{result: result()}, sup)

	s.send(t, "workspace/executeCommand", 4, map[string]interface{}{
		"command":   commandSuppress,
		"arguments": []interface{}{"dom-6"},
	})
	s.expect(t, "")

	sup.mu.Lock()
	defer sup.mu.Unlock()
	if len(sup.ids) != 1 || sup.ids[0] != "dom-6" {
		t.Errorf("expected dom-6 to be suppressed, got %v", sup.ids)
	}
}

func TestUnknownRequestGetsMethodNotFound(t *testing.T) {
	s := newSession(t, &fakeBackend{result: result()}, nil)
	s.send(t, "textDocument/somethingElse", 11, map[string]interface{}{})

	m := s.expect(t, "")
	if m.Error == nil || m.Error.Code != codeMethodNotFound {
		t.Errorf("expected method-not-found, got %+v", m.Error)
	}
}

func TestNonFileURIIsSkippedNotCrashed(t *testing.T) {
	backend := &fakeBackend{result: result()}
	s := newSession(t, backend, nil)

	// An untitled buffer has no path; the server must ignore it rather than die.
	s.send(t, "textDocument/didOpen", nil, didOpenParams{TextDocument: textDocumentItem{
		URI: "untitled:Untitled-1", Text: "{}",
	}})
	// Follow with a request that does produce a reply, to prove we are alive.
	s.send(t, "textDocument/somethingElse", 12, map[string]interface{}{})
	m := s.expect(t, "")
	if m.Error == nil {
		t.Fatal("server stopped responding after an unsupported URI scheme")
	}
	if backend.callCount() != 0 {
		t.Errorf("a non-file document should not be validated, got %d calls", backend.callCount())
	}
}

func TestFramingRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	c := newConn(strings.NewReader(""), &buf)
	if err := c.notify("test/method", map[string]string{"a": "b"}); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	if !strings.HasPrefix(got, "Content-Length: ") {
		t.Errorf("missing Content-Length header: %q", got)
	}
	rc := newConn(strings.NewReader(got), io.Discard)
	m, err := rc.read()
	if err != nil {
		t.Fatal(err)
	}
	if m.Method != "test/method" || m.JSONRPC != "2.0" {
		t.Errorf("round trip lost data: %+v", m)
	}
}

func TestReadRejectsMissingContentLength(t *testing.T) {
	c := newConn(strings.NewReader("X-Other: 1\r\n\r\n{}"), io.Discard)
	if _, err := c.read(); err == nil {
		t.Error("a message without Content-Length must be rejected")
	}
}

func TestHoverExplainsCanonicalQualifiedMessageID(t *testing.T) {
	// The validator reports some ids as a canonical URL with a fragment. The
	// explanation corpus is keyed by the fragment, so the hover has to strip it
	// or the most common constraint findings come back unexplained.
	backend := &fakeBackend{result: result(validator.Issue{
		Severity:  "warning",
		Message:   "Constraint failed: dom-6",
		Location:  "Patient (line 1, col 1)",
		MessageID: "http://hl7.org/fhir/StructureDefinition/DomainResource#dom-6",
	})}
	s := newSession(t, backend, nil)

	s.send(t, "textDocument/didOpen", nil, didOpenParams{TextDocument: textDocumentItem{
		URI: "file:///tmp/patient.json", Text: `{"resourceType":"Patient"}`,
	}})
	s.expect(t, "textDocument/publishDiagnostics")

	s.send(t, "textDocument/hover", 21, hoverParams{
		TextDocument: textDocumentIdentifier{URI: "file:///tmp/patient.json"},
		Position:     position{Line: 0, Character: 2},
	})
	m := s.expect(t, "")
	raw, _ := json.Marshal(m.Result)
	var h hover
	if err := json.Unmarshal(raw, &h); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(h.Contents.Value), "narrative") {
		t.Errorf("canonical-qualified id should still resolve to an explanation, got: %s", h.Contents.Value)
	}
}

func TestCodeActionSuppressesByShortMessageID(t *testing.T) {
	s := newSession(t, &fakeBackend{result: result()}, &fakeSuppressor{})

	s.send(t, "textDocument/codeAction", 22, codeActionParams{
		TextDocument: textDocumentIdentifier{URI: "file:///tmp/patient.json"},
		Context: codeActionContext{Diagnostics: []diagnostic{
			{Code: "http://hl7.org/fhir/StructureDefinition/DomainResource#dom-6"},
		}},
	})
	m := s.expect(t, "")
	raw, _ := json.Marshal(m.Result)
	var actions []codeAction
	if err := json.Unmarshal(raw, &actions); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	// "Suppress dom-6", not the whole canonical URL.
	if !strings.Contains(actions[0].Title, "dom-6") || strings.Contains(actions[0].Title, "http://") {
		t.Errorf("action should use the short id, got %q", actions[0].Title)
	}
	if got := actions[0].Command.Arguments[0]; got != "dom-6" {
		t.Errorf("command argument = %v, want dom-6", got)
	}
}

func TestShortMessageID(t *testing.T) {
	tests := map[string]string{
		"dom-6": "dom-6",
		"http://hl7.org/fhir/StructureDefinition/DomainResource#dom-6": "dom-6",
		"Terminology_TX_NoValid_16":                                    "Terminology_TX_NoValid_16",
		"":                                                             "",
	}
	for in, want := range tests {
		if got := shortMessageID(in); got != want {
			t.Errorf("shortMessageID(%q) = %q, want %q", in, got, want)
		}
	}
}
