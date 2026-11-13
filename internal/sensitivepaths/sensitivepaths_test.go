package sensitivepaths

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repoRoot is two levels up from this test file (internal/sensitivepaths).
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// runScript runs .github/scripts/sensitive-paths.sh with args and stdin, from
// the repo root, with dir prepended to PATH, and returns its output and exit code.
func runScript(t *testing.T, dir, stdin string, args ...string) (string, int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the script runs on the Linux runner")
	}
	for _, tool := range []string{"bash", "jq", "sha256sum"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not installed", tool)
		}
	}
	cmd := exec.Command("bash", append([]string{".github/scripts/sensitive-paths.sh"}, args...)...) //nolint:gosec // G204: fixed script, arguments from the test table
	cmd.Dir = repoRoot(t)
	cmd.Stdin = strings.NewReader(stdin)
	if dir != "" {
		cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	out, err := cmd.CombinedOutput()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return string(out), code
}

type compareFile struct {
	Filename         string `json:"filename"`
	PreviousFilename string `json:"previous_filename,omitempty"`
	Patch            string `json:"patch,omitempty"`
}

// compareJSON is a compare API response with the given files.
func compareJSON(t *testing.T, files ...compareFile) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{"files": files})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The filter must match the list the workflow enforced before #541 moved it
// into the script, plus .github/scripts/ itself.
func TestFilter(t *testing.T) {
	sensitive := []string{
		// plugin
		"hooks/validate-fhir.sh",
		"skills/fhirlint/SKILL.md",
		".claude-plugin/plugin.json",
		// pipelines
		"templates/fhirlint.yml",
		"action.yml",
		"action.yaml",
		".gitlab-ci.yml",
		".pre-commit-hooks.yaml",
		// build, release
		".github/workflows/test.yml",
		".github/actions/setup/action.yml",
		".github/scripts/sensitive-paths.sh",
		".goreleaser.yml",
		".goreleaser.yaml",
		"Dockerfile",
		// verification
		"internal/validator/hl7_signing_key.asc",
		"internal/validator/signature.go",
		// agent config
		"CLAUDE.md",
		"sub/CLAUDE.md",
		"CLAUDE.local.md",
		"AGENTS.md",
		"AGENT.md",
		"GEMINI.md",
		".claude/settings.json",
		".agents/x.md",
		".cursor/rules",
		".vscode/settings.json",
		".devcontainer/devcontainer.json",
		".mcp.json",
		".envrc",
		".cursorrules",
		".claudeignore",
		".github/copilot-instructions.md",
		".github/instructions/go.md",
		".github/agents/a.md",
		".github/skills/s.md",
		// other checks
		".gitignore",
		".golangci.yml",
		".github/dependabot.yml",
		".github/actionlint.yaml",
		"CODEOWNERS",
		".github/CODEOWNERS",
		"docs/CODEOWNERS",
	}
	other := []string{
		"README.md",
		"cmd/validate.go",
		"internal/validator/run.go",
		"internal/validator/signature_test.go",
		"docs/hooks.md",
		"fhirlint.yml.example",
		".github/pull_request_template.md",
		"testdata/templates/x.json",
		"sub/action.yml",
	}
	in := strings.Join(append(append([]string{}, other...), sensitive...), "\n") + "\n"
	got, code := runScript(t, "", in, "filter")
	if code != 0 {
		t.Fatalf("filter exit %d: %s", code, got)
	}
	gotLines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	gotSet := map[string]bool{}
	for _, l := range gotLines {
		gotSet[l] = true
	}
	for _, f := range sensitive {
		if !gotSet[f] {
			t.Errorf("filter dropped sensitive path %q", f)
		}
	}
	for _, f := range other {
		if gotSet[f] {
			t.Errorf("filter matched non-sensitive path %q", f)
		}
	}
	if len(gotLines) != len(sensitive) {
		t.Errorf("filter printed %d lines, want %d:\n%s", len(gotLines), len(sensitive), got)
	}
	if got, code := runScript(t, "", "README.md\ncmd/validate.go\n", "filter"); code != 0 || got != "" {
		t.Errorf("filter with no sensitive path = %q (exit %d), want empty, exit 0", got, code)
	}
}

// fakeGH writes a gh that answers `gh api repos/R/compare/B...<sha>` with the
// response stored for that sha, and fails for any other sha.
func fakeGH(t *testing.T, responses map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/usr/bin/env bash\nsha=${2##*...}\nf=\"" + dir + "/$sha.json\"\n[ -f \"$f\" ] || { echo \"not found\" >&2; exit 1; }\ncat \"$f\"\n"
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o700); err != nil { //nolint:gosec // G306: an executable test helper
		t.Fatal(err)
	}
	for sha, body := range responses {
		if err := os.WriteFile(filepath.Join(dir, sha+".json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// The label stays only when the pull request's diff on the sensitive paths is
// the same before and after the push (#541); every doubt removes it.
func TestUnchanged(t *testing.T) {
	wf := compareFile{Filename: ".github/workflows/test.yml", Patch: "@@ -1 +1 @@\n-a\n+b"}
	code := compareFile{Filename: "cmd/validate.go", Patch: "@@ -1 +1 @@\n-x\n+y"}
	tests := []struct {
		name          string
		before, after string
		want          int
	}{
		{"unchanged", compareJSON(t, wf, code), compareJSON(t, wf, code), 0},
		{"only other files changed", compareJSON(t, wf, code),
			compareJSON(t, wf, compareFile{Filename: "cmd/validate.go", Patch: "@@ -1 +1 @@\n-x\n+z"}, compareFile{Filename: "README.md", Patch: "+r"}), 0},
		{"no sensitive files at all", compareJSON(t, code), compareJSON(t, code, compareFile{Filename: "docs/x.md", Patch: "+d"}), 0},
		{"sensitive file changed", compareJSON(t, wf),
			compareJSON(t, compareFile{Filename: wf.Filename, Patch: "@@ -1 +1 @@\n-a\n+c"}), 1},
		{"sensitive file added", compareJSON(t, wf),
			compareJSON(t, wf, compareFile{Filename: "hooks/hooks.json", Patch: "+{}"}), 1},
		{"sensitive file added to a clean pull request", compareJSON(t, code),
			compareJSON(t, code, compareFile{Filename: "templates/fhirlint.yml", Patch: "+x"}), 1},
		{"sensitive change reverted", compareJSON(t, wf, code), compareJSON(t, code), 1},
		{"renamed out of a sensitive path", compareJSON(t, wf),
			compareJSON(t, wf, compareFile{Filename: "docs/hooks.json", PreviousFilename: "hooks/hooks.json", Patch: ""}), 1},
		{"sensitive file without a patch", compareJSON(t, wf, compareFile{Filename: "testdata/x.png"}),
			compareJSON(t, wf, compareFile{Filename: "internal/validator/hl7_signing_key.asc"}), 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := fakeGH(t, map[string]string{"before": tt.before, "after": tt.after})
			out, code := runScript(t, dir, "", "unchanged", "o/r", "main", "before", "after")
			if code != tt.want {
				t.Errorf("exit %d, want %d; output:\n%s", code, tt.want, out)
			}
		})
	}

	t.Run("compare fails", func(t *testing.T) {
		dir := fakeGH(t, map[string]string{"after": compareJSON(t, wf)})
		if out, code := runScript(t, dir, "", "unchanged", "o/r", "main", "gone", "after"); code == 0 {
			t.Errorf("exit 0 for a before commit the API cannot compare; output:\n%s", out)
		}
	})
	t.Run("at the 300-file limit", func(t *testing.T) {
		many := make([]compareFile, 299, 300)
		for i := range many {
			many[i] = compareFile{Filename: fmt.Sprintf("cmd/f%d.go", i), Patch: "+"}
		}
		body := compareJSON(t, append(many, wf)...)
		dir := fakeGH(t, map[string]string{"before": body, "after": body})
		if out, code := runScript(t, dir, "", "unchanged", "o/r", "main", "before", "after"); code == 0 {
			t.Errorf("exit 0 for a compare at the API's file limit; output:\n%s", out)
		}
	})
}
