// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package validator

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// The Go side of a run parses every issue of the validator's output; for a
// whole IG that is tens of thousands. These check that the parsers grow
// linearly with it (#552); internal/scaling covers the stages after them.

// syntheticOutput is the validator's JSON output (a Bundle of
// OperationOutcomes) and stdout for files × perFile issues.
func syntheticOutput(t testing.TB, files, perFile int) (out []byte, stdout string, paths []string) {
	t.Helper()
	type ext struct {
		URL          string `json:"url"`
		ValueInteger int    `json:"valueInteger,omitempty"`
		ValueCode    string `json:"valueCode,omitempty"`
		ValueString  string `json:"valueString,omitempty"`
	}
	type entry struct {
		FullURL  string `json:"fullUrl"`
		Resource any    `json:"resource"`
	}
	var entries []entry
	var sb strings.Builder
	sb.WriteString("  Load hl7.fhir.r4.core#4.0.1 - 4580 resources (00:01.2)\nValidating\n")
	for f := range files {
		p := fmt.Sprintf("fhir/Observation-%05d.json", f)
		paths = append(paths, p)
		var issues []map[string]any
		for i := range perFile {
			issues = append(issues, map[string]any{
				"extension": []ext{
					{URL: "http://hl7.org/fhir/StructureDefinition/operationoutcome-issue-line", ValueInteger: 10 + i},
					{URL: "http://hl7.org/fhir/StructureDefinition/operationoutcome-issue-col", ValueInteger: 5},
					{URL: "http://hl7.org/fhir/StructureDefinition/operationoutcome-message-id", ValueCode: fmt.Sprintf("MSG_%03d", (f+i)%120)},
				},
				"severity":   []string{"error", "warning", "information"}[i%3],
				"code":       "invalid",
				"details":    map[string]string{"text": fmt.Sprintf("Observation.component[%d]: value %d is not valid", i, f)},
				"expression": []string{fmt.Sprintf("Observation.component[%d]", i)},
			})
		}
		entries = append(entries, entry{FullURL: "file://" + p, Resource: map[string]any{
			"resourceType": "OperationOutcome",
			"extension":    []ext{{URL: "http://hl7.org/fhir/StructureDefinition/operationoutcome-file", ValueString: p}},
			"issue":        issues,
		}})
		fmt.Fprintf(&sb, "  Validate %s\n", p)
		if f%10 == 0 {
			fmt.Fprintf(&sb, "  Load extra.pkg%d#1.0.0 - 3 resources (00:00.1)\n", f)
		}
		if f%17 == 0 {
			fmt.Fprintf(&sb, "Exception generating snapshot for http://example.org/SD/p%d: base not found\n", f)
		}
		fmt.Fprintf(&sb, " 00:00.%03d\n", f%1000)
	}
	sb.WriteString("Done. Times: Loading: 00:17.024, validation: 00:01.321.\n")
	data, err := json.Marshal(map[string]any{"resourceType": "Bundle", "type": "collection", "entry": entries})
	if err != nil {
		t.Fatal(err)
	}
	return data, sb.String(), paths
}

type parseStage struct {
	name string
	run  func(t testing.TB, out []byte, stdout string, paths []string)
}

var parseStages = []parseStage{
	{"output", func(t testing.TB, out []byte, _ string, paths []string) {
		if _, err := parseOutput(out, paths, ""); err != nil {
			t.Fatal(err)
		}
	}},
	{"file durations", func(_ testing.TB, _ []byte, stdout string, paths []string) { fileDurations(stdout, paths) }},
	{"implicit packages", func(_ testing.TB, _ []byte, stdout string, paths []string) { implicitPackages(stdout, paths) }},
	{"snapshot failures", func(_ testing.TB, _ []byte, stdout string, paths []string) { snapshotFailures(stdout, paths) }},
}

func measureParse(t testing.TB, s parseStage, out []byte, stdout string, paths []string, n int) (time.Duration, uint64) {
	t.Helper()
	var d time.Duration
	var allocs uint64
	for i := range n {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		start := time.Now()
		s.run(t, out, stdout, paths)
		took := time.Since(start)
		runtime.ReadMemStats(&after)
		if i == 0 || took < d {
			d = took
		}
		allocs = after.Mallocs - before.Mallocs
	}
	return d, allocs
}

// TestParsersScaleLinearly: 8× the files must not cost more than 12× the
// allocations or 20× the time; a quadratic parser costs 64×. Same limits as
// internal/scaling.
func TestParsersScaleLinearly(t *testing.T) {
	if raceEnabled {
		t.Skip("the race detector distorts timings; CI runs this without -race")
	}
	const (
		factor   = 8
		maxAlloc = 12.0
		maxTime  = 20.0
		minTime  = 2 * time.Millisecond
	)
	so, ss, sp := syntheticOutput(t, 50, 40)
	lo, ls, lp := syntheticOutput(t, 50*factor, 40)
	for _, s := range parseStages {
		t.Run(s.name, func(t *testing.T) {
			ds, as := measureParse(t, s, so, ss, sp, 3)
			dl, al := measureParse(t, s, lo, ls, lp, 3)
			if as > 0 {
				if r := float64(al) / float64(as); r > maxAlloc {
					t.Errorf("%dx the input made %.1fx the allocations (%d vs %d), want at most %.0fx", factor, r, al, as, maxAlloc)
				}
			}
			if ds < minTime {
				return
			}
			if r := float64(dl) / float64(ds); r > maxTime {
				ds, _ = measureParse(t, s, so, ss, sp, 7)
				dl, _ = measureParse(t, s, lo, ls, lp, 7)
				if r = float64(dl) / float64(ds); r > maxTime {
					t.Errorf("%dx the input took %.1fx as long (%s vs %s), want at most %.0fx", factor, r, dl, ds, maxTime)
				}
			}
		})
	}
}

// BenchmarkParsers: an IG-sized run, 400 files with 40 issues each.
func BenchmarkParsers(b *testing.B) {
	out, stdout, paths := syntheticOutput(b, 400, 40)
	for _, s := range parseStages {
		b.Run(s.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				s.run(b, out, stdout, paths)
			}
		})
	}
}

// pathIndex answers exactly what the linear scan did: the first index whose
// path, cleaned or made absolute, equals the cleaned logged path.
func TestPathIndexMatchesLinearScan(t *testing.T) {
	cwd, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{"a.json", "./dir/../b.json", filepath.Join(cwd, "c.json"), "a.json", "c.json"}
	scan := func(logged string) int {
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
	ix := newPathIndex(paths)
	for _, logged := range []string{
		"a.json", "b.json", "dir/../b.json", filepath.Join(cwd, "a.json"),
		filepath.Join(cwd, "c.json"), "c.json", "missing.json", "",
	} {
		if got, want := ix.lookup(logged), scan(logged); got != want {
			t.Errorf("lookup(%q) = %d, linear scan = %d", logged, got, want)
		}
	}
}
