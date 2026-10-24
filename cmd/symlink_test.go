package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/input"
)

// symlinkFixture builds base/repo with one plain resource, a link inside the
// repo, a link to a file outside and a link to a directory outside, plus the
// outside file the links point at. It skips where symlinks are unavailable.
func symlinkFixture(t *testing.T) (repo, target string) {
	t.Helper()
	base := t.TempDir()
	repo = filepath.Join(base, "repo")
	outside := filepath.Join(base, "outside")
	for _, d := range []string{repo, outside} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	target = filepath.Join(outside, "config.json")
	for p, body := range map[string]string{
		filepath.Join(repo, "patient.json"):  `{"resourceType":"Patient"}`,
		target:                               `{"note":"c2VjcmV0"}`,
		filepath.Join(outside, "other.json"): `{}`,
	} {
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for name, dest := range map[string]string{
		"inside.json": "patient.json",
		"leak.json":   target,
		"leakdir":     outside,
	} {
		if err := os.Symlink(dest, filepath.Join(repo, name)); err != nil {
			t.Skipf("symlinks not available: %v", err)
		}
	}
	return repo, target
}

// A link that leads out of the directory is not collected, and the warning
// names the link, not what it points at (#500).
func TestCollectFHIRPaths_SkipsLinksThatLeaveTheDirectory(t *testing.T) {
	repo, target := symlinkFixture(t)

	var paths []string
	stderr := captureStderr(t, func() {
		var err error
		paths, err = collectFHIRPaths(&input.Input{Source: input.SourceDir, Path: repo}, nil)
		if err != nil {
			t.Fatal(err)
		}
	})

	got := map[string]bool{}
	for _, p := range paths {
		got[filepath.Base(p)] = true
	}
	if !got["patient.json"] || !got["inside.json"] {
		t.Errorf("in-directory files missing: %v", paths)
	}
	if got["leak.json"] || got["config.json"] || got["other.json"] {
		t.Errorf("a file outside the directory was collected: %v", paths)
	}
	if !strings.Contains(stderr, filepath.Join(repo, "leak.json")) {
		t.Errorf("warning does not name the link:\n%s", stderr)
	}
	if strings.Contains(stderr, target) {
		t.Errorf("warning names the link's target:\n%s", stderr)
	}
}

func TestDiscoverConfig_RefusesALinkOutOfItsDirectory(t *testing.T) {
	repo, target := symlinkFixture(t)
	t.Chdir(repo)

	if got := discoverConfig(); got != "" {
		t.Fatalf("discoverConfig() = %q with no config present", got)
	}

	if err := os.Symlink(target, "fhirlint.yml"); err != nil {
		t.Fatal(err)
	}
	var got string
	stderr := captureStderr(t, func() { got = discoverConfig() })
	if got != "" {
		t.Errorf("discoverConfig() = %q, want the link refused", got)
	}
	if !strings.Contains(stderr, "fhirlint.yml") || !strings.Contains(stderr, "--config") {
		t.Errorf("warning should name the file and the way out:\n%s", stderr)
	}

	// A plain file, and a link that stays in the directory, are still found.
	if err := os.Remove("fhirlint.yml"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("real.yml", []byte("severity: error\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.yml", "fhirlint.yml"); err != nil {
		t.Fatal(err)
	}
	if got := discoverConfig(); filepath.Base(got) != "fhirlint.yml" {
		t.Errorf("discoverConfig() = %q, want the in-directory link", got)
	}
}

func TestLoadIgnoreFile_RefusesALinkOutOfItsDirectory(t *testing.T) {
	repo, target := symlinkFixture(t)
	link := filepath.Join(repo, ".fhirlintignore")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	patterns, err := loadIgnoreFile(link)
	if err == nil {
		t.Fatalf("loadIgnoreFile accepted a link out of the directory, read %q", patterns)
	}
	if strings.Contains(err.Error(), "c2VjcmV0") {
		t.Errorf("error quotes the target's content: %v", err)
	}
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() { os.Stderr = orig }()

	fn()
	_ = w.Close()
	out, _ := io.ReadAll(r)
	return string(out)
}
