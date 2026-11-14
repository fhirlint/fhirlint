// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package input

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const defaultHTTPTimeout = 30 * time.Second

// maxURLBytes bounds a --url response. The body comes from a server fhirlint
// has no reason to trust, so it is streamed to disk and cut off here rather
// than read into memory whole (#586). The limit matches the one for package
// downloads in internal/coverage. A variable so tests can lower it.
var maxURLBytes int64 = 512 << 20

type Source int

const (
	SourceFile Source = iota
	SourceDir
	SourceStdin
	SourceURL
)

type Input struct {
	Source   Source
	Path     string // final path passed to validator (may be a temp file)
	TempFile string // set when we created a temp file that must be cleaned up
	Label    string // human-readable label for reports
}

func (i *Input) Cleanup() {
	if i.TempFile != "" {
		_ = os.Remove(i.TempFile) // best-effort cleanup; error not actionable here
	}
}

// Resolve determines the input type and prepares a file path the validator can use.
// httpTimeout applies only to URL inputs; 0 uses the default (30s).
func Resolve(arg string, fromURL string, httpTimeout time.Duration) (*Input, error) {
	if fromURL != "" {
		return fromHTTP(fromURL, httpTimeout)
	}
	if arg == "" || arg == "-" {
		return fromStdin()
	}
	info, err := os.Stat(arg)
	if err != nil {
		return nil, fmt.Errorf("cannot access %q: %w", arg, err)
	}
	if info.IsDir() {
		return &Input{Source: SourceDir, Path: arg, Label: arg}, nil
	}
	return &Input{Source: SourceFile, Path: arg, Label: arg}, nil
}

func fromStdin() (*Input, error) {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, fmt.Errorf("reading stdin: %w", err)
	}
	return writeTempFileDetectExt(data, "stdin")
}

func fromHTTP(rawURL string, timeout time.Duration) (*Input, error) {
	if timeout == 0 {
		timeout = defaultHTTPTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil) //nolint:gosec // intentional: user-supplied URL
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", rawURL, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", rawURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, rawURL)
	}
	if resp.ContentLength > maxURLBytes {
		return nil, fmt.Errorf("response from %s is %d bytes, over the limit of %d", rawURL, resp.ContentLength, maxURLBytes)
	}
	ext := "json"
	if strings.Contains(resp.Header.Get("Content-Type"), "xml") {
		ext = "xml"
	}
	f, err := os.CreateTemp("", "fhirlint-*."+ext)
	if err != nil {
		return nil, fmt.Errorf("creating temp file: %w", err)
	}
	n, copyErr := io.Copy(f, io.LimitReader(resp.Body, maxURLBytes+1))
	closeErr := f.Close()
	switch {
	case copyErr != nil:
		_ = os.Remove(f.Name())
		return nil, fmt.Errorf("reading response: %w", copyErr)
	case n > maxURLBytes:
		_ = os.Remove(f.Name())
		return nil, fmt.Errorf("response from %s is over the limit of %d bytes", rawURL, maxURLBytes)
	case closeErr != nil:
		_ = os.Remove(f.Name())
		return nil, closeErr
	}
	return &Input{
		Source:   SourceStdin,
		Path:     f.Name(),
		TempFile: f.Name(),
		Label:    rawURL,
	}, nil
}

// writeTempFileDetectExt creates a temp file with .xml or .json extension based on content.
// The HL7 validator uses the file extension to determine the input format.
func writeTempFileDetectExt(data []byte, label string) (*Input, error) {
	ext := "json"
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("<")) {
		ext = "xml"
	}
	return writeTempFile(data, label, ext)
}

func writeTempFile(data []byte, label, ext string) (*Input, error) {
	f, err := os.CreateTemp("", "fhirlint-*."+ext)
	if err != nil {
		return nil, fmt.Errorf("creating temp file: %w", err)
	}
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil {
		_ = os.Remove(f.Name())
		return nil, writeErr
	}
	if closeErr != nil {
		_ = os.Remove(f.Name())
		return nil, closeErr
	}
	return &Input{
		Source:   SourceStdin,
		Path:     f.Name(),
		TempFile: f.Name(),
		Label:    label,
	}, nil
}
