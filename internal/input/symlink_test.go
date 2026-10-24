package input

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInside(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "repo")
	outside := filepath.Join(base, "outside")
	for _, d := range []string{filepath.Join(root, "sub"), outside} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p string) {
		if err := os.WriteFile(p, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root, "sub", "a.json"))
	write(filepath.Join(outside, "secret.json"))
	link := func(target, name string) string {
		p := filepath.Join(root, name)
		if err := os.Symlink(target, p); err != nil {
			t.Skipf("symlinks not available: %v", err)
		}
		return p
	}

	cases := []struct {
		name string
		path string
		want bool
	}{
		{"plain file", filepath.Join(root, "sub", "a.json"), true},
		{"link inside", link(filepath.Join("sub", "a.json"), "in.json"), true},
		{"link to a file outside", link(filepath.Join("..", "outside", "secret.json"), "out.json"), false},
		{"absolute link outside", link(filepath.Join(outside, "secret.json"), "abs.json"), false},
		{"link to a directory outside", link(outside, "dir"), false},
		{"file under a linked directory", filepath.Join(root, "dir", "secret.json"), false},
		{"dangling link", link(filepath.Join(base, "missing.json"), "gone.json"), false},
		// A sibling whose name starts with the root's must not count as inside.
		{"sibling with a common prefix", link(filepath.Join(base, "repo-evil"), "prefix"), false},
	}
	if err := os.MkdirAll(filepath.Join(base, "repo-evil"), 0o750); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		if got := Inside(root, c.path); got != c.want {
			t.Errorf("%s: Inside(%s) = %v, want %v", c.name, c.path, got, c.want)
		}
	}
}
