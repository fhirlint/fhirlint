// Package baseline implements the fhirlint-baseline.json file for suppressing
// pre-existing validation issues and failing only on regressions.
package baseline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fhirlint/fhirlint/internal/validator"
)

// Entry records one class of issue in the baseline.
// Count tracks how many occurrences of this issue were present at generation time.
type Entry struct {
	File      string `json:"file"`
	MessageID string `json:"messageId"`
	Location  string `json:"location"` // normalized: no line/col suffix
	Count     int    `json:"count"`
}

// BaselineFile is the in-memory representation of fhirlint-baseline.json.
type BaselineFile struct {
	Entries []Entry `json:"entries"`
}

// normalizeLocation strips the " (line X, col Y)" suffix so that the key
// remains stable across reformatting.
func normalizeLocation(loc string) string {
	if i := strings.Index(loc, " (line "); i >= 0 {
		return loc[:i]
	}
	return loc
}

// relPath converts an absolute file path to a slash-separated path relative to
// the current working directory. If the path is already relative, it is returned
// as-is (with forward slashes). This keeps the baseline portable across machines.
func relPath(file string) string {
	if !filepath.IsAbs(file) {
		return filepath.ToSlash(file)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return filepath.ToSlash(file)
	}
	rel, err := filepath.Rel(cwd, file)
	if err != nil {
		return filepath.ToSlash(file)
	}
	return filepath.ToSlash(rel)
}

func entryKey(file, messageID, location string) string {
	return file + "\x00" + messageID + "\x00" + location
}

// Generate creates a BaselineFile from the active issues in results.
// Issues are grouped by (file, messageId, location) and counted.
func Generate(results []*validator.Result) *BaselineFile {
	type countKey struct{ file, messageID, location string }
	counts := make(map[countKey]int)

	for _, r := range results {
		f := relPath(r.Filename)
		for _, issue := range r.Issues {
			loc := normalizeLocation(issue.Location)
			counts[countKey{f, issue.MessageID, loc}]++
		}
	}

	entries := make([]Entry, 0, len(counts))
	for k, count := range counts {
		entries = append(entries, Entry{
			File:      k.file,
			MessageID: k.messageID,
			Location:  k.location,
			Count:     count,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].File != entries[j].File {
			return entries[i].File < entries[j].File
		}
		if entries[i].Location != entries[j].Location {
			return entries[i].Location < entries[j].Location
		}
		return entries[i].MessageID < entries[j].MessageID
	})

	return &BaselineFile{Entries: entries}
}

// Read parses the baseline file at path.
// Returns nil, nil if the file does not exist.
func Read(path string) (*BaselineFile, error) {
	data, err := os.ReadFile(path) //nolint:gosec // caller-supplied path
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var bf BaselineFile
	if err := json.Unmarshal(data, &bf); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &bf, nil
}

// Write serialises bf as pretty-printed JSON and writes it to path.
func Write(path string, bf *BaselineFile) error {
	data, err := json.MarshalIndent(bf, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0600) //nolint:gosec // baseline file in project dir
}

// Apply suppresses issues that match baseline entries by moving them into
// r.Suppressed with SuppressReason "baseline". Results are mutated in-place.
// Returns the total number of stale occurrences (baseline entries that were not
// matched by any active issue — these are issues that have already been fixed).
func Apply(results []*validator.Result, bf *BaselineFile) int {
	remaining := make(map[string]int, len(bf.Entries))
	for _, e := range bf.Entries {
		k := entryKey(e.File, e.MessageID, e.Location)
		remaining[k] += e.Count
	}

	for _, r := range results {
		f := relPath(r.Filename)
		var active []validator.Issue
		for _, issue := range r.Issues {
			loc := normalizeLocation(issue.Location)
			k := entryKey(f, issue.MessageID, loc)
			if remaining[k] > 0 {
				remaining[k]--
				issue.SuppressReason = "baseline"
				r.Suppressed = append(r.Suppressed, issue)
			} else {
				active = append(active, issue)
			}
		}
		r.Issues = active
		r.Valid = issuesValid(r.Issues)
	}

	stale := 0
	for _, v := range remaining {
		stale += v
	}
	return stale
}

func issuesValid(issues []validator.Issue) bool {
	for _, iss := range issues {
		if iss.Severity == "error" || iss.Severity == "fatal" {
			return false
		}
	}
	return true
}
