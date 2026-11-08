package validator

import (
	"regexp"
	"strings"

	"github.com/fhirlint/fhirlint/internal/termsafe"
)

// MessageIDSnapshotFailed marks a profile whose snapshot the validator could
// not generate, when no issue of the run names that profile (#521).
const MessageIDSnapshotFailed = "fhirlint-snapshot-failed"

// messageIDNoSnapshot is the validator's consequence of a failed snapshot:
// "StructureDefinition <canonical> has no snapshot - validation is against the
// snapshot, so it must be provided". It says nothing about the cause.
const messageIDNoSnapshot = "Validation_VAL_Profile_NoSnapshot"

// snapshotExceptionLine carries the cause, on stdout only. 6.10.3 and 7.0.3
// word it the same; it repeats once per validation pass.
var snapshotExceptionLine = regexp.MustCompile(`^Exception generating snapshot for (\S+): (.+)$`)

// snapshotFailure is one profile the validator could not generate a snapshot
// for, and why.
type snapshotFailure struct {
	canonical string
	reason    string
	// file is the index into the run's paths of the file during which the
	// failure was first logged, or -1 when it was logged while loading.
	file int
}

// snapshotFailures collects the distinct snapshot exceptions from the
// validator's stdout, in the order they first appear. The text quotes profile
// content, so control characters are dropped and the reason is capped.
func snapshotFailures(stdout string, paths []string) []snapshotFailure {
	var out []snapshotFailure
	seen := map[string]bool{}
	file := -1
	for _, l := range strings.Split(stripANSI(stdout), "\n") {
		if m := validateLine.FindStringSubmatch(l); m != nil {
			file = indexOfPath(m[1], paths)
			continue
		}
		m := snapshotExceptionLine.FindStringSubmatch(strings.TrimSpace(plainText(l)))
		if m == nil {
			continue
		}
		key := m[1] + "\x00" + m[2]
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, snapshotFailure{canonical: m[1], reason: termsafe.Clip(strings.TrimSpace(m[2])), file: file})
	}
	return out
}

// attachSnapshotFailures carries the cause of each failed snapshot into the
// results. An issue saying a profile "has no snapshot" gets the cause; a
// failure that no issue names becomes an issue of its own, so it cannot vanish:
// on the file it was logged for, or, when it was logged while loading, on the
// first file.
func attachSnapshotFailures(results []*Result, failures []snapshotFailure) {
	if len(results) == 0 {
		return
	}
	for _, f := range failures {
		matched := false
		for _, r := range results {
			if r == nil {
				continue
			}
			for i := range r.Issues {
				if namesFailedSnapshot(r.Issues[i], f.canonical) {
					r.Issues[i].Cause = joinCause(r.Issues[i].Cause, f.reason)
					matched = true
				}
			}
		}
		if matched {
			continue
		}
		target := 0
		if f.file >= 0 && f.file < len(results) {
			target = f.file
		}
		r := results[target]
		if r == nil {
			continue
		}
		r.Issues = append(r.Issues, Issue{
			Severity:  "error",
			Message:   "the validator could not generate the snapshot of " + f.canonical,
			MessageID: MessageIDSnapshotFailed,
			Cause:     f.reason,
		})
		r.Valid = false
	}
}

// namesFailedSnapshot reports whether iss is the validator's "has no snapshot"
// for canonical. The exception names the canonical with its version; the
// message may name it without.
func namesFailedSnapshot(iss Issue, canonical string) bool {
	if iss.MessageID != messageIDNoSnapshot && !strings.Contains(iss.Message, "has no snapshot") {
		return false
	}
	if strings.Contains(iss.Message, canonical+" ") {
		return true
	}
	url, _, versioned := strings.Cut(canonical, "|")
	return versioned && strings.Contains(iss.Message, url+" has no snapshot")
}

// joinCause adds reason to an issue's cause unless it is already there: two
// failures of the same profile with different reasons both belong to it.
func joinCause(cause, reason string) string {
	switch {
	case cause == "":
		return reason
	case strings.Contains(cause, reason):
		return cause
	default:
		return cause + "; " + reason
	}
}
