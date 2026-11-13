package validator

import (
	"strconv"
	"strings"
)

// fileDurations reads the time the validator logs after each file of a
// multi-file run (#522): "  Validate <path>", the file's messages, then
// " 00:00.476". It returns the milliseconds per index into paths.
//
// A file without its time line, such as the one a crash interrupted, has no
// entry: fhirlint never invents a duration. The time excludes package loading,
// lazy loads during the file included (#520): measured with 6.10.3, a US Core
// Patient logged 00:00.815 after 15 loads that took over 20 s.
func fileDurations(stdout string, paths []string) map[int]int64 {
	out := map[int]int64{}
	current := -1
	ix := newPathIndex(paths)
	for _, l := range strings.Split(stripANSI(stdout), "\n") {
		if m := validateLine.FindStringSubmatch(l); m != nil {
			current = ix.lookup(m[1])
			continue
		}
		if current < 0 || !fileTimeLine.MatchString(l) {
			continue
		}
		if ms, ok := parseFileTime(l); ok {
			out[current] = ms
		}
		current = -1
	}
	return out
}

// parseFileTime parses the validator's elapsed-time format, "mm:ss.fff", with
// an hours field in front once a file takes that long.
func parseFileTime(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	whole, frac, ok := strings.Cut(s, ".")
	if !ok {
		return 0, false
	}
	parts := strings.Split(whole, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	var secs int64
	for _, part := range parts {
		n, err := strconv.ParseInt(part, 10, 64)
		if err != nil || n < 0 || n > 1<<31 {
			return 0, false
		}
		secs = secs*60 + n
	}
	// Milliseconds: the first three digits of the fraction, padded.
	frac = (frac + "000")[:3]
	ms, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, false
	}
	return secs*1000 + ms, true
}

// attachDurations sets DurationMs on the results of one validator run.
func attachDurations(results []*Result, durations map[int]int64) {
	for i, ms := range durations {
		if i < len(results) && results[i] != nil {
			results[i].DurationMs = ms
		}
	}
}
