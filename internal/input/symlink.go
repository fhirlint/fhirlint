package input

import (
	"fmt"
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
	realDir, err := resolve(dir)
	if err != nil {
		return false
	}
	realPath, err := resolve(path)
	if err != nil {
		return false
	}
	return contains(realDir, realPath)
}

// LinkLeaves reports whether d, met while walking root, is a symbolic link
// that leads outside root. filepath.WalkDir does not descend into linked
// directories, so only the link entries themselves need resolving.
func LinkLeaves(root, path string, d fs.DirEntry) bool {
	return d.Type()&fs.ModeSymlink != 0 && !Inside(root, path)
}

// Within reports whether path lies inside dir once symbolic links are
// resolved, like Inside, but for a path that may not exist yet: it resolves
// the deepest ancestor that does and appends the rest. That is what a
// directory fhirlint is about to create needs, such as the result cache
// (#498).
func Within(dir, path string) bool {
	realDir, err := resolve(dir)
	if err != nil {
		return false
	}
	realPath, err := resolveExisting(path)
	if err != nil {
		return false
	}
	return contains(realDir, realPath)
}

// resolveExisting resolves symbolic links in the longest existing prefix of
// path and joins the missing remainder onto it.
func resolveExisting(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	var rest []string
	for p := abs; ; p = filepath.Dir(p) {
		if real, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(append([]string{real}, rest...)...), nil
		}
		if filepath.Dir(p) == p {
			return "", fmt.Errorf("no existing ancestor of %s", path)
		}
		rest = append([]string{filepath.Base(p)}, rest...)
	}
}

// contains reports whether the resolved path lies in or at the resolved dir.
func contains(realDir, realPath string) bool {
	rel, err := filepath.Rel(realDir, realPath)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// resolve makes path absolute before resolving its links. EvalSymlinks keeps
// a relative path relative, and filepath.Rel cannot compare "." with an
// absolute link target.
func resolve(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}
