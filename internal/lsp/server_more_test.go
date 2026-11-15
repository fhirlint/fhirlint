// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// More of the protocol the editor drives (#616): saving, the suppress
// command's error paths and shutdown.

const docURI = "file:///work/patient.json"

func openDoc(t *testing.T, s *session, text string) {
	t.Helper()
	s.send(t, "textDocument/didOpen", nil, didOpenParams{TextDocument: textDocumentItem{URI: docURI, Version: 1, Text: text}})
	s.expect(t, "textDocument/publishDiagnostics")
}

func TestDidSaveRevalidatesWithTheSavedText(t *testing.T) {
	backend := &fakeBackend{result: result()}
	s := newSession(t, backend, nil)
	openDoc(t, s, `{"v":1}`)

	saved := `{"v":2}`
	s.send(t, "textDocument/didSave", nil, didSaveParams{TextDocument: textDocumentIdentifier{URI: docURI}, Text: &saved})
	s.expect(t, "textDocument/publishDiagnostics")
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.calls != 2 || string(backend.lastArg) != saved {
		t.Errorf("save must validate the saved text: %d calls, last %q", backend.calls, backend.lastArg)
	}
}

func TestExecuteCommandErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		params map[string]interface{}
		sup    *fakeSuppressor
		want   string
	}{
		{"unknown command", map[string]interface{}{"command": "fhirlint.other"}, &fakeSuppressor{}, "unknown command"},
		{"no suppressor", map[string]interface{}{"command": commandSuppress, "arguments": []interface{}{"dom-6"}}, nil, "unknown command"},
		{"no argument", map[string]interface{}{"command": commandSuppress}, &fakeSuppressor{}, "needs a message id"},
		{"not a string", map[string]interface{}{"command": commandSuppress, "arguments": []interface{}{42}}, &fakeSuppressor{}, "must be a message id"},
		{"write fails", map[string]interface{}{"command": commandSuppress, "arguments": []interface{}{"dom-6"}}, &fakeSuppressor{err: errors.New("fhirlint.yml is read-only")}, "read-only"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var sup Suppressor
			if tc.sup != nil {
				sup = tc.sup
			}
			s := newSession(t, &fakeBackend{result: result()}, sup)
			s.send(t, "workspace/executeCommand", 7, tc.params)
			m := s.expect(t, "")
			if m.Error == nil || !strings.Contains(m.Error.Message, tc.want) {
				t.Errorf("want an error with %q, got %+v", tc.want, m.Error)
			}
		})
	}
}

func TestSuppressRevalidatesOpenDocuments(t *testing.T) {
	backend := &fakeBackend{result: result()}
	sup := &fakeSuppressor{}
	s := newSession(t, backend, sup)
	openDoc(t, s, `{}`)

	s.send(t, "workspace/executeCommand", 8, map[string]interface{}{"command": commandSuppress, "arguments": []interface{}{"dom-6"}})
	s.expect(t, "")
	if n := backend.callCount(); n != 2 {
		t.Errorf("a new suppression must re-validate the open documents: %d calls", n)
	}
}

func TestShutdownCancelsPendingValidation(t *testing.T) {
	backend := &fakeBackend{result: result()}
	s := newSession(t, backend, nil)
	openDoc(t, s, `{}`)

	// A change starts the debounce timer; shutdown must stop it.
	s.send(t, "textDocument/didChange", nil, map[string]interface{}{
		"textDocument":   map[string]interface{}{"uri": docURI, "version": 2},
		"contentChanges": []interface{}{map[string]interface{}{"text": `{"x":1}`}},
	})
	s.send(t, "shutdown", 9, nil)
	s.expect(t, "")
	time.Sleep(debounceDelay + 200*time.Millisecond)
	if n := backend.callCount(); n != 1 {
		t.Errorf("after shutdown the pending change must not be validated: %d calls", n)
	}
}

func TestMessageIDOf(t *testing.T) {
	for _, tc := range []struct {
		d    diagnostic
		want string
	}{
		{diagnostic{Code: "dom-6"}, "dom-6"},
		{diagnostic{Data: diagnosticData{MessageID: "ele-1"}}, "ele-1"},
		// As the client sends it back: Data round-tripped through JSON.
		{diagnostic{Data: map[string]interface{}{"messageId": "sdf-0"}}, "sdf-0"},
		{diagnostic{Data: map[string]interface{}{"other": 1}}, ""},
		{diagnostic{}, ""},
	} {
		if got := messageIDOf(tc.d); got != tc.want {
			t.Errorf("messageIDOf(%+v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}
