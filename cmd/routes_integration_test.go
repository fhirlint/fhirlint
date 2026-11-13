//go:build integration

package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/fhirlint/fhirlint/pkg/fhirlint"
)

// Equivalent routes give the same findings (#544). fhirlint reaches the
// validator's verdict by many routes, each with its own Go code between the
// JAR and the report. Every fixture goes through every route below, and the
// normalised findings must equal the plain one-shot file run.
//
// Compared per file: severity, message ID, location and message, sorted.
// Expected to differ, and so not compared: durationMs, cached, meta, and the
// label of a stdin run ("stdin" instead of the path).
//
// Not covered here, each for a reason:
//   - --tx-offline replay: needs a recording made against a live terminology
//     server; the fixtures run with --no-terminology-server.
//   - --url: needs an HTTP server serving the file; the input package's tests
//     cover the fetch, and the result goes down the same path as stdin.
//   - LSP: publishes diagnostics, not reports; covered by internal/lsp.
//   - --bundle-entries, --extract, NDJSON: remap locations by design, so the
//     findings are not equal to the whole-file run.
//
// A new route (input kind, server mode) adds itself to routes below.

// routesFixtures is relative to the repository root, where every CLI route
// runs: a path starting with ../ crashes the JAR (#560), which is its own bug
// and would mask what this test compares. Add a ../ route once #560 is fixed.
const routesFixtures = "testdata/routes"

// repoRoot is the parent of cmd/, where this test runs.
const repoRoot = ".."

type routeFinding struct {
	Severity, MessageID, Location, Message string
}

// findings maps a fixture's base name to its sorted findings.
type findings map[string][]routeFinding

func (f findings) add(name string, sev, id, loc, msg string) {
	f[name] = append(f[name], routeFinding{sev, id, loc, msg})
}

func (f findings) sort() {
	for _, list := range f {
		sort.Slice(list, func(i, j int) bool {
			a, b := list[i], list[j]
			return fmt.Sprint(a) < fmt.Sprint(b)
		})
	}
}

// buildFhirlint builds the binary once for the whole test.
func buildFhirlint(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "fhirlint")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, "..") //nolint:gosec // G204: fixed arguments
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// common are the flags every CLI route shares: no terminology server, so the
// result depends on nothing outside the fixture.
var common = []string{"--no-terminology-server", "--fhir-version", "4.0.1", "--format", "json"}

// runJSON runs fhirlint validate and parses its JSON report. Exit code 1
// (findings) is expected; anything else is a failure.
func runJSON(t *testing.T, bin string, stdin []byte, args ...string) findings {
	t.Helper()
	cmd := exec.Command(bin, append([]string{"validate"}, args...)...) //nolint:gosec // G204: the binary just built, fixed flags
	cmd.Dir = repoRoot
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ee, ok := err.(*exec.ExitError); err != nil && (!ok || ee.ExitCode() != 1) {
		t.Fatalf("fhirlint validate %v: %v\nstderr:\n%s", args, err, stderr.String())
	}
	var report struct {
		Files []struct {
			Filename string `json:"filename"`
			Issues   []struct {
				Severity  string `json:"severity"`
				MessageID string `json:"messageId"`
				Location  string `json:"location"`
				Message   string `json:"message"`
			} `json:"issues"`
		} `json:"files"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("fhirlint validate %v: parsing the JSON report: %v\n%s\nstderr:\n%s", args, err, stdout.String(), stderr.String())
	}
	out := findings{}
	for _, f := range report.Files {
		name := filepath.Base(f.Filename)
		if _, ok := out[name]; !ok {
			out[name] = []routeFinding{}
		}
		for _, i := range f.Issues {
			out.add(name, i.Severity, i.MessageID, i.Location, i.Message)
		}
	}
	out.sort()
	return out
}

func fixtureNames(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(repoRoot, routesFixtures))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// startServe runs fhirlint serve and returns its URL once it is ready.
func startServe(t *testing.T, bin string) string {
	t.Helper()
	port, err := freePort()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin, "serve", "--port", fmt.Sprint(port), "--no-terminology-server", "--fhir-version", "4.0.1") //nolint:gosec // G204: the binary just built
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
	})
	ready := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			if line := sc.Text(); strings.Contains(line, "server ready at ") {
				ready <- strings.TrimSpace(line[strings.Index(line, "ready at ")+len("ready at "):])
			}
		}
	}()
	select {
	case url := <-ready:
		return url
	case <-time.After(5 * time.Minute):
		t.Fatal("fhirlint serve did not become ready within 5 minutes")
		return ""
	}
}

func TestIntegration_EquivalentRoutesSameFindings(t *testing.T) {
	bin := buildFhirlint(t)
	names := fixtureNames(t)

	// Reference: each file on its own, one-shot.
	want := findings{}
	for _, n := range names {
		for k, v := range runJSON(t, bin, nil, append(common, filepath.Join(routesFixtures, n))...) {
			want[k] = v
		}
	}
	// The comparison is worthless if the reference found nothing (as in
	// glsec/glsec#1187): every fixture is invalid on purpose.
	for _, n := range names {
		if len(want[n]) == 0 {
			t.Fatalf("reference run found nothing for %s; the comparison would pass on empty input", n)
		}
	}

	cacheDir := t.TempDir()
	routes := []struct {
		name string
		run  func(t *testing.T) findings
	}{
		{"directory, one JVM for all files", func(t *testing.T) findings {
			return runJSON(t, bin, nil, append(common, routesFixtures)...)
		}},
		{"stdin", func(t *testing.T) findings {
			got := findings{}
			for _, n := range names {
				data, err := os.ReadFile(filepath.Join(repoRoot, routesFixtures, n)) //nolint:gosec // G304: fixture
				if err != nil {
					t.Fatal(err)
				}
				for _, v := range runJSON(t, bin, data, common...) {
					got[n] = v // the label is "stdin"; name it after the fixture
				}
			}
			return got
		}},
		{"--cache, cold", func(t *testing.T) findings {
			return runJSON(t, bin, nil, append(common, "--cache", "--cache-dir", cacheDir, routesFixtures)...)
		}},
		{"--cache, warm", func(t *testing.T) findings {
			return runJSON(t, bin, nil, append(common, "--cache", "--cache-dir", cacheDir, routesFixtures)...)
		}},
		{"--server (fhirlint serve)", func(t *testing.T) findings {
			url := startServe(t, bin)
			return runJSON(t, bin, nil, append(common, "--server", url, routesFixtures)...)
		}},
		{"pkg/fhirlint ValidateFile", func(t *testing.T) findings {
			// Absolute, for the same reason as repoRoot (#560).
			wd, err := filepath.Abs(repoRoot)
			if err != nil {
				t.Fatal(err)
			}
			got := findings{}
			for _, n := range names {
				res, err := fhirlint.ValidateFile(filepath.Join(wd, routesFixtures, n), fhirlint.Options{FHIRVersion: "4.0.1", NoTerminologyServer: true})
				if err != nil {
					t.Fatal(err)
				}
				got[n] = []routeFinding{}
				for _, i := range res.Issues {
					got.add(n, i.Severity, i.MessageID, i.Location, i.Message)
				}
			}
			got.sort()
			return got
		}},
	}
	for _, r := range routes {
		t.Run(r.name, func(t *testing.T) {
			got := r.run(t)
			for _, n := range names {
				if fmt.Sprint(got[n]) != fmt.Sprint(want[n]) {
					t.Errorf("%s: findings differ from the one-shot file run\n got: %v\nwant: %v", n, got[n], want[n])
				}
			}
		})
	}
}
