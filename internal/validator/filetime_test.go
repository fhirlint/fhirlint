// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package validator

import (
	"testing"
)

// The recorded runs from #521 log each file's time after its messages.
func TestFileDurations_RecordedStdout(t *testing.T) {
	for v, want := range map[string]map[int]int64{
		"6.10.3": {0: 2667, 1: 432},
		"7.0.3":  {0: 4302, 1: 590},
	} {
		got := fileDurations(readFixture(t, "stdout-snapshot-failure-"+v+".txt"), snapshotFixturePaths)
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("%s: got %v, want %v", v, got, want)
		}
	}
}

// 6.10.3, one file and no IG: the validator prints a stack trace and an HTML
// page between the Validate line and the time. Neither is a time.
func TestFileDurations_NoiseBetween(t *testing.T) {
	stdout := "Validating\n  Validate in/patient.json\nXHTML content could not be parsed\n\tat org.hl7.X(X.java:716)\n<!doctype html>\n\n 00:01.320\nDone. Times: Loading: 00:17.024, validation: 00:01.321.\n"
	got := fileDurations(stdout, []string{"in/patient.json"})
	if got[0] != 1320 || len(got) != 1 {
		t.Errorf("got %v", got)
	}
}

// A file the validator did not finish has no time; the next file's time must
// not be credited to it, and a time line outside any file counts for nothing.
func TestFileDurations_MissingAndStray(t *testing.T) {
	stdout := " 00:09.000\n  Validate a.json\n  Validate b.json\n 00:00.250\n 00:07.000\n  Validate unknown.json\n 00:01.000\n"
	got := fileDurations(stdout, []string{"a.json", "b.json"})
	if len(got) != 1 || got[1] != 250 {
		t.Errorf("got %v, want only b.json = 250", got)
	}
}

func TestParseFileTime(t *testing.T) {
	cases := map[string]int64{
		" 00:00.476":  476,
		"00:01.5":     1500,
		"02:03.004":   123004,
		"1:00:00.000": 3600000,
		" 00:00.4761": 476,
	}
	for in, want := range cases {
		if got, ok := parseFileTime(in); !ok || got != want {
			t.Errorf("parseFileTime(%q) = %d, %v; want %d", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "00:00", "a:00.1", "00:00.x", "5.000", "1:2:3:4.000", "-1:00.000"} {
		if _, ok := parseFileTime(bad); ok {
			t.Errorf("parseFileTime(%q) accepted", bad)
		}
	}
}

// FuzzFileDurations: arbitrary stdout never panics, and every duration
// belongs to a file of the run and is not negative.
func FuzzFileDurations(f *testing.F) {
	f.Add("  Validate a.json\n 00:00.476\n")
	f.Add("  Validate b.json\n 99999999999:00.1\n")
	f.Add("  Validate a.json\n 2147483648:2147483648:2147483648.999\n")
	paths := []string{"a.json", "b.json"}
	f.Fuzz(func(t *testing.T, stdout string) {
		for i, ms := range fileDurations(stdout, paths) {
			if i < 0 || i >= len(paths) || ms < 0 {
				t.Fatalf("duration %d for index %d", ms, i)
			}
		}
	})
}
