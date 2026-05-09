package input

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

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
func Resolve(arg string, fromURL string) (*Input, error) {
	if fromURL != "" {
		return fromHTTP(fromURL)
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
	return writeTempFile(data, "stdin")
}

func fromHTTP(rawURL string) (*Input, error) {
	resp, err := http.Get(rawURL) //nolint:gosec,noctx // intentional: user-supplied URL
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", rawURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, rawURL)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	ext := ".json"
	if strings.Contains(resp.Header.Get("Content-Type"), "xml") {
		ext = ".xml"
	}
	tmp, err := writeTempFile(data, rawURL)
	if err != nil {
		return nil, err
	}
	// Rename temp file to the correct extension for the validator.
	newPath := strings.TrimSuffix(tmp.Path, filepath.Ext(tmp.Path)) + ext
	if err := os.Rename(tmp.Path, newPath); err != nil {
		return nil, err
	}
	tmp.Path = newPath
	tmp.TempFile = newPath
	tmp.Label = rawURL
	return tmp, nil
}

func writeTempFile(data []byte, label string) (*Input, error) {
	f, err := os.CreateTemp("", "fhirlint-*.json")
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
