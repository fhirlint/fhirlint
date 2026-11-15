// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
)

// The error paths of the language server (#633).

func framed(msgs ...string) string {
	var b strings.Builder
	for _, m := range msgs {
		fmt.Fprintf(&b, "Content-Length: %d\r\n\r\n%s", len(m), m)
	}
	return b.String()
}

// A message that isn't JSON, or has no length, is logged and skipped; the
// session goes on, and exit ends it cleanly.
func TestRun_ProtocolErrorsAreLoggedAndServingContinues(t *testing.T) {
	in := framed(`{not json`) + "X-Other: 1\r\n\r\n" + framed(`{"jsonrpc":"2.0","id":7,"method":"shutdown"}`, `{"jsonrpc":"2.0","method":"exit"}`, `{"jsonrpc":"2.0","id":8,"method":"shutdown"}`)
	var out, log bytes.Buffer
	srv := NewServer(strings.NewReader(in), &out, &fakeBackend{result: result()}, nil, &log, "test")
	if err := srv.Run(); err != nil {
		t.Fatalf("exit ends the session without an error: %v", err)
	}
	if strings.Count(log.String(), "protocol error") != 2 {
		t.Errorf("both bad messages must be logged:\n%s", log.String())
	}
	if !strings.Contains(out.String(), `"id":7`) || strings.Contains(out.String(), `"id":8`) {
		t.Errorf("the request before exit is answered, nothing after it:\n%s", out.String())
	}
}

// A body shorter than its Content-Length breaks the framing: no message after
// it can be trusted, so the session ends with the error.
func TestRun_TruncatedBodyEndsTheSession(t *testing.T) {
	srv := NewServer(strings.NewReader("Content-Length: 50\r\n\r\n{}"), &bytes.Buffer{}, &fakeBackend{}, nil, nil, "test")
	if err := srv.Run(); err == nil || !strings.Contains(err.Error(), "reading message body") {
		t.Errorf("got %v", err)
	}
}

// Params of the wrong shape fail that one handler, logged, not the session.
// A nil log writer is fine too.
func TestRun_BadParamsAreLoggedPerMethod(t *testing.T) {
	methods := []string{"textDocument/didOpen", "textDocument/didChange", "textDocument/didSave", "textDocument/didClose", "textDocument/hover", "textDocument/codeAction", "workspace/executeCommand"}
	var msgs []string
	for i, m := range methods {
		msgs = append(msgs, fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q,"params":"not an object"}`, i+1, m))
	}
	msgs = append(msgs, `{"jsonrpc":"2.0","method":"exit"}`)
	var log bytes.Buffer
	srv := NewServer(strings.NewReader(framed(msgs...)), &bytes.Buffer{}, &fakeBackend{result: result()}, &fakeSuppressor{}, &log, "test")
	if err := srv.Run(); err != nil {
		t.Fatal(err)
	}
	for _, m := range methods {
		if !strings.Contains(log.String(), "handling "+m+":") {
			t.Errorf("%s: bad params not logged:\n%s", m, log.String())
		}
	}
	if err := NewServer(strings.NewReader(framed(`{bad`)), &bytes.Buffer{}, &fakeBackend{}, nil, nil, "test").Run(); err != nil {
		t.Errorf("without a log writer: %v", err)
	}
}

func TestRequestsAfterShutdownAreRefused(t *testing.T) {
	s := newSession(t, &fakeBackend{result: result()}, nil)
	s.send(t, "shutdown", 1, nil)
	s.expect(t, "")
	s.send(t, "textDocument/hover", 2, hoverParams{TextDocument: textDocumentIdentifier{URI: docURI}})
	m := s.expect(t, "")
	if m.Error == nil || !strings.Contains(m.Error.Message, "shutting down") {
		t.Errorf("a request after shutdown must be refused: %+v", m)
	}
}

// didChange for a document never opened still validates it, and one without
// changes does nothing.
func TestDidChangeEdges(t *testing.T) {
	backend := &fakeBackend{result: result()}
	s := newSession(t, backend, nil)
	s.send(t, "textDocument/didChange", nil, didChangeParams{TextDocument: versionedTextDocumentIdentifier{URI: docURI, Version: 2}})
	// Not tryExpect: its reader would stay behind and take the next message.
	time.Sleep(debounceDelay + 200*time.Millisecond)
	if backend.callCount() != 0 {
		t.Error("a change without content must not validate")
	}
	s.send(t, "textDocument/didChange", nil, didChangeParams{TextDocument: versionedTextDocumentIdentifier{URI: docURI, Version: 3}, ContentChanges: []contentChange{{Text: `{"v":3}`}}})
	if m := s.expect(t, "textDocument/publishDiagnostics"); m == nil || backend.callCount() != 1 {
		t.Errorf("a change to an unopened document validates it: %d calls", backend.callCount())
	}
}

func TestCodeActionWithoutSuppressorOffersNothing(t *testing.T) {
	s := newSession(t, &fakeBackend{result: result()}, nil)
	openDoc(t, s, `{}`)
	s.send(t, "textDocument/codeAction", 3, codeActionParams{TextDocument: textDocumentIdentifier{URI: docURI}})
	m := s.expect(t, "")
	if list, ok := m.Result.([]interface{}); !ok || len(list) != 0 {
		t.Errorf("want an empty list, got %#v", m.Result)
	}
}

func TestURIToPath(t *testing.T) {
	if _, err := uriToPath("file://%zz"); err == nil || !strings.Contains(err.Error(), "invalid document URI") {
		t.Errorf("a malformed URI: %v", err)
	}
	if _, err := uriToPath("untitled:Untitled-1"); err == nil || !strings.Contains(err.Error(), "unsupported document scheme") {
		t.Errorf("an untitled buffer: %v", err)
	}
}
