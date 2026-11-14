// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package scaling

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/fhirlint/fhirlint/internal/baseline"
	"github.com/fhirlint/fhirlint/internal/pkgindex"
	"github.com/fhirlint/fhirlint/internal/reporter"
	"github.com/fhirlint/fhirlint/internal/suppress"
	"github.com/fhirlint/fhirlint/internal/validator"
)

// synthetic builds results for files × perFile issues, shaped like a real IG
// run: a spread of message IDs and severities, locations with line and column,
// some issues that ask for a package hint, and a duration per file.
func synthetic(files, perFile int) []*validator.Result {
	sev := []string{"error", "warning", "information", "fatal"}
	out := make([]*validator.Result, files)
	for f := range files {
		r := &validator.Result{
			Filename:   fmt.Sprintf("fhir/resources/Observation-%05d.json", f),
			Label:      fmt.Sprintf("fhir/resources/Observation-%05d.json", f),
			DurationMs: int64(100 + f%997),
		}
		for i := range perFile {
			id := fmt.Sprintf("MSG_%03d", (f*7+i)%120)
			msg := fmt.Sprintf("Observation.component[%d].code: value %d not in value set", i, f)
			if i%25 == 0 {
				id = "Validation_VAL_Profile_Unknown"
				msg = fmt.Sprintf("Profile reference 'http://example.org/StructureDefinition/p%d' has not been checked because it could not be found", i%40)
			}
			r.Issues = append(r.Issues, validator.Issue{
				Severity:  sev[(f+i)%len(sev)],
				MessageID: id,
				Location:  fmt.Sprintf("Observation.component[%d].code (line %d, col %d)", i, 10+i*3, 5+i%9),
				Message:   msg,
			})
		}
		r.Valid = false
		out[f] = r
	}
	return out
}

// clone deep-copies results, since most stages change them in place.
func clone(results []*validator.Result) []*validator.Result {
	out := make([]*validator.Result, len(results))
	for i, r := range results {
		c := *r
		c.Issues = append([]validator.Issue(nil), r.Issues...)
		c.Suppressed = append([]validator.Issue(nil), r.Suppressed...)
		out[i] = &c
	}
	return out
}

func rules(t testing.TB) []suppress.Rule {
	t.Helper()
	var rs []suppress.Rule
	for _, s := range []string{
		"messageId:MSG_001", "messageId:MSG_050", "messageId:MSG_099",
		"constraint:dom-6", "expression:Observation.component",
		"pattern:not in value set$", "pattern:^Profile reference",
	} {
		r, err := suppress.ParseCLI(s)
		if err != nil {
			t.Fatal(err)
		}
		rs = append(rs, r)
	}
	return rs
}

type stage struct {
	name string
	run  func(t testing.TB, results []*validator.Result)
}

func stages() []stage {
	writeTo := func(write func(dest string) error) func(t testing.TB, _ []*validator.Result) {
		return func(t testing.TB, _ []*validator.Result) {
			t.Helper()
			if err := write(filepath.Join(t.TempDir(), "report")); err != nil {
				t.Fatal(err)
			}
		}
	}
	report := func(f func(rs []*validator.Result, dest string) error) func(t testing.TB, rs []*validator.Result) {
		return func(t testing.TB, rs []*validator.Result) {
			t.Helper()
			writeTo(func(d string) error { return f(rs, d) })(t, rs)
		}
	}
	info := reporter.RunInfo{Fhirlint: "1.0.0", Validator: "6.10.4", FHIRVersion: "4.0.1"}
	return []stage{
		{"suppress", func(t testing.TB, rs []*validator.Result) {
			suppress.ApplyAt(rs, rules(t), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
		}},
		{"baseline generate", func(_ testing.TB, rs []*validator.Result) { baseline.Generate(rs) }},
		{"baseline apply", func(_ testing.TB, rs []*validator.Result) {
			bf := baseline.Generate(rs[:len(rs)/2])
			baseline.ApplyReport(rs, bf)
		}},
		{"package hints", func(_ testing.TB, rs []*validator.Result) { pkgindex.Annotate(rs) }},
		{"--group", func(t testing.TB, rs []*validator.Result) {
			withStdout(t, io.Discard, func() { reporter.TerminalGrouped(rs, "information", false, false) })
		}},
		{"--slowest", func(_ testing.TB, rs []*validator.Result) { reporter.Slowest(io.Discard, rs, 20) }},
		{"json", report(func(rs []*validator.Result, d string) error { return reporter.JSON(rs, "information", info, d) })},
		{"sarif", report(func(rs []*validator.Result, d string) error { return reporter.SARIF(rs, "information", info, d) })},
		{"codeclimate", report(func(rs []*validator.Result, d string) error { return reporter.CodeClimate(rs, "information", d) })},
		{"junit", report(func(rs []*validator.Result, d string) error { return reporter.JUnit(rs, "information", info, d) })},
		{"html", report(func(rs []*validator.Result, d string) error { return reporter.HTML(rs, "information", info, d) })},
		{"markdown", report(func(rs []*validator.Result, d string) error { return reporter.Markdown(rs, "information", info, d) })},
	}
}

// withStdout runs f with os.Stdout sent to w, for the reporters that print.
func withStdout(t testing.TB, w io.Writer, f func()) {
	t.Helper()
	r, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = pw
	done := make(chan struct{})
	go func() { _, _ = io.Copy(w, r); close(done) }()
	defer func() {
		os.Stdout = old
		_ = pw.Close()
		<-done
		_ = r.Close()
	}()
	f()
}

// measure runs one stage on a fresh copy and returns its time and the number
// of heap allocations it made.
func measure(t testing.TB, s stage, input []*validator.Result) (time.Duration, uint64) {
	t.Helper()
	rs := clone(input)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	s.run(t, rs)
	d := time.Since(start)
	runtime.ReadMemStats(&after)
	return d, after.Mallocs - before.Mallocs
}

// best is the fastest of n runs; allocations don't vary between runs.
func best(t testing.TB, s stage, input []*validator.Result, n int) (time.Duration, uint64) {
	t.Helper()
	d, a := measure(t, s, input)
	for range n - 1 {
		if d2, _ := measure(t, s, input); d2 < d {
			d = d2
		}
	}
	return d, a
}

// TestPipelineScalesLinearly runs every stage on 8× the input and fails when
// its allocations grow more than 12× or its time more than 20× (#552). A
// quadratic step grows 64×; the margins absorb maps and slices that grow in
// steps, and timer noise. Time is only judged above minTime, as in
// glsec/glsec#868: below that, a scheduler hiccup is the whole measurement.
func TestPipelineScalesLinearly(t *testing.T) {
	if raceEnabled {
		t.Skip("the race detector distorts timings; CI runs this without -race")
	}
	t.Setenv("HOME", t.TempDir()) // an empty package cache for the hints
	t.Setenv("USERPROFILE", t.TempDir())
	reporter.DisableColors()

	const (
		factor   = 8
		maxAlloc = 12.0
		maxTime  = 20.0
		minTime  = 2 * time.Millisecond
	)
	cases := []struct {
		name         string
		small, large []*validator.Result
	}{
		// More files, and more issues in one file: a step can be quadratic in either.
		{"files", synthetic(50, 40), synthetic(50*factor, 40)},
		{"issues per file", synthetic(4, 250), synthetic(4, 250*factor)},
	}
	for _, c := range cases {
		for _, s := range stages() {
			t.Run(c.name+"/"+s.name, func(t *testing.T) {
				ds, as := best(t, s, c.small, 3)
				dl, al := best(t, s, c.large, 3)
				if as > 0 {
					if r := float64(al) / float64(as); r > maxAlloc {
						t.Errorf("%dx the input made %.1fx the allocations (%d vs %d), want at most %.0fx", factor, r, al, as, maxAlloc)
					}
				}
				if ds < minTime {
					return
				}
				if r := float64(dl) / float64(ds); r > maxTime {
					// Once more, slower, before blaming the code.
					ds, _ = best(t, s, c.small, 7)
					dl, _ = best(t, s, c.large, 7)
					if r = float64(dl) / float64(ds); r > maxTime {
						t.Errorf("%dx the input took %.1fx as long (%s vs %s), want at most %.0fx", factor, r, dl, ds, maxTime)
					}
				}
			})
		}
	}
}

// BenchmarkPipeline benchmarks each stage on an IG-sized run: 400 files with
// 40 issues each.
func BenchmarkPipeline(b *testing.B) {
	b.Setenv("HOME", b.TempDir())
	reporter.DisableColors()
	input := synthetic(400, 40)
	for _, s := range stages() {
		b.Run(s.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				b.StopTimer()
				rs := clone(input)
				b.StartTimer()
				s.run(b, rs)
			}
		})
	}
}
