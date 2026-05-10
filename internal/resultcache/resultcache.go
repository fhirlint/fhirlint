// Package resultcache stores validation results on disk keyed by file content hash.
// Cache entries are only valid when the file content and all validation options match.
package resultcache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fhirlint/fhirlint/internal/validator"
)

// KeyOpts contains every option that affects validation output and must be part of the key.
type KeyOpts struct {
	FhirlintVersion string
	FHIRVersion     string
	Profiles        []string
	IGs             []string
}

// Key computes a SHA-256 hex key from the file content and validation options.
// Two calls with identical content and options always produce the same key.
func Key(filePath string, opts KeyOpts) (string, error) {
	f, err := os.Open(filePath) //nolint:gosec // user-supplied path, intentional
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	profiles := append([]string(nil), opts.Profiles...)
	igs := append([]string(nil), opts.IGs...)
	sort.Strings(profiles)
	sort.Strings(igs)

	_, _ = fmt.Fprintf(h, "\x00%s\x00%s\x00%s\x00%s",
		opts.FhirlintVersion,
		opts.FHIRVersion,
		strings.Join(profiles, ","),
		strings.Join(igs, ","),
	)

	return hex.EncodeToString(h.Sum(nil)), nil
}

// Entry is the data stored for one cache hit.
type Entry struct {
	CachedAt        time.Time        `json:"cachedAt"`
	FhirlintVersion string           `json:"fhirlintVersion"`
	Result          validator.Result `json:"result"`
}

// Get retrieves a cached result. Returns os.ErrNotExist when there is no entry for key.
func Get(cacheDir, key string) (*Entry, error) {
	data, err := os.ReadFile(entryPath(cacheDir, key)) //nolint:gosec // path is derived from hex key
	if err != nil {
		return nil, err
	}
	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

// Put writes a result to the cache, creating the cache directory if needed.
func Put(cacheDir, key string, entry Entry) error {
	if err := os.MkdirAll(cacheDir, 0750); err != nil {
		return err
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return os.WriteFile(entryPath(cacheDir, key), data, 0600) //nolint:gosec // path is derived from hex key
}

// Clear removes all .json cache entries from cacheDir.
// It does nothing and returns nil when cacheDir does not exist.
func Clear(cacheDir string) (int, error) {
	entries, err := os.ReadDir(cacheDir)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			if rerr := os.Remove(filepath.Join(cacheDir, e.Name())); rerr == nil {
				removed++
			}
		}
	}
	return removed, nil
}

func entryPath(cacheDir, key string) string {
	return filepath.Join(cacheDir, key+".json")
}
