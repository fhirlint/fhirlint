// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package input

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// The corners of the symlink checks (#691): anything that can't be resolved
// counts as outside, so a path is only trusted when it is shown to be inside.
func TestInsideAndWithin_Unresolvable(t *testing.T) {
	dir := t.TempDir()
	gone := filepath.Join(dir, "gone")
	if Inside(gone, dir) {
		t.Error("Inside: a directory that doesn't exist contains nothing")
	}
	if Inside(dir, gone) {
		t.Error("Inside: a path that doesn't exist is not shown to be inside")
	}
	if Within(gone, filepath.Join(dir, "x")) {
		t.Error("Within: a directory that doesn't exist contains nothing")
	}
	// Within accepts a path that doesn't exist yet when its ancestor is inside…
	if !Within(dir, filepath.Join(dir, "a", "b", "cache")) {
		t.Error("Within: a not-yet-created path below dir is inside")
	}
	// …but not one whose existing ancestor is elsewhere.
	if Within(dir, filepath.Join(t.TempDir(), "a", "cache")) {
		t.Error("Within: a not-yet-created path below another directory is outside")
	}
}

type linkEntry struct{ fs.DirEntry }

func (linkEntry) Type() fs.FileMode { return fs.ModeSymlink }

type fileEntry struct{ fs.DirEntry }

func (fileEntry) Type() fs.FileMode { return 0 }

// Only a link entry that leads out of the root is reported; a plain file
// entry never is, whatever its path.
func TestLinkLeaves(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	root, outside := t.TempDir(), t.TempDir()
	in := filepath.Join(root, "in.json")
	if err := os.WriteFile(in, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "out.json")
	target := filepath.Join(outside, "secret.json")
	if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, out); err != nil {
		t.Fatal(err)
	}
	sibling := filepath.Join(root, "sibling.json")
	if err := os.Symlink(in, sibling); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, e := range entries {
		got[e.Name()] = LinkLeaves(root, filepath.Join(root, e.Name()), e)
	}
	if got["in.json"] || got["sibling.json"] || !got["out.json"] {
		t.Errorf("LinkLeaves = %v; only out.json leads outside", got)
	}
	// A link entry that can't be resolved leads nowhere safe.
	if !LinkLeaves(root, filepath.Join(root, "dangling"), linkEntry{}) {
		t.Error("a dangling link must count as leaving the root")
	}
	// A plain entry isn't resolved at all: WalkDir found it under the root.
	if LinkLeaves(root, filepath.Join(root, "vanished.json"), fileEntry{}) {
		t.Error("a plain file entry never leaves the root")
	}
}
