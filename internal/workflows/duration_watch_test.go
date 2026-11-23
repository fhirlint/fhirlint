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

// fakeGH is a gh that answers `gh run list` for the workflow named in
// --workflow from $FAKE_GH_DIR/<workflow>: the durations in seconds, one per
// line, as the real gh prints them after --jq. A file <workflow>.fail holds how
// many calls fail first, with gh's exit code 1 and its error line.
const fakeGH = `#!/usr/bin/env bash
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

// runDurationWatch runs .github/scripts/duration-watch.sh against budgets,
// with the fake gh answering from runs (workflow → durations) and failing
// each workflow in fails as often as given. It returns stdout, stderr and the
// exit code.
func runDurationWatch(t *testing.T, budgets string, runs map[string]string, fails map[string]int) (string, string, int) {
	t.Helper()
	if runtime.GOOS == "windows" {
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
	for wf, d := range runs {
		if err := os.WriteFile(filepath.Join(data, wf), []byte(d), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for wf, n := range fails {
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
	return stdout.String(), stderr.String(), code
}

const testBudgets = `# file  minutes
a.yml   5
b.yml   5
c.yml   5
`

// The exit codes the workflow tells apart: 0 within budget, 3 over budget,
// anything else a failed measurement (#736).
func TestDurationWatch_WithinBudget(t *testing.T) {
	out, _, code := runDurationWatch(t, testBudgets,
		map[string]string{"a.yml": "60\n", "b.yml": "120\n", "c.yml": "30\n"}, nil)
	if code != 0 {
		t.Fatalf("exit %d, want 0:\n%s", code, out)
	}
	if strings.Count(out, "| ok |") != 3 {
		t.Errorf("want three ok rows:\n%s", out)
	}
}

func TestDurationWatch_OverBudget(t *testing.T) {
	out, _, code := runDurationWatch(t, testBudgets,
		map[string]string{"a.yml": "60\n", "b.yml": "400\n", "c.yml": "30\n"}, nil)
	if code != 3 {
		t.Fatalf("exit %d, want 3:\n%s", code, out)
	}
	if !strings.Contains(out, "| `b.yml` | 1 | 6m40s | 6m40s | 5m | **over budget** |") {
		t.Errorf("no over-budget row for b.yml:\n%s", out)
	}
	// The rows after the overrun are still measured.
	if !strings.Contains(out, "`c.yml`") {
		t.Errorf("c.yml missing:\n%s", out)
	}
}

// #721: gh failed on the third workflow, the script stopped with gh's exit
// code 1, and the workflow filed the two-row table as an overrun.
func TestDurationWatch_FailedQueryIsNotAnOverrun(t *testing.T) {
	out, stderr, code := runDurationWatch(t, testBudgets,
		map[string]string{"a.yml": "60\n", "b.yml": "60\n", "c.yml": "60\n"}, map[string]int{"b.yml": 3})
	if code == 0 || code == 3 {
		t.Fatalf("exit %d, want a failure that is neither 0 nor 3:\n%s", code, out)
	}
	if strings.Contains(out, "`c.yml`") {
		t.Errorf("the script went on after the failure:\n%s", out)
	}
	if !strings.Contains(stderr, "giving up") {
		t.Errorf("stderr does not say it gave up:\n%s", stderr)
	}
}

func TestDurationWatch_RetriesATransientFailure(t *testing.T) {
	out, stderr, code := runDurationWatch(t, testBudgets,
		map[string]string{"a.yml": "60\n", "b.yml": "60\n", "c.yml": "60\n"}, map[string]int{"b.yml": 2})
	if code != 0 {
		t.Fatalf("exit %d, want 0 after the retries:\n%s\n%s", code, out, stderr)
	}
	if strings.Count(out, "| ok |") != 3 {
		t.Errorf("want three ok rows:\n%s", out)
	}
	if strings.Count(stderr, "retrying") != 2 {
		t.Errorf("want two retries:\n%s", stderr)
	}
}

func TestDurationWatch_NoRuns(t *testing.T) {
	out, _, code := runDurationWatch(t, testBudgets, map[string]string{"a.yml": "60\n", "c.yml": "60\n"}, nil)
	if code != 0 {
		t.Fatalf("exit %d, want 0:\n%s", code, out)
	}
	if !strings.Contains(out, "| `b.yml` | 0 | | | 5m | no runs |") {
		t.Errorf("no 'no runs' row:\n%s", out)
	}
}
