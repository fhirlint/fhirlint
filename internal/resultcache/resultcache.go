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
	"time"

	"github.com/fhirlint/fhirlint/internal/validator"
)

// KeyOpts carries everything outside the file's own bytes that decides what
// the validator reports.
//
// The key is derived from the whole validator.Options value rather than from a
// list of the fields that happen to matter. An allowlist has to be remembered
// on every new flag, and #417 is what forgetting it looks like: --best-practice,
// --jurisdiction, --locale and --display-issues-are-warnings each change the
// result, none of them reached the key, and two runs differing only in those
// shared an entry — the second silently inheriting the first one's findings and
// exit code. A denylist fails the other way round: forgetting to exclude
// something costs a cache hit, not a wrong answer.
type KeyOpts struct {
	FhirlintVersion string

	// ValidatorVersion is the *effective* JAR version, not the
	// --validator-version flag. An empty flag means "whatever is installed",
	// and `fhirlint update` changes that underneath an unchanged command line.
	// Use validator.EffectiveValidatorVersion to resolve it.
	ValidatorVersion string

	// Options is the run's full option set. Fields that cannot change the
	// result, and those represented below by a fingerprint, are dropped from
	// the key by keyedOptions.
	Options validator.Options

	// Fingerprints of the file-valued options, standing in for their paths:
	// editing one of these files must invalidate the entry, and moving the same
	// file elsewhere must not throw it away (#407).
	ExpansionParameters string
	FHIRSettings        string
	POFiles             []string
}

// keyedOptions returns the part of an Options value that belongs in the key.
//
// Only two reasons to drop a field: it provably cannot change what the
// validator reports, or KeyOpts already carries it in a better form. Anything
// else stays, including options whose effect is arguable — a needless cache
// miss is the cheaper mistake.
func keyedOptions(o validator.Options) validator.Options {
	// Cannot change the result. Timeout bounds how long fhirlint waits before
	// giving up, which produces an error rather than a verdict; TxLog is a
	// debug artefact written beside the run. Note that ValidationTimeout and
	// MaxMessages are *not* here: both cut a run short and return partial
	// results, so they change what the user is shown.
	o.Timeout = 0
	o.TxLog = ""

	// Carried by KeyOpts.ValidatorVersion, which resolves an explicit pin, an
	// explicit JAR path and the installed default to the one thing that
	// matters — the version that will actually run.
	o.JARPath = ""
	o.ValidatorVersion = ""

	// Carried by KeyOpts as content fingerprints.
	o.ExpansionParameters = ""
	o.FHIRSettings = ""
	o.POFiles = nil

	// Order carries no meaning for these two, and sorting keeps one entry
	// shared across argument orders. ExtraArgs is deliberately not sorted: it
	// is handed to the JAR verbatim, where order does mean something. Neither
	// are the POFiles fingerprints in KeyOpts, for the same reason — a later
	// translation file overrides an earlier one.
	o.Profiles = sortedCopy(o.Profiles)
	o.IGs = sortedCopy(o.IGs)

	return o
}

func sortedCopy(in []string) []string {
	if in == nil {
		return nil
	}
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
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

	// A struct marshals in field-declaration order, so this is stable across
	// runs and across Go versions in a way a map would not be.
	canonical, err := json.Marshal(struct {
		FhirlintVersion     string
		ValidatorVersion    string
		Options             validator.Options
		ExpansionParameters string
		FHIRSettings        string
		POFiles             []string
	}{
		FhirlintVersion:     opts.FhirlintVersion,
		ValidatorVersion:    opts.ValidatorVersion,
		Options:             keyedOptions(opts.Options),
		ExpansionParameters: opts.ExpansionParameters,
		FHIRSettings:        opts.FHIRSettings,
		POFiles:             opts.POFiles,
	})
	if err != nil {
		return "", fmt.Errorf("building cache key: %w", err)
	}

	_, _ = h.Write([]byte{0})
	_, _ = h.Write(canonical)

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

// Clear removes all .json cache entries from cacheDir, returning how many it
// removed. It does nothing and returns nil when cacheDir does not exist.
//
// Entries that cannot be removed are reported rather than skipped in silence:
// "Removed 0 cached result(s)" over a directory that is still full reads as
// success, and the next run then serves from a cache the user believes they
// emptied (#316). Removal continues past a failure — clearing what can be
// cleared is still worth doing — and the count reflects what actually went.
func Clear(cacheDir string) (int, error) {
	entries, err := os.ReadDir(cacheDir)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	removed, failed := 0, 0
	var firstErr error
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		rerr := os.Remove(filepath.Join(cacheDir, e.Name()))
		switch {
		case rerr == nil, os.IsNotExist(rerr):
			// Already gone counts as cleared: a concurrent run removing the same
			// entry is not a failure to report.
			if rerr == nil {
				removed++
			}
		default:
			failed++
			if firstErr == nil {
				firstErr = rerr
			}
		}
	}
	if failed > 0 {
		return removed, fmt.Errorf("%d cache entr%s could not be removed: %w",
			failed, plural(failed, "y", "ies"), firstErr)
	}
	return removed, nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func entryPath(cacheDir, key string) string {
	return filepath.Join(cacheDir, key+".json")
}
