// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/fhirlint/fhirlint/internal/txreplay"
)

// maxProxiedBody bounds a validator response the restoring proxy reads into
// memory. An OperationOutcome for one resource is far smaller.
const maxProxiedBody = 64 << 20

// startRestoringProxy listens on port and forwards to the validator server at
// target, rewriting the replay server's URL in JSON responses to the server
// the recording stands for (#684). Under serve --tx-offline the clients talk
// to the validator directly, so this is the one place fhirlint sees the
// findings; without it they would name 127.0.0.1 and a port that is new every
// start. It returns the proxy's URL and a function that stops it.
func startRestoringProxy(port int, target, base string, m *txreplay.Manifest) (string, func() error, error) {
	to, err := url.Parse(target)
	if err != nil {
		return "", nil, err
	}
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		return "", nil, fmt.Errorf("listening on port %d: %w", port, err)
	}
	proxy := httputil.NewSingleHostReverseProxy(to)
	proxy.ModifyResponse = func(resp *http.Response) error {
		if !strings.Contains(resp.Header.Get("Content-Type"), "json") {
			return nil
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxProxiedBody+1))
		_ = resp.Body.Close()
		if err != nil {
			return err
		}
		if len(body) > maxProxiedBody {
			return fmt.Errorf("validator response over %d bytes", maxProxiedBody)
		}
		restored := []byte(txreplay.RestoreURLs(string(body), base, m))
		resp.Body = io.NopCloser(bytes.NewReader(restored))
		resp.ContentLength = int64(len(restored))
		resp.Header.Set("Content-Length", strconv.Itoa(len(restored)))
		return nil
	}
	srv := &http.Server{Handler: proxy, ReadHeaderTimeout: 30 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	stop := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
	return fmt.Sprintf("http://localhost:%d", port), stop, nil
}

// freeLoopbackPort is a port nothing listens on right now, for a server
// fhirlint starts behind its own.
func freeLoopbackPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port, nil
}
