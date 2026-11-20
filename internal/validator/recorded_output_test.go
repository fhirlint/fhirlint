// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package validator

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateRecorded = flag.Bool("update-recorded", false,
	"rewrite expected.json in testdata/validator-output from the recordings")

// goldenReading is the part of a runReading a recording pins: what fhirlint
// makes of the validator's output, in a form that diffs well.
type goldenReading struct {
	Error string `json:"error,omitempty"`
	// Stderr is what fhirlint itself printed while reading the run: the
	// explanation that goes with an error.
	Stderr  string         `json:"stderr,omitempty"`
	Crash   *goldenCrash   `json:"crash,omitempty"`
	Results []goldenResult `json:"results,omitempty"`
}

type goldenCrash struct {
	File      string `json:"file"`
	Exception string `json:"exception"`
}

type goldenResult struct {
	File             string   `json:"file"`
	Valid            bool     `json:"valid"`
	ValidatorBuild   string   `json:"validatorBuild,omitempty"`
	DurationMs       int64    `json:"durationMs,omitempty"`
	ImplicitPackages []string `json:"implicitPackages,omitempty"`
	Issues           []Issue  `json:"issues"`
}

func toGolden(r runReading, paths []string, err error) goldenReading {
	if err != nil {
		return goldenReading{Error: err.Error()}
	}
	if r.results == nil {
		return goldenReading{Crash: &goldenCrash{File: paths[r.crashed], Exception: r.exception}}
	}
	var g goldenReading
	for _, res := range r.results {
		gr := goldenResult{
			File:           res.Filename,
			Valid:          res.Valid,
			ValidatorBuild: res.ValidatorBuild,
			DurationMs:     res.DurationMs,
			Issues:         res.Issues,
		}
		for _, p := range res.ImplicitPackages {
			gr.ImplicitPackages = append(gr.ImplicitPackages, p.String())
		}
		g.Results = append(g.Results, gr)
	}
	return g
}

// recordedVersions lists the validator versions there are recordings of.
func recordedVersions(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(recordedOutputDir)
	if err != nil {
		t.Fatal(err)
	}
	var versions []string
	for _, e := range entries {
		if e.IsDir() && e.Name() != recordedInputsDir {
			versions = append(versions, e.Name())
		}
	}
	if len(versions) == 0 {
		t.Fatalf("no recordings in %s", recordedOutputDir)
	}
	return versions
}

func loadRecordedCatalog(t *testing.T, dir string) *messageCatalog {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "Messages.properties.gz")) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatal(err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	c, err := parseMessageCatalog(zr)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func loadRecordedRun(t *testing.T, dir string) (jarRun, []string) {
	t.Helper()
	read := func(name string) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // test fixture path
		if err != nil && (!errors.Is(err, os.ErrNotExist) || name != "output.json") {
			t.Fatal(err)
		}
		// A Windows checkout may turn the recordings' line ends into CRLF.
		return bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	}
	var meta recordedRun
	if err := json.Unmarshal(read("run.json"), &meta); err != nil {
		t.Fatal(err)
	}
	return jarRun{
		json:   read("output.json"),
		stdout: string(read("stdout.txt")),
		stderr: string(read("stderr.txt")),
		failed: meta.Failed,
	}, meta.Paths
}

// TestRecordedValidatorOutput parses the real output of each recorded
// validator version and compares it with expected.json (#715). A validator
// release that changes its OperationOutcome, its console lines or its message
// ids shows up here as soon as it is recorded, not first in the integration
// suite or the canary. Re-record with scripts/record-validator-output.sh.
//
// expected.json pins the details. The checks per case pin what the case is
// for, so that rewriting expected.json cannot quietly accept a regression.
func TestRecordedValidatorOutput(t *testing.T) {
	for _, version := range recordedVersions(t) {
		t.Run(version, func(t *testing.T) {
			vdir := filepath.Join(recordedOutputDir, version)
			catalog := loadRecordedCatalog(t, vdir)
			for _, c := range recordedCases {
				t.Run(c.name, func(t *testing.T) {
					dir := filepath.Join(vdir, c.name)
					if _, err := os.Stat(dir); err != nil {
						t.Fatalf("not recorded for %s: run scripts/record-validator-output.sh %s", version, version)
					}
					out, paths := loadRecordedRun(t, dir)
					if !equalStrings(paths, c.paths) {
						t.Fatalf("recorded with paths %v, the case now has %v: re-record", paths, c.paths)
					}
					var reading runReading
					var err error
					printed := captureStderr(t, func() {
						reading, err = readRun(out, paths, c.opts, catalog)
					})
					got := toGolden(reading, paths, err)
					got.Stderr = printed

					checkRecordedCase(t, version, c.name, got)

					data, mErr := json.MarshalIndent(got, "", "  ")
					if mErr != nil {
						t.Fatal(mErr)
					}
					data = append(data, '\n')
					expected := filepath.Join(dir, "expected.json")
					if *updateRecorded {
						if err := os.WriteFile(expected, data, 0o600); err != nil {
							t.Fatal(err)
						}
						return
					}
					want, rErr := os.ReadFile(expected) //nolint:gosec // test fixture path
					if rErr != nil {
						t.Fatalf("%v: run the test with -update-recorded", rErr)
					}
					if !bytes.Equal(data, bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))) {
						t.Errorf("%s differs from what fhirlint now makes of the recording; if the change is intended, run with -update-recorded and review the diff\ngot:\n%s", expected, data)
					}
				})
			}
		})
	}
}

// captureStderr returns what f printed to os.Stderr.
func captureStderr(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stderr
	os.Stderr = w
	done := make(chan string)
	go func() {
		var b bytes.Buffer
		_, _ = b.ReadFrom(r)
		done <- b.String()
	}()
	defer func() {
		os.Stderr = saved
	}()
	f()
	_ = w.Close()
	return <-done
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// checkRecordedCase asserts what each case exists to show, on every version.
func checkRecordedCase(t *testing.T, version, name string, g goldenReading) {
	t.Helper()
	major7 := strings.HasPrefix(version, "7.")

	if name != "crash" && name != "tx-refused" && name != "tx-unreachable" {
		if g.Error != "" || g.Crash != nil {
			t.Fatalf("want results, got error %q, crash %+v", g.Error, g.Crash)
		}
		for _, r := range g.Results {
			// The validator-version extension is 7.0's (#452); before it the
			// build line is on the console only.
			if major7 && !strings.Contains(r.ValidatorBuild, "Version "+version) {
				t.Errorf("%s: validatorBuild = %q, want it to name %s", r.File, r.ValidatorBuild, version)
			}
			if !major7 && r.ValidatorBuild != "" {
				t.Errorf("%s: validatorBuild = %q from a 6.x OperationOutcome", r.File, r.ValidatorBuild)
			}
		}
	}

	switch name {
	case "single-file":
		if len(g.Results) != 1 || g.Results[0].Valid {
			t.Fatalf("want one invalid result, got %+v", g.Results)
		}
		requireStatedIDs(t, g.Results[0])
	case "two-files":
		if len(g.Results) != 2 {
			t.Fatalf("want two results from the Bundle, got %d", len(g.Results))
		}
		if r := g.Results[0]; r.File != "in/patient.json" || !r.Valid {
			t.Errorf("first: %s valid=%v, want in/patient.json valid", r.File, r.Valid)
		}
		if r := g.Results[1]; r.File != "in/bad-gender.json" || r.Valid {
			t.Errorf("second: %s valid=%v, want in/bad-gender.json invalid", r.File, r.Valid)
		}
		requireStatedIDs(t, g.Results[1])
	case "crash":
		if g.Crash == nil || g.Crash.File != "in/bodystructure.json" || g.Crash.Exception == "" {
			t.Fatalf("want the crash pinned on in/bodystructure.json with its exception, got %+v", g)
		}
	case "snapshot-failure":
		found := false
		for _, r := range g.Results {
			for _, iss := range r.Issues {
				if strings.Contains(iss.Cause, "does not repeat") {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("no issue carries the snapshot failure's cause from stdout")
		}
	case "tx-refused":
		if !strings.Contains(g.Error, "refused by SSRF protection") ||
			!strings.Contains(g.Stderr, "Refusing to fetch from non-https URL") {
			t.Errorf("want the SSRF refusal with the validator's reason, got %+v", g)
		}
	case "tx-unreachable":
		if g.Error != "terminology server unreachable" ||
			!strings.Contains(g.Stderr, "Cannot reach the terminology server https://tx.invalid") {
			t.Errorf("want the unreachable-server error naming the server, got %+v", g)
		}
	case "tx-no-id":
		if len(g.Results) != 1 {
			t.Fatalf("want one result, got %d", len(g.Results))
		}
		// After inference every finding must carry an id, stated or recovered:
		// baselines and suppressions key on it (#449).
		for _, iss := range g.Results[0].Issues {
			if iss.MessageID == "" && iss.Severity != "information" {
				t.Errorf("%s without a message id: %q", iss.Severity, iss.Message)
			}
		}
	default:
		t.Fatalf("case %q has no check", name)
	}
}

// requireStatedIDs fails for an error or warning the validator gave no id.
// These cases run without a terminology server, so every id is the
// validator's own.
func requireStatedIDs(t *testing.T, r goldenResult) {
	t.Helper()
	for _, iss := range r.Issues {
		if iss.Severity == "information" {
			continue
		}
		if iss.MessageID == "" || iss.MessageIDInferred {
			t.Errorf("%s: %s %q has no message id of the validator's own", r.File, iss.Severity, iss.Message)
		}
	}
}

// The normaliser takes out the machine and the moment, and nothing else.
func TestRecordingNormaliser(t *testing.T) {
	n := recordingNormaliser{work: []string{"/private/tmp/rec123", "/tmp/rec123"}, home: "/Users/someone"}
	in := "FHIR Validation tool Version 7.0.6 (Git# abc). Built 2026-11-13T10:00:00.000Z (6 days old)\n" +
		"  Java:   21.0.13 from /usr/lib/jvm/java-21 on amd64 (64bit). 3200MB available\n" +
		"  Paths:  Current = /private/tmp/rec123, Package Cache = /Users/someone/.fhir/packages\n" +
		"  Params: in/a.json -output /var/folders/x/fhirlint-result-4711.json -txCache /tmp/rec123/tx-cache\n" +
		"  Load hl7.terminology.r4#6.2.0 - 4288 resources (00:50.433)\n" +
		" 01:02.003\n" +
		"Validate in/a.json against code 12:34 and 2026-11-13T10:00:00.000Z\n"
	want := "FHIR Validation tool Version 7.0.6 (Git# abc). Built 2026-11-13T10:00:00.000Z (1 days old)\n" +
		"  Java:   <java> from /usr/lib/jvm on amd64 (64bit). 1000MB available\n" +
		"  Paths:  Current = /work, Package Cache = /home/user/.fhir/packages\n" +
		"  Params: in/a.json -output out.json -txCache /work/tx-cache\n" +
		"  Load hl7.terminology.r4#6.2.0 - 4288 resources (00:00.100)\n" +
		" 00:00.100\n" +
		"Validate in/a.json against code 12:34 and 2026-11-13T10:00:00.000Z\n"
	if got := n.normalise(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
