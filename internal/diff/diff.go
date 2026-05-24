// Package diff compares two validation runs and categorises every issue as
// new, resolved, or unchanged. It is the engine behind the `fhirlint diff`
// command used for change-control evidence in regulated environments.
package diff

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/fhirlint/fhirlint/internal/validator"
)

// Issue is a single categorised finding in a diff.
type Issue struct {
	File      string `json:"file"`
	Severity  string `json:"severity"`
	MessageID string `json:"messageId"`
	Location  string `json:"location"`
	Message   string `json:"message"`
}

// Result holds the outcome of comparing a baseline run against a current run.
type Result struct {
	New       []Issue `json:"new"`
	Resolved  []Issue `json:"resolved"`
	Unchanged []Issue `json:"unchanged"`
}

// severityOrder ranks severities so a minimum-severity filter can be applied.
var severityOrder = map[string]int{"information": 0, "warning": 1, "error": 2, "fatal": 3}

// Compute compares baseline against current and returns the categorised diff.
// Only issues at or above minSeverity are considered. An issue is identified by
// the tuple (file, messageId, normalized location); line/column suffixes are
// stripped from the location so findings stay matched across reformatting.
// Occurrence counts are honoured: if a file gains a second copy of the same
// issue, the extra copy is reported as new.
func Compute(baseline, current []*validator.Result, minSeverity string) *Result {
	baseByKey := groupByKey(baseline, minSeverity)
	curByKey := groupByKey(current, minSeverity)

	res := &Result{New: []Issue{}, Resolved: []Issue{}, Unchanged: []Issue{}}

	for key, cur := range curByKey {
		base := baseByKey[key]
		common := min(len(base), len(cur))
		res.Unchanged = append(res.Unchanged, cur[:common]...)
		res.New = append(res.New, cur[common:]...)
	}
	for key, base := range baseByKey {
		cur := curByKey[key]
		if len(base) > len(cur) {
			res.Resolved = append(res.Resolved, base[len(cur):]...)
		}
	}

	sortIssues(res.New)
	sortIssues(res.Resolved)
	sortIssues(res.Unchanged)
	return res
}

// groupByKey flattens results into a map of issue-key → occurrences, keeping the
// original (un-normalized) location on each Issue for display and SARIF output.
func groupByKey(results []*validator.Result, minSeverity string) map[string][]Issue {
	minRank := severityOrder[minSeverity]
	out := make(map[string][]Issue)
	for _, r := range results {
		file := filepath.ToSlash(r.Filename)
		for _, iss := range r.Issues {
			if severityOrder[iss.Severity] < minRank {
				continue
			}
			key := file + "\x00" + iss.MessageID + "\x00" + normalizeLocation(iss.Location)
			out[key] = append(out[key], Issue{
				File:      file,
				Severity:  iss.Severity,
				MessageID: iss.MessageID,
				Location:  iss.Location,
				Message:   iss.Message,
			})
		}
	}
	return out
}

// normalizeLocation strips the " (line X, col Y)" suffix so the key stays stable
// when only line numbers shift between runs.
func normalizeLocation(loc string) string {
	if i := strings.Index(loc, " (line "); i >= 0 {
		return loc[:i]
	}
	return loc
}

func sortIssues(issues []Issue) {
	sort.SliceStable(issues, func(i, j int) bool {
		a, b := issues[i], issues[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Location != b.Location {
			return a.Location < b.Location
		}
		return a.MessageID < b.MessageID
	})
}
