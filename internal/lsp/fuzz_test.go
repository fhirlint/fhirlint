package lsp

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

// FuzzRead reads messages from a fuzzed stream until it ends, the way the
// server loop does. A malformed frame or body is a protocol error the server
// survives; a panic, a hang or an allocation the header asked for but the
// stream does not back would take the editor's language server down.
func FuzzRead(f *testing.F) {
	f.Add([]byte("Content-Length: 38\r\n\r\n{\"jsonrpc\":\"2.0\",\"method\":\"test/next\"}"))
	f.Add([]byte("Content-Length: 2\r\nContent-Type: application/vscode-jsonrpc\r\n\r\n{}Content-Length: 1\r\n\r\n["))
	f.Add([]byte("Content-Length: -2\r\n\r\n"))
	f.Add([]byte("Content-Length: 99999999999999999999\r\n\r\n"))
	f.Add([]byte("Content-Length: 67108865\r\n\r\n{}"))
	f.Add([]byte("no colon\r\n\r\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		c := newConn(bytes.NewReader(data), io.Discard)
		for i := 0; i <= len(data); i++ {
			_, err := c.read()
			if err == nil {
				continue
			}
			var rpcErr *rpcError
			if errors.As(err, &rpcErr) {
				continue
			}
			return // the stream ended or broke off mid-message
		}
		t.Fatalf("still reading after %d messages from %d bytes", len(data)+1, len(data))
	})
}
