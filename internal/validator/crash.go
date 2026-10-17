package validator

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Message ids of the issues fhirlint reports in place of the validator's own
// when a multi-file run crashes (#471).
const (
	// MessageIDValidatorCrash marks the file the validator threw on.
	MessageIDValidatorCrash = "fhirlint-validator-crash"
	// MessageIDNotValidated marks a file fhirlint gave up on after crashes or
	// when --timeout ran out while re-running the rest.
	MessageIDNotValidated = "fhirlint-not-validated"
)

// maxValidatorCrashes bounds how often one run re-starts the JVM after a
// crash. Each re-run costs a JVM start and a package load, so a directory full
// of files that crash the validator must not cost one of those per file.
const maxValidatorCrashes = 3

// maxCrashMessageLen caps the exception line in a crash issue. The
// validator's lines run to well over a kilobyte (the snapshot exception lists
// every allowed type), which is noise in a terminal report.
const maxCrashMessageLen = 400

// validateLine is what the validator logs before it starts on each file.
var validateLine = regexp.MustCompile(`^\s*Validate (.+?)\s*$`)

// fileTimeLine is what it logs after a file it finished, e.g. " 00:00.294".
var fileTimeLine = regexp.MustCompile(`^\s*\d+:\d+\.\d+\s*$`)

// crashedFile finds the file the validator was working on when it stopped
// without writing output, by the last "Validate <path>" line in its stdout.
// It returns the index of that file in paths and the first line the validator
// logged after it, which is the exception it died on.
//
// It reports false when it cannot say: no Validate line (the crash happened
// while loading, before any file), a path that is not one of ours, or a file
// that was followed by its timing line, which means the validator finished it
// and failed somewhere after. Blaming a file there would be a guess.
func crashedFile(stdout string, paths []string) (idx int, message string, ok bool) {
	lines := strings.Split(stripANSI(stdout), "\n")
	last := -1
	for i, l := range lines {
		if validateLine.MatchString(l) {
			last = i
		}
	}
	if last < 0 {
		return 0, "", false
	}

	logged := validateLine.FindStringSubmatch(lines[last])[1]
	idx = indexOfPath(logged, paths)
	if idx < 0 {
		return 0, "", false
	}

	for _, l := range lines[last+1:] {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if fileTimeLine.MatchString(l) {
			return 0, "", false
		}
		return idx, truncateRunes(l, maxCrashMessageLen), true
	}
	return idx, "the validator stopped without logging an error", true
}

// indexOfPath finds the input the validator named. It logs absolute paths,
// while fhirlint may have passed relative ones.
func indexOfPath(logged string, paths []string) int {
	want := filepath.Clean(logged)
	for i, p := range paths {
		if filepath.Clean(p) == want {
			return i
		}
		if abs, err := filepath.Abs(p); err == nil && abs == want {
			return i
		}
	}
	return -1
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}

// crashResult is the result for the file the validator threw on.
func crashResult(path, exception string) *Result {
	return &Result{
		Filename:   path,
		SourcePath: path,
		Issues: []Issue{{
			Severity:  "fatal",
			Message:   "the validator crashed on this file: " + exception,
			MessageID: MessageIDValidatorCrash,
		}},
	}
}

// notValidatedResult is the result for a file fhirlint did not get a verdict
// for. It is fatal, not a warning: a file nobody validated must not count as
// valid.
func notValidatedResult(path, reason string) *Result {
	return &Result{
		Filename:   path,
		SourcePath: path,
		Issues: []Issue{{
			Severity:  "fatal",
			Message:   "not validated: " + reason,
			MessageID: MessageIDNotValidated,
		}},
	}
}

// Incomplete reports whether fhirlint stood in for the validator's verdict on
// this file, because the validator crashed on it or it was never validated.
// Such a result says nothing about the file's content and must not be cached.
func (r *Result) Incomplete() bool {
	for _, i := range r.Issues {
		if i.MessageID == MessageIDValidatorCrash || i.MessageID == MessageIDNotValidated {
			return true
		}
	}
	return false
}

// crashSummary is the notice on stderr that explains the extra JVM start.
func crashSummary(path string, remaining int) string {
	return fmt.Sprintf("warn: the validator crashed on %s; re-running the other %d file(s) in a new JVM\n", path, remaining)
}
