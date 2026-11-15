// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package input

import (
	"path"
	"path/filepath"
	"strings"
)

// MatchesExclude reports whether relPath (forward-slash, relative to walk root)
// matches the given gitignore-style pattern.
func MatchesExclude(relPath, pattern string) bool {
	pattern = filepath.ToSlash(strings.TrimSpace(pattern))
	if pattern == "" {
		return false
	}
	// Trailing "/" or "/**" → directory prefix match
	if strings.HasSuffix(pattern, "/**") {
		dir := strings.TrimSuffix(pattern, "/**")
		return relPath == dir || strings.HasPrefix(relPath, dir+"/")
	}
	if strings.HasSuffix(pattern, "/") {
		dir := strings.TrimSuffix(pattern, "/")
		return relPath == dir || strings.HasPrefix(relPath, dir+"/")
	}
	// Try matching the full relative path
	if matched, _ := path.Match(pattern, relPath); matched {
		return true
	}
	// Patterns without "/" are matched against the basename only
	if !strings.Contains(pattern, "/") {
		if matched, _ := path.Match(pattern, path.Base(relPath)); matched {
			return true
		}
	}
	return false
}
