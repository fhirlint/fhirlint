// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func allPaths() map[string]func() (string, error) {
	return map[string]func() (string, error){
		"Dir":                  Dir,
		"JARPath":              JARPath,
		"ValidatorVersionPath": ValidatorVersionPath,
		"UpdateCheckPath":      UpdateCheckPath,
		"AdvisoryCheckPath":    AdvisoryCheckPath,
		"ChecksumStatusPath":   ChecksumStatusPath,
		"ResultCacheDir":       ResultCacheDir,
	}
}

// The advisory lookup is remembered next to the update check, under the same
// override, so a read-only home or a CI volume holds it too (#704).
func TestAdvisoryCheckPath_InsideTheCacheDir(t *testing.T) {
	root := t.TempDir()
	t.Setenv(DirEnvVar, root)
	got, err := AdvisoryCheckPath()
	if err != nil || got != filepath.Join(root, "advisory_check.json") {
		t.Errorf("AdvisoryCheckPath() = %q, %v; want %q", got, err, filepath.Join(root, "advisory_check.json"))
	}
}

// A cache directory that cannot be created is an error from every path, not a
// path into a directory that is not there: the caller would otherwise fail
// later with a less useful message, or silently skip its cache (#704).
func TestCachePaths_ErrorWhenTheCacheDirCannotBeCreated(t *testing.T) {
	file := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(DirEnvVar, filepath.Join(file, "cache"))
	for name, fn := range allPaths() {
		if got, err := fn(); err == nil {
			t.Errorf("%s() = %q with no error, under a path below a file", name, got)
		}
	}
}
