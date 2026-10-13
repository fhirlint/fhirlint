// Package lsp implements a Language Server Protocol server for FHIR resources,
// so validation findings appear in an editor while authoring instead of only
// when the CLI is run.
//
// The protocol layer is hand-rolled rather than pulled from a library: LSP over
// stdio is Content-Length-framed JSON-RPC 2.0 and the subset a linter needs is
// small, which is cheaper to own than another dependency in the SBOM.
package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// JSON-RPC error codes used by this server.
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInternalError  = -32603
)

// maxMessageBytes caps the Content-Length the server will buffer. The body is
// allocated in one piece, so without a cap a single header could ask for more
// memory than the machine has, and running out of memory cannot be recovered.
// It matches the 64 MiB the CLI reads from an input file.
const maxMessageBytes = 64 << 20

// message is a JSON-RPC 2.0 request, notification or response. A notification
// is a request without an id; a response carries result or error.
type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return e.Message }

// conn is a framed JSON-RPC connection over a reader/writer pair.
type conn struct {
	r *bufio.Reader

	mu sync.Mutex // serialises writes; notifications can race with responses
	w  io.Writer
}

func newConn(r io.Reader, w io.Writer) *conn {
	return &conn{r: bufio.NewReader(r), w: w}
}

// read returns the next message. It returns io.EOF when the client disconnects.
func (c *conn) read() (*message, error) {
	length, err := c.readHeader()
	if err != nil {
		return nil, err
	}
	if length > maxMessageBytes {
		// Skip the body rather than buffer it, so the stream stays framed and the
		// next message still parses.
		if _, err := io.CopyN(io.Discard, c.r, int64(length)); err != nil {
			return nil, fmt.Errorf("skipping oversized message body: %w", err)
		}
		return nil, &rpcError{Code: codeInvalidRequest,
			Message: fmt.Sprintf("message of %d bytes exceeds the %d byte limit", length, maxMessageBytes)}
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(c.r, body); err != nil {
		return nil, fmt.Errorf("reading message body: %w", err)
	}
	var m message
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, &rpcError{Code: codeParseError, Message: err.Error()}
	}
	return &m, nil
}

// readHeader consumes the header block and returns the Content-Length.
func (c *conn) readHeader() (int, error) {
	length := -1
	for {
		line, err := c.r.ReadString('\n')
		if err != nil {
			return 0, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			// Blank line ends the header block.
			if length < 0 {
				return 0, &rpcError{Code: codeInvalidRequest, Message: "message has no Content-Length header"}
			}
			return length, nil
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return 0, &rpcError{Code: codeInvalidRequest, Message: "invalid Content-Length: " + value}
			}
			length = n
		}
	}
}

// write frames and sends one message.
func (c *conn) write(m *message) error {
	m.JSONRPC = "2.0"
	body, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("encoding message: %w", err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := fmt.Fprintf(c.w, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err = c.w.Write(body)
	return err
}

// reply answers a request with a result.
func (c *conn) reply(id json.RawMessage, result interface{}) error {
	return c.write(&message{ID: id, Result: result})
}

// replyError answers a request with an error.
func (c *conn) replyError(id json.RawMessage, code int, msg string) error {
	return c.write(&message{ID: id, Error: &rpcError{Code: code, Message: msg}})
}

// notify sends a server-initiated notification.
func (c *conn) notify(method string, params interface{}) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("encoding %s params: %w", method, err)
	}
	return c.write(&message{Method: method, Params: raw})
}
