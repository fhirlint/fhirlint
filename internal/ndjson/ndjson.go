// Package ndjson splits NDJSON (Newline Delimited JSON) files produced by FHIR
// Bulk Data exports into per-resource temp files for individual validation.
package ndjson

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fhirlint/fhirlint/internal/input"
)

// IsNDJSON reports whether path has the .ndjson extension.
func IsNDJSON(path string) bool {
	return strings.ToLower(filepath.Ext(path)) == ".ndjson"
}

// Split reads an NDJSON file and writes each non-empty line to a separate temp
// file. Returns one *input.Input per resource with a label of "basename:line".
// The caller is responsible for calling Cleanup() on each returned Input.
// Empty lines are skipped. Lines larger than 10 MiB return an error.
func Split(path string) ([]*input.Input, error) {
	f, err := os.Open(path) //nolint:gosec // caller-supplied path
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	base := filepath.Base(path)
	var ins []*input.Input
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 10*1024*1024), 10*1024*1024)
	lineNum := 0

	for sc.Scan() {
		lineNum++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		tmp, err := writeTempJSON([]byte(line))
		if err != nil {
			for _, t := range ins {
				t.Cleanup()
			}
			return nil, fmt.Errorf("%s line %d: %w", path, lineNum, err)
		}
		tmp.Label = fmt.Sprintf("%s:%d", base, lineNum)
		ins = append(ins, tmp)
	}
	if err := sc.Err(); err != nil {
		for _, t := range ins {
			t.Cleanup()
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return ins, nil
}

func writeTempJSON(data []byte) (*input.Input, error) {
	f, err := os.CreateTemp("", "fhirlint-ndjson-*.json")
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
	return &input.Input{
		Source:   input.SourceFile,
		Path:     f.Name(),
		TempFile: f.Name(),
	}, nil
}
