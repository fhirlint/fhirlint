// Package txreplay records the validator's terminology traffic and replays it
// from disk, so a validation run can be reproduced without reaching a
// terminology server.
//
// It exists because the validator JAR cannot do this itself. Its own -txCache
// is a latency optimisation: the JAR contacts the terminology server for its
// CapabilityStatement before validating anything and aborts hard when that
// fails, warm cache or not. Making a run genuinely hermetic therefore means
// standing in for the server, not caching behind it.
package txreplay

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultDir is where recordings live when the user does not choose a path.
const DefaultDir = ".fhirlint-tx"

// manifestName holds provenance for the directory as a whole. It is written for
// humans and for error messages; nothing keys off it.
const manifestName = "manifest.json"

// Interaction is one recorded request/response pair.
//
// Response is stored as raw JSON when the body parses as JSON — which is the
// case for every FHIR terminology response — so a recording stays readable and
// diffable in review. ResponseText is the fallback for anything else.
type Interaction struct {
	Method       string          `json:"method"`
	Path         string          `json:"path"`
	Query        string          `json:"query,omitempty"`
	Request      json.RawMessage `json:"request,omitempty"`
	Status       int             `json:"status"`
	ContentType  string          `json:"contentType,omitempty"`
	Response     json.RawMessage `json:"response,omitempty"`
	ResponseText string          `json:"responseText,omitempty"`
}

// Manifest describes a recording directory.
//
// ValidatorVersion matters more than it looks: which terminology requests get
// made is a property of the validator, not just of the resources. 6.10.0
// changed how code systems are resolved, so a recording made with one version
// can miss under another. Recording it turns an unexplained replay miss into a
// warning that names the cause.
type Manifest struct {
	Upstream         string `json:"upstream"`
	FHIRVersion      string `json:"fhirVersion,omitempty"`
	ValidatorVersion string `json:"validatorVersion,omitempty"`
	Recorded         string `json:"recorded"`
	Entries          int    `json:"entries"`
}

// Key identifies an interaction by what the validator asked for. Query
// parameters are sorted and the request body is canonicalised, so the same
// logical request keys identically across runs regardless of map ordering.
func Key(method, path, rawQuery string, body []byte) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s\n%s\n%s\n", strings.ToUpper(method), path, canonicalQuery(rawQuery))
	h.Write(canonicalJSON(body))
	return hex.EncodeToString(h.Sum(nil))[:32]
}

func canonicalQuery(raw string) string {
	if raw == "" {
		return ""
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return raw
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		vs := values[k]
		sort.Strings(vs)
		for _, v := range vs {
			if b.Len() > 0 {
				b.WriteByte('&')
			}
			b.WriteString(k)
			b.WriteByte('=')
			b.WriteString(v)
		}
	}
	return b.String()
}

// canonicalJSON re-encodes JSON so that object key order and whitespace cannot
// affect the key. Non-JSON input is returned unchanged.
func canonicalJSON(body []byte) []byte {
	if len(body) == 0 {
		return nil
	}
	var v interface{}
	if err := json.Unmarshal(body, &v); err != nil {
		return body
	}
	out, err := json.Marshal(v) // encoding/json sorts map keys
	if err != nil {
		return body
	}
	return out
}

// Store is a directory of recorded interactions.
type Store struct {
	dir     string
	entries map[string]*Interaction
}

// Open reads a recording directory. A directory that does not exist yields an
// empty store, which is what recording into a fresh path needs.
func Open(dir string) (*Store, error) {
	s := &Store{dir: dir, entries: make(map[string]*Interaction)}
	names, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading terminology recording %s: %w", dir, err)
	}
	for _, e := range names {
		name := e.Name()
		if e.IsDir() || name == manifestName || !strings.HasSuffix(name, ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // path built from the recording directory
		if err != nil {
			return nil, fmt.Errorf("reading terminology recording %s: %w", name, err)
		}
		var in Interaction
		if err := json.Unmarshal(data, &in); err != nil {
			return nil, fmt.Errorf("terminology recording %s is corrupt: %w", name, err)
		}
		s.entries[strings.TrimSuffix(name, ".json")] = &in
	}
	return s, nil
}

// Len reports how many interactions the store holds.
func (s *Store) Len() int { return len(s.entries) }

// Dir reports the directory the store was opened from.
func (s *Store) Dir() string { return s.dir }

// Lookup finds a recorded response, or reports false when nothing matches.
func (s *Store) Lookup(method, path, rawQuery string, body []byte) (*Interaction, bool) {
	in, ok := s.entries[Key(method, path, rawQuery, body)]
	return in, ok
}

// Put records an interaction, both in memory and on disk.
func (s *Store) Put(in *Interaction, body []byte) error {
	if err := os.MkdirAll(s.dir, 0o750); err != nil {
		return fmt.Errorf("creating terminology recording directory %s: %w", s.dir, err)
	}
	key := Key(in.Method, in.Path, in.Query, body)
	data, err := json.MarshalIndent(in, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding terminology interaction: %w", err)
	}
	if err := os.WriteFile(filepath.Join(s.dir, key+".json"), data, 0o600); err != nil {
		return fmt.Errorf("writing terminology recording: %w", err)
	}
	s.entries[key] = in
	return nil
}

// WriteManifest records where a recording came from. It is provenance for the
// humans reviewing a committed recording, not something replay depends on.
func (s *Store) WriteManifest(m Manifest) error {
	m.Entries = len(s.entries)
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding terminology manifest: %w", err)
	}
	if err := os.MkdirAll(s.dir, 0o750); err != nil {
		return fmt.Errorf("creating terminology recording directory %s: %w", s.dir, err)
	}
	return os.WriteFile(filepath.Join(s.dir, manifestName), data, 0o600)
}

// ReadManifest returns the recording's provenance, or nil when there is none.
func (s *Store) ReadManifest() *Manifest {
	data, err := os.ReadFile(filepath.Join(s.dir, manifestName)) //nolint:gosec // path built from the recording directory
	if err != nil {
		return nil
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return &m
}
