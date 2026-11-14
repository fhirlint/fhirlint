package workflows

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

type workflow struct {
	On   any                       `yaml:"on"`
	Jobs map[string]map[string]any `yaml:"jobs"`
}

func workflows(t *testing.T) map[string]workflow {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(repoRoot(t), ".github", "workflows", "*.yml"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no workflows found: %v", err)
	}
	out := map[string]workflow{}
	for _, p := range paths {
		data, err := os.ReadFile(p) //nolint:gosec // G304: the repository's own workflow files
		if err != nil {
			t.Fatal(err)
		}
		var w workflow
		if err := yaml.Unmarshal(data, &w); err != nil {
			t.Fatalf("%s: %v", filepath.Base(p), err)
		}
		out[filepath.Base(p)] = w
	}
	return out
}

// A job without timeout-minutes runs for GitHub's default of 6 hours when it
// hangs, blocking its concurrency group and every pull request behind it
// (#551). A job that calls a reusable workflow can't set one; the called
// workflow's jobs do.
func TestEveryJobHasATimeout(t *testing.T) {
	for file, w := range workflows(t) {
		for name, job := range w.Jobs {
			if _, ok := job["uses"]; ok {
				continue
			}
			v, ok := job["timeout-minutes"]
			if !ok {
				t.Errorf("%s: job %q has no timeout-minutes; set one from its measured duration plus margin, with the basis in a comment", file, name)
				continue
			}
			if n, ok := v.(int); ok && (n <= 0 || n > 120) {
				t.Errorf("%s: job %q has timeout-minutes %d; expected 1..120", file, name, n)
			}
		}
	}
}

// onlyCalled reports whether a workflow runs only as a reusable workflow.
func onlyCalled(on any) bool {
	m, ok := on.(map[string]any)
	if !ok {
		return false
	}
	_, called := m["workflow_call"]
	return called && len(m) == 1
}

// Every workflow that runs on its own has a budget for the duration watch,
// and every budget names a workflow that exists.
func TestWorkflowBudgets(t *testing.T) {
	f, err := os.Open(filepath.Join(repoRoot(t), ".github", "workflow-budgets.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	budgets := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		if len(fields) < 2 {
			t.Errorf("budget line %q: want <file> <minutes>", sc.Text())
			continue
		}
		if budgets[fields[0]] {
			t.Errorf("budget for %s listed twice", fields[0])
		}
		budgets[fields[0]] = true
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}

	all := workflows(t)
	var missing, stale []string
	for file, w := range all {
		if !onlyCalled(w.On) && !budgets[file] {
			missing = append(missing, file)
		}
	}
	for file := range budgets {
		if _, ok := all[file]; !ok {
			stale = append(stale, file)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) > 0 {
		t.Errorf("no budget in .github/workflow-budgets.txt for %v", missing)
	}
	if len(stale) > 0 {
		t.Errorf(".github/workflow-budgets.txt has budgets for workflows that don't exist: %v", stale)
	}
}
