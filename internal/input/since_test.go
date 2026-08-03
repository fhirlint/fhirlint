package input

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newRepo creates a git repository with one commit on main holding the given
// files, and returns its path.
func newRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	run(t, dir, "init", "-b", "main")
	run(t, dir, "config", "user.email", "test@example.com")
	run(t, dir, "config", "user.name", "test")
	for name, content := range files {
		write(t, dir, name, content)
	}
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "initial")
	return dir
}

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...) //nolint:gosec // test helper: fixed binary, arguments come from the test itself
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// names reduces absolute results to repo-relative names so assertions stay
// readable. Both sides are symlink-resolved first: on macOS t.TempDir() hands
// back /var/... while git reports the real /private/var/... path.
func names(t *testing.T, dir string, paths []string) map[string]bool {
	t.Helper()
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]bool, len(paths))
	for _, p := range paths {
		resolved, err := filepath.EvalSymlinks(filepath.Dir(p))
		if err != nil {
			t.Fatalf("resolving %q: %v", p, err)
		}
		rel, err := filepath.Rel(root, filepath.Join(resolved, filepath.Base(p)))
		if err != nil {
			t.Fatalf("result %q is not under the repo: %v", p, err)
		}
		out[filepath.ToSlash(rel)] = true
	}
	return out
}

func TestChangedSince_CommittedChangeOnBranch(t *testing.T) {
	dir := newRepo(t, map[string]string{
		"a.json": `{"resourceType":"Patient"}`,
		"b.json": `{"resourceType":"Observation"}`,
	})
	run(t, dir, "checkout", "-b", "feature")
	write(t, dir, "a.json", `{"resourceType":"Patient","id":"x"}`)
	run(t, dir, "commit", "-am", "touch a")

	got, err := ChangedSince("main", dir)
	if err != nil {
		t.Fatal(err)
	}
	n := names(t, dir, got)
	if !n["a.json"] {
		t.Errorf("changed file a.json missing from %v", n)
	}
	if n["b.json"] {
		t.Errorf("unchanged file b.json should not be reported: %v", n)
	}
}

func TestChangedSince_ExcludesDeleted(t *testing.T) {
	dir := newRepo(t, map[string]string{
		"a.json": `{"resourceType":"Patient"}`,
		"b.json": `{"resourceType":"Observation"}`,
	})
	run(t, dir, "checkout", "-b", "feature")
	run(t, dir, "rm", "b.json")
	run(t, dir, "commit", "-m", "drop b")

	got, err := ChangedSince("main", dir)
	if err != nil {
		t.Fatal(err)
	}
	if n := names(t, dir, got); n["b.json"] {
		t.Errorf("deleted file must not be reported: %v", n)
	}
}

func TestChangedSince_RenameReportsNewPath(t *testing.T) {
	dir := newRepo(t, map[string]string{"old.json": `{"resourceType":"Patient"}`})
	run(t, dir, "checkout", "-b", "feature")
	run(t, dir, "mv", "old.json", "new.json")
	run(t, dir, "commit", "-m", "rename")

	got, err := ChangedSince("main", dir)
	if err != nil {
		t.Fatal(err)
	}
	n := names(t, dir, got)
	if !n["new.json"] {
		t.Errorf("renamed file should be reported at its new path: %v", n)
	}
	if n["old.json"] {
		t.Errorf("old path must not be reported: %v", n)
	}
}

func TestChangedSince_IncludesUncommittedAndUntracked(t *testing.T) {
	dir := newRepo(t, map[string]string{
		"a.json": `{"resourceType":"Patient"}`,
		"b.json": `{"resourceType":"Observation"}`,
	})
	// No new commit: an edit in the working tree and a brand-new file.
	write(t, dir, "b.json", `{"resourceType":"Observation","id":"y"}`)
	write(t, dir, "c.json", `{"resourceType":"Encounter"}`)

	got, err := ChangedSince("main", dir)
	if err != nil {
		t.Fatal(err)
	}
	n := names(t, dir, got)
	if !n["b.json"] {
		t.Errorf("uncommitted edit should be reported: %v", n)
	}
	if !n["c.json"] {
		t.Errorf("untracked file should be reported: %v", n)
	}
	if n["a.json"] {
		t.Errorf("untouched file should not be reported: %v", n)
	}
}

func TestChangedSince_IgnoredFilesStayOut(t *testing.T) {
	dir := newRepo(t, map[string]string{
		"a.json":     `{"resourceType":"Patient"}`,
		".gitignore": "generated/\n",
	})
	write(t, dir, "generated/x.json", `{"resourceType":"Patient"}`)

	got, err := ChangedSince("main", dir)
	if err != nil {
		t.Fatal(err)
	}
	if n := names(t, dir, got); n["generated/x.json"] {
		t.Errorf("gitignored file must not be reported: %v", n)
	}
}

func TestChangedSince_NoChanges(t *testing.T) {
	dir := newRepo(t, map[string]string{"a.json": `{"resourceType":"Patient"}`})

	got, err := ChangedSince("main", dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected no changed files, got %v", got)
	}
}

func TestChangedSince_UnknownRef(t *testing.T) {
	dir := newRepo(t, map[string]string{"a.json": `{"resourceType":"Patient"}`})

	_, err := ChangedSince("no-such-ref", dir)
	if err == nil {
		t.Fatal("expected an error for an unresolvable ref")
	}
	if !strings.Contains(err.Error(), "cannot resolve git ref") {
		t.Errorf("error should name the unresolvable ref, got: %v", err)
	}
}

func TestChangedSince_NotAGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()

	_, err := ChangedSince("main", dir)
	if err == nil {
		t.Fatal("expected an error outside a git repository")
	}
	if !strings.Contains(err.Error(), "needs a git repository") {
		t.Errorf("error should say a repository is required, got: %v", err)
	}
}
