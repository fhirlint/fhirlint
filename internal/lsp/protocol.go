package lsp

import "encoding/json"

// Diagnostic severities, per the LSP specification.
const (
	severityError       = 1
	severityWarning     = 2
	severityInformation = 3
	severityHint        = 4
)

// TextDocumentSyncFull tells the client to send whole documents on change.
// Incremental sync would buy nothing here: validation needs the full resource
// anyway, so there is no partial work to save.
const textDocumentSyncFull = 1

type position struct {
	Line      int `json:"line"`      // zero-based
	Character int `json:"character"` // zero-based
}

type textRange struct {
	Start position `json:"start"`
	End   position `json:"end"`
}

// diagnostic is one finding shown in the editor.
type diagnostic struct {
	Range    textRange   `json:"range"`
	Severity int         `json:"severity,omitempty"`
	Code     string      `json:"code,omitempty"`
	Source   string      `json:"source,omitempty"`
	Message  string      `json:"message"`
	Data     interface{} `json:"data,omitempty"`
}

// diagnosticData travels with a diagnostic and comes back on a code action
// request, so an action can be built without re-validating.
type diagnosticData struct {
	MessageID  string `json:"messageId,omitempty"`
	Expression string `json:"expression,omitempty"`
}

type publishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []diagnostic `json:"diagnostics"`
}

type textDocumentItem struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
	Text    string `json:"text"`
}

type versionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

type textDocumentIdentifier struct {
	URI string `json:"uri"`
}

type didOpenParams struct {
	TextDocument textDocumentItem `json:"textDocument"`
}

type contentChange struct {
	Text string `json:"text"`
}

type didChangeParams struct {
	TextDocument   versionedTextDocumentIdentifier `json:"textDocument"`
	ContentChanges []contentChange                 `json:"contentChanges"`
}

type didSaveParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Text         *string                `json:"text,omitempty"`
}

type didCloseParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

type hoverParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Position     position               `json:"position"`
}

type markupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type hover struct {
	Contents markupContent `json:"contents"`
	Range    *textRange    `json:"range,omitempty"`
}

type codeActionContext struct {
	Diagnostics []diagnostic `json:"diagnostics"`
}

type codeActionParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Range        textRange              `json:"range"`
	Context      codeActionContext      `json:"context"`
}

type command struct {
	Title     string        `json:"title"`
	Command   string        `json:"command"`
	Arguments []interface{} `json:"arguments,omitempty"`
}

type codeAction struct {
	Title       string       `json:"title"`
	Kind        string       `json:"kind,omitempty"`
	Diagnostics []diagnostic `json:"diagnostics,omitempty"`
	Command     *command     `json:"command,omitempty"`
}

type initializeParams struct {
	RootURI string `json:"rootUri"`
}

type serverCapabilities struct {
	TextDocumentSync   int         `json:"textDocumentSync"`
	HoverProvider      bool        `json:"hoverProvider"`
	CodeActionProvider bool        `json:"codeActionProvider"`
	ExecuteCommand     interface{} `json:"executeCommandProvider,omitempty"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type initializeResult struct {
	Capabilities serverCapabilities `json:"capabilities"`
	ServerInfo   serverInfo         `json:"serverInfo"`
}

type executeCommandParams struct {
	Command   string            `json:"command"`
	Arguments []json.RawMessage `json:"arguments"`
}

type showMessageParams struct {
	Type    int    `json:"type"`
	Message string `json:"message"`
}
