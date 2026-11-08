package validator

import (
	"os"
	"strings"
	"testing"
	"unicode"
)

const slicedProfile = "http://example.org/fhir/StructureDefinition/Slice-nonrepeating|1.0.0"

// The profile from the chat.fhir.org thread in #521: a differential that
// slices the non-repeating Patient.maritalStatus, validated with two Patients
// claiming it. The cause is on stdout five times per file, and nowhere else.
func readFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

var snapshotFixturePaths = []string{"in/patient.json", "in/patient2.json"}

func TestSnapshotFailures_RecordedStdout(t *testing.T) {
	for _, v := range []string{"6.10.3", "7.0.3"} {
		t.Run(v, func(t *testing.T) {
			got := snapshotFailures(readFixture(t, "stdout-snapshot-failure-"+v+".txt"), snapshotFixturePaths)
			if len(got) != 1 {
				t.Fatalf("got %d failures, want 1 (the line repeats per pass): %+v", len(got), got)
			}
			f := got[0]
			if f.canonical != slicedProfile {
				t.Errorf("canonical = %q", f.canonical)
			}
			if !strings.HasPrefix(f.reason, "Attempt to a slice an element that does not repeat: Patient.maritalStatus") ||
				!strings.HasSuffix(f.reason, "at element married (slice = {5})") {
				t.Errorf("reason = %q", f.reason)
			}
			if strings.ContainsAny(f.reason, "\x1b\x07") {
				t.Errorf("reason keeps a control character: %q", f.reason)
			}
			if f.file != -1 {
				t.Errorf("file = %d, want -1: it is first logged while loading", f.file)
			}
		})
	}
}

func TestAttachSnapshotFailures_RecordedRun(t *testing.T) {
	results, err := parseOutput([]byte(readFixture(t, "oo-snapshot-failure-6.10.3.json")), snapshotFixturePaths, "")
	if err != nil {
		t.Fatal(err)
	}
	attachSnapshotFailures(results, snapshotFailures(readFixture(t, "stdout-snapshot-failure-6.10.3.txt"), snapshotFixturePaths))

	for _, r := range results {
		var found bool
		for _, iss := range r.Issues {
			if iss.MessageID == messageIDNoSnapshot {
				found = true
				if !strings.Contains(iss.Cause, "does not repeat: Patient.maritalStatus") {
					t.Errorf("%s: cause = %q", r.Filename, iss.Cause)
				}
			} else if iss.Cause != "" {
				t.Errorf("%s: %s got a cause it is not about", r.Filename, iss.MessageID)
			}
			if iss.MessageID == MessageIDSnapshotFailed {
				t.Errorf("%s: a failure that an issue names became an issue of its own", r.Filename)
			}
		}
		if !found {
			t.Errorf("%s: no %s issue in the fixture", r.Filename, messageIDNoSnapshot)
		}
	}
}

// A failure no issue names, e.g. a profile in the IG that no file claims,
// must not vanish: it becomes an error on the file it was logged during.
func TestAttachSnapshotFailures_UnmatchedBecomesIssue(t *testing.T) {
	stdout := "  Validate a.json\n 00:00.100\n  Validate b.json\n" +
		"\x1b[1;31mException generating snapshot for http://x.org/SD/other|2.0: bad \x1b]8;;u\x07slice\n 00:00.200\n"
	results := []*Result{{Filename: "a.json", Valid: true}, {Filename: "b.json", Valid: true}}
	attachSnapshotFailures(results, snapshotFailures(stdout, []string{"a.json", "b.json"}))

	if len(results[0].Issues) != 0 || !results[0].Valid {
		t.Errorf("a.json got %+v", results[0].Issues)
	}
	if len(results[1].Issues) != 1 {
		t.Fatalf("b.json got %+v", results[1].Issues)
	}
	iss := results[1].Issues[0]
	if iss.MessageID != MessageIDSnapshotFailed || iss.Severity != "error" || results[1].Valid {
		t.Errorf("issue = %+v, valid = %v", iss, results[1].Valid)
	}
	if !strings.Contains(iss.Message, "http://x.org/SD/other|2.0") || iss.Cause != "bad ]8;;uslice" {
		t.Errorf("issue = %+v", iss)
	}
}

// Logged while loading and named by no issue: the first file carries it.
func TestAttachSnapshotFailures_LoadPhaseGoesToFirstFile(t *testing.T) {
	stdout := "Exception generating snapshot for http://x.org/SD/p: why\n  Validate a.json\n 00:00.1\n"
	results := []*Result{{Filename: "a.json", Valid: true}, {Filename: "b.json", Valid: true}}
	attachSnapshotFailures(results, snapshotFailures(stdout, []string{"a.json", "b.json"}))
	if len(results[0].Issues) != 1 || len(results[1].Issues) != 0 {
		t.Errorf("issues: %+v / %+v", results[0].Issues, results[1].Issues)
	}
}

func TestNamesFailedSnapshot(t *testing.T) {
	msg := func(c string) Issue {
		return Issue{MessageID: messageIDNoSnapshot, Message: "StructureDefinition " + c + " has no snapshot - validation is against the snapshot"}
	}
	cases := []struct {
		iss       Issue
		canonical string
		want      bool
	}{
		{msg("http://x/SD/p|1.0"), "http://x/SD/p|1.0", true},
		{msg("http://x/SD/p"), "http://x/SD/p|1.0", true},
		{msg("http://x/SD/p"), "http://x/SD/p", true},
		{msg("http://x/SD/p-extended"), "http://x/SD/p", false},
		{msg("http://x/SD/p|2.0"), "http://x/SD/p|1.0", false},
		{Issue{MessageID: "other", Message: "about http://x/SD/p|1.0 here"}, "http://x/SD/p|1.0", false},
	}
	for _, c := range cases {
		if got := namesFailedSnapshot(c.iss, c.canonical); got != c.want {
			t.Errorf("namesFailedSnapshot(%q, %q) = %v", c.iss.Message, c.canonical, got)
		}
	}
}

// FuzzSnapshotFailures feeds arbitrary validator stdout to the parser: it must
// not panic, every reason is free of control characters and capped, and every
// failure points at a file of the run or at the load phase.
func FuzzSnapshotFailures(f *testing.F) {
	f.Add("Exception generating snapshot for http://x/SD/p|1: why\n  Validate a.json\n")
	f.Add("\x1b[1;31mException generating snapshot for a: b\x1b]8;;u\x07c\n")
	f.Add("  Validate b.json\nException generating snapshot for x: " + strings.Repeat("ä", 3000) + "\n")
	paths := []string{"a.json", "b.json"}
	f.Fuzz(func(t *testing.T, stdout string) {
		for _, fl := range snapshotFailures(stdout, paths) {
			if strings.ContainsFunc(fl.reason, isControl) || strings.ContainsFunc(fl.canonical, isControl) {
				t.Fatalf("control character in %+v", fl)
			}
			if fl.file < -1 || fl.file >= len(paths) {
				t.Fatalf("file index %d", fl.file)
			}
		}
	})
}

func isControl(r rune) bool { return unicode.IsControl(r) }
