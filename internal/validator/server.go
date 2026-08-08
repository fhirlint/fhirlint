package validator

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// serverReadyMarker is the log line the validator prints once its HTTP service
// is accepting requests.
const serverReadyMarker = "FHIR Validator HTTP Service started"

// ServerConfig configures a persistent validator HTTP server. The FHIR version,
// IGs, profiles and terminology settings are fixed for the lifetime of the
// server — per-request only the resource (and an optional profile) vary.
type ServerConfig struct {
	Port                int
	FHIRVersion         string
	IGs                 []string
	Profiles            []string // pre-loaded at startup so requests can select them
	NoTerminologyServer bool
	TerminologyServer   string
	TxCache             string
	FHIRSettings        string // path to a fhir-settings.json for the JAR (-fhir-settings)
	JARPath             string
	Proxy               ProxyConfig
	ValidatorVersion    string
}

// Server is a handle to a running validator HTTP server process.
type Server struct {
	cmd  *exec.Cmd
	port int
	done chan error
}

// serverArgs builds the `server <port> …` argument list.
func serverArgs(jarPath string, cfg ServerConfig) []string {
	args := []string{"-jar", jarPath, "server", strconv.Itoa(cfg.Port), "-version", cfg.FHIRVersion}
	switch {
	case cfg.NoTerminologyServer:
		args = append(args, "-tx", "n/a")
	case cfg.TerminologyServer != "":
		args = append(args, "-tx", cfg.TerminologyServer)
	}
	if cfg.TxCache != "" {
		args = append(args, "-txCache", cfg.TxCache)
	}
	if cfg.FHIRSettings != "" {
		args = append(args, "-fhir-settings", cfg.FHIRSettings)
	}
	for _, ig := range cfg.IGs {
		args = append(args, "-ig", ig)
	}
	for _, p := range cfg.Profiles {
		args = append(args, "-profile", p)
	}
	args = append(args, proxyArgs(cfg.Proxy)...)
	return args
}

// StartServer launches the validator HTTP server and blocks until it is ready to
// accept requests or startup fails. The server's log output (package loading
// progress, the ready banner) is streamed to logW. readyTimeout bounds how long
// to wait for the ready banner.
func StartServer(cfg ServerConfig, logW io.Writer, readyTimeout time.Duration) (*Server, error) {
	if cfg.Port <= 0 {
		return nil, fmt.Errorf("server port must be positive, got %d", cfg.Port)
	}
	if err := validateFHIRVersion(cfg.FHIRVersion); err != nil {
		return nil, err
	}
	jarPath, err := EnsureJAR(cfg.JARPath, cfg.ValidatorVersion)
	if err != nil {
		return nil, err
	}

	rw := &readyWriter{w: logW, marker: serverReadyMarker, ready: make(chan struct{})}
	cmd := exec.Command("java", serverArgs(jarPath, cfg)...) //nolint:gosec // runs java with validated config
	cmd.Stdout = rw
	cmd.Stderr = rw
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting validator server: %w", err)
	}

	s := &Server{cmd: cmd, port: cfg.Port, done: make(chan error, 1)}
	go func() { s.done <- cmd.Wait() }()

	select {
	case <-rw.ready:
		return s, nil
	case err := <-s.done:
		// The process exited before signalling readiness.
		if err != nil {
			return nil, fmt.Errorf("validator server exited during startup: %w", err)
		}
		return nil, fmt.Errorf("validator server exited during startup before becoming ready")
	case <-time.After(readyTimeout):
		_ = s.Stop()
		return nil, fmt.Errorf("validator server did not become ready within %s", formatDuration(readyTimeout))
	}
}

// URL returns the base URL of the server.
func (s *Server) URL() string { return fmt.Sprintf("http://localhost:%d", s.port) }

// Wait blocks until the server process exits, returning its exit error.
func (s *Server) Wait() error { return <-s.done }

// Stop asks the server to shut down (graceful /stop, then process signal) and
// waits for the process to exit.
func (s *Server) Stop() error {
	// Best-effort graceful stop.
	client := &http.Client{Timeout: 3 * time.Second}
	if req, err := http.NewRequest(http.MethodPost, s.URL()+"/stop", nil); err == nil {
		if resp, err := client.Do(req); err == nil {
			_ = resp.Body.Close()
		}
	}
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	select {
	case err := <-s.done:
		return err
	case <-time.After(5 * time.Second):
		return fmt.Errorf("validator server did not stop within 5s")
	}
}

// readyWriter tees the child process output to w and closes ready the first time
// the marker is seen. A small rolling buffer catches the marker even when it is
// split across writes.
type readyWriter struct {
	w      io.Writer
	marker string
	ready  chan struct{}
	once   sync.Once
	mu     sync.Mutex
	buf    []byte
}

func (rw *readyWriter) Write(p []byte) (int, error) {
	n, err := rw.w.Write(p)
	rw.mu.Lock()
	rw.buf = append(rw.buf, p...)
	if len(rw.buf) > 8192 {
		rw.buf = rw.buf[len(rw.buf)-8192:]
	}
	found := bytes.Contains(rw.buf, []byte(rw.marker))
	rw.mu.Unlock()
	if found {
		rw.once.Do(func() { close(rw.ready) })
	}
	return n, err
}

// RunMultipleViaServer validates each path by posting it to a running validator
// server at serverURL, returning results in input order. It mirrors
// RunMultiple's signature so callers can dispatch between the JVM and the server
// backend transparently. Only per-request options (profiles) are applied; the
// FHIR version, IGs and terminology settings come from the server's startup
// configuration.
func RunMultipleViaServer(serverURL string, paths []string, opts Options) ([]*Result, error) {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	client := &http.Client{Timeout: timeout}

	results := make([]*Result, 0, len(paths))
	for _, p := range paths {
		content, err := os.ReadFile(p) //nolint:gosec // path from resolved input
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", p, err)
		}
		res, err := validateBytesViaServer(client, serverURL, content, p, opts.Profiles)
		if err != nil {
			return nil, err
		}
		results = append(results, res)
	}
	return results, nil
}

// ValidateBytesViaServer validates in-memory content against a running
// validator server. It exists for callers that hold a document rather than a
// file — the language server validates unsaved editor buffers, which never
// reach disk.
func ValidateBytesViaServer(serverURL string, content []byte, label string, opts Options) (*Result, error) {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	return validateBytesViaServer(&http.Client{Timeout: timeout}, serverURL, content, label, opts.Profiles)
}

// validateBytesViaServer posts one resource to the server's /validateResource
// endpoint and parses the returned OperationOutcome into a Result.
func validateBytesViaServer(client *http.Client, serverURL string, resource []byte, label string, profiles []string) (*Result, error) {
	u, err := url.Parse(strings.TrimRight(serverURL, "/") + "/validateResource")
	if err != nil {
		return nil, fmt.Errorf("invalid server URL %q: %w", serverURL, err)
	}
	if len(profiles) > 0 {
		q := u.Query()
		for _, p := range profiles {
			q.Add("profile", p)
		}
		u.RawQuery = q.Encode()
	}

	contentType := "application/fhir+json"
	if looksLikeXML(resource) {
		contentType = "application/fhir+xml"
	}

	req, err := http.NewRequest(http.MethodPost, u.String(), bytes.NewReader(resource))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/fhir+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("posting %s to validator server %s: %w", label, serverURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading validator server response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("validator server returned HTTP %d for %s: %s", resp.StatusCode, label, strings.TrimSpace(string(body)))
	}

	parsed, err := parseOutput(body, []string{label}, "")
	if err != nil {
		return nil, err
	}
	if len(parsed) == 0 {
		return nil, fmt.Errorf("validator server produced no result for %s", label)
	}
	return parsed[0], nil
}

// looksLikeXML reports whether content's first non-whitespace byte is '<'.
func looksLikeXML(content []byte) bool {
	for _, b := range content {
		switch b {
		case ' ', '\t', '\r', '\n':
			continue
		case '<':
			return true
		default:
			return false
		}
	}
	return false
}
