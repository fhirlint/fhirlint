// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package workflows

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// fakeGH is a gh that answers from $FAKE_GH_DIR, as the real gh prints after
// --jq:
//
//   - `gh run list --workflow W` prints the file W: "id wall-seconds" per run.
//     A file W.fail holds how many calls fail first, with gh's exit code 1
//     and its error line.
//   - `gh api repos/R/actions/runs/ID/jobs?…` prints the file jobs-ID: the
//     run time in seconds. Each such call is logged to the file api.log.
const fakeGH = `#!/usr/bin/env bash
if [ "$1" = api ]; then
  id=${2#*/runs/}; id=${id%%/*}
  echo "$id" >> "$FAKE_GH_DIR/api.log"
  cat "$FAKE_GH_DIR/jobs-$id"
  exit
fi
wf=""
while [ $# -gt 0 ]; do
  if [ "$1" = --workflow ]; then wf=$2; fi
  shift
done
fail="$FAKE_GH_DIR/$wf.fail"
if [ -f "$fail" ] && [ "$(cat "$fail")" -gt 0 ]; then
  echo $(( $(cat "$fail") - 1 )) > "$fail"
  echo "failed to get runs: HTTP 502: Server Error" >&2
  exit 1
fi
cat "$FAKE_GH_DIR/$wf" 2>/dev/null || true
`

// watch is one run of the script against the fake gh.
type watch struct {
	runs  map[string]string // workflow → "id wall" lines
	jobs  map[string]int    // run id → run time
	fails map[string]int    // workflow → failing calls first
}

type watched struct {
	out, stderr string
	code        int
	measured    []string // the run ids whose jobs were queried
}

// runDurationWatch runs .github/scripts/duration-watch.sh against budgets,
// with the fake gh answering from w.
func runDurationWatch(t *testing.T, budgets string, w watch) watched {
	t.Helper()
	// The script runs on the ubuntu runner only and uses GNU date -d, which
	// the BSD date on macOS does not have.
	if runtime.GOOS != "linux" {
		t.Skip("the script runs on the Linux runner")
	}
	for _, tool := range []string{"bash", "awk", "date"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not installed", tool)
		}
	}
	bin, data := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(fakeGH), 0o755); err != nil { //nolint:gosec // G306: the fake gh must be executable
		t.Fatal(err)
	}
	for wf, d := range w.runs {
		if err := os.WriteFile(filepath.Join(data, wf), []byte(d), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for id, sec := range w.jobs {
		if err := os.WriteFile(filepath.Join(data, "jobs-"+id), []byte(strconv.Itoa(sec)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for wf, n := range w.fails {
		if err := os.WriteFile(filepath.Join(data, wf+".fail"), []byte(strconv.Itoa(n)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	budgetFile := filepath.Join(data, "budgets.txt")
	if err := os.WriteFile(budgetFile, []byte(budgets), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("bash", ".github/scripts/duration-watch.sh", "owner/repo", budgetFile, "7") //nolint:gosec // G204: fixed script, arguments from the test
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_GH_DIR="+data,
		"DURATION_WATCH_RETRY_DELAY=0",
	)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	res := watched{out: stdout.String(), stderr: stderr.String(), code: code}
	if log, err := os.ReadFile(filepath.Join(data, "api.log")); err == nil { //nolint:gosec // G304: a file in the test's own temp dir
		res.measured = strings.Fields(string(log))
	}
	return res
}

const testBudgets = `# file  minutes
a.yml   5
b.yml   5
c.yml   5
`

// The exit codes the workflow tells apart: 0 within budget, 3 over budget,
// anything else a failed measurement (#736).
func TestDurationWatch_WithinBudget(t *testing.T) {
	r := runDurationWatch(t, testBudgets, watch{runs: map[string]string{
		"a.yml": "1 60\n", "b.yml": "2 120\n", "c.yml": "3 30\n",
	}})
	if r.code != 0 {
		t.Fatalf("exit %d, want 0:\n%s", r.code, r.out)
	}
	if !strings.Contains(r.out, "| `b.yml` | 1 | 2m00s | 2m00s | ≤ 2m00s | 5m | ok |") {
		t.Errorf("no within-budget row for b.yml:\n%s", r.out)
	}
	// Within the budget on wall time, nothing needs a jobs query.
	if len(r.measured) != 0 {
		t.Errorf("queried the jobs of %v", r.measured)
	}
}

// #721: a run that waited for a runner is over the budget on wall time only.
func TestDurationWatch_QueueIsNotRunTime(t *testing.T) {
	r := runDurationWatch(t, testBudgets, watch{
		runs: map[string]string{"a.yml": "1 60\n", "b.yml": "2 60\n3 722\n", "c.yml": "4 30\n"},
		jobs: map[string]int{"3": 5}, // Sensitive paths on 2026-11-16: 717 s queued, 5 s ran
	})
	if r.code != 0 {
		t.Fatalf("exit %d, want 0:\n%s", r.code, r.out)
	}
	if !strings.Contains(r.out, "| `b.yml` | 2 | 1m00s | 12m02s | ≤ 1m00s | 5m | ok |") {
		t.Errorf("wrong row for b.yml:\n%s", r.out)
	}
	if strings.Join(r.measured, ",") != "3" {
		t.Errorf("queried the jobs of %v, want only run 3", r.measured)
	}
}

func TestDurationWatch_OverBudget(t *testing.T) {
	r := runDurationWatch(t, testBudgets, watch{
		runs: map[string]string{"a.yml": "1 60\n", "b.yml": "2 900\n", "c.yml": "3 30\n"},
		jobs: map[string]int{"2": 400},
	})
	if r.code != 3 {
		t.Fatalf("exit %d, want 3:\n%s", r.code, r.out)
	}
	if !strings.Contains(r.out, "| `b.yml` | 1 | 15m00s | 15m00s | 6m40s | 5m | **over budget** |") {
		t.Errorf("no over-budget row for b.yml:\n%s", r.out)
	}
	// The rows after the overrun are still measured.
	if !strings.Contains(r.out, "`c.yml`") {
		t.Errorf("c.yml missing:\n%s", r.out)
	}
}

// #736: gh failed on the third workflow, the script stopped with gh's exit
// code 1, and the workflow filed the two-row table as an overrun (#721).
func TestDurationWatch_FailedQueryIsNotAnOverrun(t *testing.T) {
	r := runDurationWatch(t, testBudgets, watch{
		runs:  map[string]string{"a.yml": "1 60\n", "b.yml": "2 60\n", "c.yml": "3 60\n"},
		fails: map[string]int{"b.yml": 3},
	})
	if r.code == 0 || r.code == 3 {
		t.Fatalf("exit %d, want a failure that is neither 0 nor 3:\n%s", r.code, r.out)
	}
	if strings.Contains(r.out, "`c.yml`") {
		t.Errorf("the script went on after the failure:\n%s", r.out)
	}
	if !strings.Contains(r.stderr, "giving up") {
		t.Errorf("stderr does not say it gave up:\n%s", r.stderr)
	}
}

func TestDurationWatch_RetriesATransientFailure(t *testing.T) {
	r := runDurationWatch(t, testBudgets, watch{
		runs:  map[string]string{"a.yml": "1 60\n", "b.yml": "2 60\n", "c.yml": "3 60\n"},
		fails: map[string]int{"b.yml": 2},
	})
	if r.code != 0 {
		t.Fatalf("exit %d, want 0 after the retries:\n%s\n%s", r.code, r.out, r.stderr)
	}
	if strings.Count(r.out, "| ok |") != 3 {
		t.Errorf("want three ok rows:\n%s", r.out)
	}
	if strings.Count(r.stderr, "retrying") != 2 {
		t.Errorf("want two retries:\n%s", r.stderr)
	}
}

func TestDurationWatch_NoRuns(t *testing.T) {
	r := runDurationWatch(t, testBudgets, watch{runs: map[string]string{"a.yml": "1 60\n", "c.yml": "2 60\n"}})
	if r.code != 0 {
		t.Fatalf("exit %d, want 0:\n%s", r.code, r.out)
	}
	if !strings.Contains(r.out, "| `b.yml` | 0 | | | | 5m | no runs |") {
		t.Errorf("no 'no runs' row:\n%s", r.out)
	}
}
