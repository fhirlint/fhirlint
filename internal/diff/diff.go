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

	// MessageIDInferred carries validator.Issue.MessageIDInferred: fhirlint
	// recovered the id from the message text (#449).
	MessageIDInferred bool `json:"messageIdInferred,omitempty"`
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
	pairInferredIDs(res)

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
				File:              file,
				Severity:          iss.Severity,
				MessageID:         iss.MessageID,
				Location:          iss.Location,
				Message:           iss.Message,
				MessageIDInferred: iss.MessageIDInferred,
			})
		}
	}
	return out
}

// pairInferredIDs moves a new and a resolved issue to unchanged when they are
// the same finding with and without an inferred message id: one side's id was
// recovered from the text (#449), the other side has none, and file and
// location agree. That is a report from before the inference compared with one
// from after it, or a JAR whose message bundle could not be read; either way
// the validator said the same thing, at the same severity, at the same place.
func pairInferredIDs(res *Result) {
	if len(res.New) == 0 || len(res.Resolved) == 0 {
		return
	}
	matches := func(a, b Issue) bool {
		return a.File == b.File && a.Severity == b.Severity &&
			normalizeLocation(a.Location) == normalizeLocation(b.Location) &&
			((a.MessageIDInferred && b.MessageID == "") || (b.MessageIDInferred && a.MessageID == ""))
	}
	var stillNew []Issue
	for _, n := range res.New {
		paired := false
		for j, r := range res.Resolved {
			if matches(n, r) {
				res.Resolved = append(res.Resolved[:j], res.Resolved[j+1:]...)
				res.Unchanged = append(res.Unchanged, n)
				paired = true
				break
			}
		}
		if !paired {
			stillNew = append(stillNew, n)
		}
	}
	if stillNew == nil {
		stillNew = []Issue{}
	}
	res.New = stillNew
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
