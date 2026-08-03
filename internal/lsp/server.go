package lsp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fhirlint/fhirlint/internal/explain"
	"github.com/fhirlint/fhirlint/internal/validator"
)

// debounceDelay is how long to wait after the last keystroke before validating.
// Long enough that typing does not queue a run per character, short enough that
// diagnostics still feel attached to the edit.
const debounceDelay = 400 * time.Millisecond

// commandSuppress is the client-invocable command that writes a suppression.
const commandSuppress = "fhirlint.suppress"

// Backend validates in-memory document content. It is an interface so the
// protocol layer can be tested without a JVM.
type Backend interface {
	// ValidateContent validates the given bytes. label is the file path, used
	// for reporting only.
	ValidateContent(content []byte, label string) (*validator.Result, error)
}

// Suppressor records a suppression decision, normally by writing to fhirlint.yml.
type Suppressor interface {
	Suppress(messageID string) error
}

// document is an open editor buffer and the diagnostics last published for it.
type document struct {
	text    string
	version int
	diags   []diagnostic
	timer   *time.Timer
}

// Server speaks LSP over a reader/writer pair.
type Server struct {
	conn       *conn
	backend    Backend
	suppressor Suppressor
	logW       io.Writer
	version    string

	mu   sync.Mutex
	docs map[string]*document

	shutdownRequested bool
}

// NewServer builds a server. suppressor may be nil, in which case the suppress
// code action is not offered.
func NewServer(r io.Reader, w io.Writer, backend Backend, suppressor Suppressor, logW io.Writer, version string) *Server {
	return &Server{
		conn:       newConn(r, w),
		backend:    backend,
		suppressor: suppressor,
		logW:       logW,
		version:    version,
		docs:       make(map[string]*document),
	}
}

// Run serves until the client disconnects or sends exit.
func (s *Server) Run() error {
	for {
		msg, err := s.conn.read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			var rpcErr *rpcError
			if errors.As(err, &rpcErr) {
				s.logf("protocol error: %v", err)
				continue
			}
			return err
		}
		if msg.Method == "exit" {
			return nil
		}
		if err := s.dispatch(msg); err != nil {
			s.logf("handling %s: %v", msg.Method, err)
		}
	}
}

func (s *Server) dispatch(m *message) error {
	isRequest := len(m.ID) > 0
	// After shutdown the spec requires requests to be refused; only exit may
	// follow, and Run handles that before we get here.
	if s.shutdownRequested && isRequest && m.Method != "shutdown" {
		return s.conn.replyError(m.ID, codeInvalidRequest, "server is shutting down")
	}
	switch m.Method {
	case "initialize":
		return s.handleInitialize(m)
	case "initialized":
		return nil
	case "shutdown":
		s.shutdownRequested = true
		s.cancelPending()
		return s.conn.reply(m.ID, nil)
	case "textDocument/didOpen":
		return s.handleDidOpen(m)
	case "textDocument/didChange":
		return s.handleDidChange(m)
	case "textDocument/didSave":
		return s.handleDidSave(m)
	case "textDocument/didClose":
		return s.handleDidClose(m)
	case "textDocument/hover":
		return s.handleHover(m)
	case "textDocument/codeAction":
		return s.handleCodeAction(m)
	case "workspace/executeCommand":
		return s.handleExecuteCommand(m)
	default:
		if isRequest {
			return s.conn.replyError(m.ID, codeMethodNotFound, "unsupported method: "+m.Method)
		}
		// Unknown notifications are ignored by design: clients send plenty the
		// server never declared support for.
		return nil
	}
}

func (s *Server) handleInitialize(m *message) error {
	var p initializeParams
	_ = json.Unmarshal(m.Params, &p) // all fields optional for our purposes
	return s.conn.reply(m.ID, initializeResult{
		Capabilities: serverCapabilities{
			TextDocumentSync:   textDocumentSyncFull,
			HoverProvider:      true,
			CodeActionProvider: s.suppressor != nil,
			ExecuteCommand: map[string]interface{}{
				"commands": []string{commandSuppress},
			},
		},
		ServerInfo: serverInfo{Name: "fhirlint", Version: s.version},
	})
}

func (s *Server) handleDidOpen(m *message) error {
	var p didOpenParams
	if err := json.Unmarshal(m.Params, &p); err != nil {
		return err
	}
	s.mu.Lock()
	s.docs[p.TextDocument.URI] = &document{text: p.TextDocument.Text, version: p.TextDocument.Version}
	s.mu.Unlock()
	s.validate(p.TextDocument.URI)
	return nil
}

func (s *Server) handleDidChange(m *message) error {
	var p didChangeParams
	if err := json.Unmarshal(m.Params, &p); err != nil {
		return err
	}
	if len(p.ContentChanges) == 0 {
		return nil
	}
	// Full sync: the last change carries the whole document.
	text := p.ContentChanges[len(p.ContentChanges)-1].Text

	s.mu.Lock()
	doc := s.docs[p.TextDocument.URI]
	if doc == nil {
		doc = &document{}
		s.docs[p.TextDocument.URI] = doc
	}
	doc.text = text
	doc.version = p.TextDocument.Version
	// Debounce: a run per keystroke would queue work faster than it completes.
	if doc.timer != nil {
		doc.timer.Stop()
	}
	uri := p.TextDocument.URI
	doc.timer = time.AfterFunc(debounceDelay, func() { s.validate(uri) })
	s.mu.Unlock()
	return nil
}

func (s *Server) handleDidSave(m *message) error {
	var p didSaveParams
	if err := json.Unmarshal(m.Params, &p); err != nil {
		return err
	}
	if p.Text != nil {
		s.mu.Lock()
		if doc := s.docs[p.TextDocument.URI]; doc != nil {
			doc.text = *p.Text
		}
		s.mu.Unlock()
	}
	s.validate(p.TextDocument.URI)
	return nil
}

func (s *Server) handleDidClose(m *message) error {
	var p didCloseParams
	if err := json.Unmarshal(m.Params, &p); err != nil {
		return err
	}
	s.mu.Lock()
	if doc := s.docs[p.TextDocument.URI]; doc != nil && doc.timer != nil {
		doc.timer.Stop()
	}
	delete(s.docs, p.TextDocument.URI)
	s.mu.Unlock()
	// Clear the editor's problem list for a file that is no longer open.
	return s.conn.notify("textDocument/publishDiagnostics",
		publishDiagnosticsParams{URI: p.TextDocument.URI, Diagnostics: []diagnostic{}})
}

func (s *Server) handleHover(m *message) error {
	var p hoverParams
	if err := json.Unmarshal(m.Params, &p); err != nil {
		return err
	}
	s.mu.Lock()
	doc := s.docs[p.TextDocument.URI]
	var diags []diagnostic
	if doc != nil {
		diags = doc.diags
	}
	s.mu.Unlock()

	d, ok := diagnosticAt(diags, p.Position)
	if !ok {
		return s.conn.reply(m.ID, nil)
	}
	text := hoverText(d)
	if text == "" {
		return s.conn.reply(m.ID, nil)
	}
	rng := d.Range
	return s.conn.reply(m.ID, hover{
		Contents: markupContent{Kind: "markdown", Value: text},
		Range:    &rng,
	})
}

// hoverText renders the offline explanation for a diagnostic's message ID.
func hoverText(d diagnostic) string {
	var b strings.Builder
	id := messageIDOf(d)
	if id != "" {
		fmt.Fprintf(&b, "**%s**\n\n", id)
	}
	b.WriteString(d.Message)

	if rule, ok := explain.Lookup(shortMessageID(id)); ok {
		if rule.Title != "" {
			fmt.Fprintf(&b, "\n\n---\n\n### %s\n", rule.Title)
		}
		if rule.Description != "" {
			fmt.Fprintf(&b, "\n%s\n", rule.Description)
		}
		if rule.HowToFix != "" {
			fmt.Fprintf(&b, "\n**How to fix**\n\n%s\n", rule.HowToFix)
		}
		if rule.DefinedIn != "" {
			fmt.Fprintf(&b, "\n_Defined in: %s_\n", rule.DefinedIn)
		}
	}
	return b.String()
}

func (s *Server) handleCodeAction(m *message) error {
	var p codeActionParams
	if err := json.Unmarshal(m.Params, &p); err != nil {
		return err
	}
	if s.suppressor == nil {
		return s.conn.reply(m.ID, []codeAction{})
	}
	actions := make([]codeAction, 0, len(p.Context.Diagnostics))
	seen := make(map[string]bool)
	for _, d := range p.Context.Diagnostics {
		// Suppress by the short id: that is what a suppression rule is written
		// as, and internal/suppress already matches it against the canonical
		// form the validator reports.
		id := shortMessageID(messageIDOf(d))
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		actions = append(actions, codeAction{
			Title:       fmt.Sprintf("Suppress %s in fhirlint.yml", id),
			Kind:        "quickfix",
			Diagnostics: []diagnostic{d},
			Command: &command{
				Title:     "Suppress " + id,
				Command:   commandSuppress,
				Arguments: []interface{}{id},
			},
		})
	}
	return s.conn.reply(m.ID, actions)
}

func (s *Server) handleExecuteCommand(m *message) error {
	var p executeCommandParams
	if err := json.Unmarshal(m.Params, &p); err != nil {
		return err
	}
	if p.Command != commandSuppress || s.suppressor == nil {
		return s.conn.replyError(m.ID, codeInvalidRequest, "unknown command: "+p.Command)
	}
	if len(p.Arguments) == 0 {
		return s.conn.replyError(m.ID, codeInvalidRequest, "suppress needs a message id")
	}
	var id string
	if err := json.Unmarshal(p.Arguments[0], &id); err != nil {
		return s.conn.replyError(m.ID, codeInvalidRequest, "suppress argument must be a message id")
	}
	if err := s.suppressor.Suppress(id); err != nil {
		_ = s.conn.notify("window/showMessage", showMessageParams{Type: 1, Message: "fhirlint: " + err.Error()})
		return s.conn.replyError(m.ID, codeInternalError, err.Error())
	}
	_ = s.conn.notify("window/showMessage", showMessageParams{
		Type: 3, Message: fmt.Sprintf("fhirlint: suppressed %s in fhirlint.yml", id),
	})
	// Re-validate every open document: the new rule may hide findings elsewhere.
	s.mu.Lock()
	uris := make([]string, 0, len(s.docs))
	for uri := range s.docs {
		uris = append(uris, uri)
	}
	s.mu.Unlock()
	for _, uri := range uris {
		s.validate(uri)
	}
	return s.conn.reply(m.ID, nil)
}

// validate runs the backend against a document's current text and publishes the
// result. It is safe to call from a timer goroutine.
func (s *Server) validate(uri string) {
	s.mu.Lock()
	doc := s.docs[uri]
	if doc == nil {
		s.mu.Unlock()
		return
	}
	text := doc.text
	s.mu.Unlock()

	path, err := uriToPath(uri)
	if err != nil {
		s.logf("skipping %s: %v", uri, err)
		return
	}

	res, err := s.backend.ValidateContent([]byte(text), path)
	if err != nil {
		s.logf("validating %s: %v", path, err)
		// Leave the previous diagnostics in place. Clearing them on a backend
		// hiccup would look like the file just became clean.
		return
	}
	diags := toDiagnostics(res, text)

	s.mu.Lock()
	// The document may have been closed, or changed again, while we validated.
	if cur := s.docs[uri]; cur == nil || cur.text != text {
		s.mu.Unlock()
		return
	}
	s.docs[uri].diags = diags
	s.mu.Unlock()

	if err := s.conn.notify("textDocument/publishDiagnostics",
		publishDiagnosticsParams{URI: uri, Diagnostics: diags}); err != nil {
		s.logf("publishing diagnostics for %s: %v", path, err)
	}
}

func (s *Server) cancelPending() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, doc := range s.docs {
		if doc.timer != nil {
			doc.timer.Stop()
		}
	}
}

func (s *Server) logf(format string, args ...interface{}) {
	if s.logW == nil {
		return
	}
	_, _ = fmt.Fprintf(s.logW, "fhirlint lsp: "+format+"\n", args...)
}

// messageIDOf recovers the message id from a diagnostic, whether it arrived
// from our own state or came back through the client (which round-trips Data
// as generic JSON).
func messageIDOf(d diagnostic) string {
	if d.Code != "" {
		return d.Code
	}
	switch data := d.Data.(type) {
	case diagnosticData:
		return data.MessageID
	case map[string]interface{}:
		if v, ok := data["messageId"].(string); ok {
			return v
		}
	}
	return ""
}

// shortMessageID strips the canonical-URL qualifier the validator puts on some
// message ids, turning
// "http://hl7.org/fhir/StructureDefinition/DomainResource#dom-6" into "dom-6".
//
// That short form is what the explanation corpus is keyed by and what a
// suppression rule is written as; internal/suppress already matches the two
// against each other.
func shortMessageID(id string) string {
	if i := strings.LastIndex(id, "#"); i >= 0 {
		return id[i+1:]
	}
	return id
}

// uriToPath converts a file:// URI to a local path. Anything else is rejected:
// validation reads real files for context and cannot serve untitled buffers or
// remote schemes.
func uriToPath(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("invalid document URI %q: %w", uri, err)
	}
	if u.Scheme != "file" {
		return "", fmt.Errorf("unsupported document scheme %q", u.Scheme)
	}
	p := u.Path
	// Windows paths arrive as /C:/dir/file.json.
	if runtime.GOOS == "windows" {
		p = strings.TrimPrefix(p, "/")
		return filepath.FromSlash(p), nil
	}
	return filepath.FromSlash(p), nil
}
