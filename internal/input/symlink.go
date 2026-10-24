package input

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// Inside reports whether path resolves to a location inside dir, following
// symbolic links in both. A path that cannot be resolved, such as a dangling
// link, is not inside.
//
// A repository can commit a link to a file elsewhere on the machine — a
// credentials file in the home directory, the runner's environment — and a
// validation report quotes what it reads: parse errors name the first token,
// and --show-source prints the line (#500). Whatever fhirlint finds while
// walking a directory or discovers next to the input must therefore really
// live there. A path the user names directly stays trusted, link or not.
func Inside(dir, path string) bool {
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false
	}
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(realDir, realPath)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// LinkLeaves reports whether d, met while walking root, is a symbolic link
// that leads outside root. filepath.WalkDir does not descend into linked
// directories, so only the link entries themselves need resolving.
func LinkLeaves(root, path string, d fs.DirEntry) bool {
	return d.Type()&fs.ModeSymlink != 0 && !Inside(root, path)
}
