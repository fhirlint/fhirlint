package input

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// ChangedSince returns the absolute paths of files that differ from ref, for
// scoping validation to what a change set actually touched.
//
// dir selects the repository to inspect; an empty dir uses the current working
// directory. Three sets are unioned:
//
//   - committed changes, as ref...HEAD — three-dot range semantics, i.e. changes
//     since the merge base. That is what a pull request against ref contributes,
//     independent of whatever else landed on ref in the meantime.
//   - uncommitted changes against HEAD, staged or not, so a local run sees the
//     file being edited rather than only what was committed.
//   - untracked files that are not ignored, so a newly added resource is not
//     silently skipped.
//
// Deletions are excluded throughout — there is nothing left to validate. A
// renamed file is reported at its new path.
func ChangedSince(ref, dir string) ([]string, error) {
	root, err := gitOutput(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("--since needs a git repository: %w", err)
	}
	if _, err := gitOutput(dir, "rev-parse", "--verify", "--quiet", ref+"^{commit}"); err != nil {
		return nil, fmt.Errorf("--since: cannot resolve git ref %q", ref)
	}

	sets := [][]string{
		{"diff", "--name-only", "--diff-filter=d", "-z", ref + "...HEAD"},
		{"diff", "--name-only", "--diff-filter=d", "-z", "HEAD"},
		{"ls-files", "--others", "--exclude-standard", "-z"},
	}

	seen := make(map[string]bool)
	var paths []string
	for _, args := range sets {
		out, err := gitOutput(dir, args...)
		if err != nil {
			return nil, fmt.Errorf("--since: git %s failed: %w", args[0], err)
		}
		for _, rel := range splitNUL(out) {
			abs := filepath.Join(root, filepath.FromSlash(rel))
			if seen[abs] {
				continue
			}
			seen[abs] = true
			paths = append(paths, abs)
		}
	}
	return paths, nil
}

// splitNUL splits git's -z output, which is NUL-separated and therefore free of
// the path quoting git applies to non-ASCII names in its default output.
func splitNUL(s string) []string {
	var out []string
	for _, p := range strings.Split(s, "\x00") {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...) //nolint:gosec // fixed binary, arguments are not user-controlled beyond the ref
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", errors.New(msg)
		}
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}
