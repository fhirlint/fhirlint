// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// --since through a symlink to the repository still finds the changed file
// (#681). git reports the repository's resolved path; on macOS every temp
// directory sits behind /var -> /private/var, so without this --since
// validated nothing there and passed.
func TestFakeValidate_SinceThroughASymlink(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("needs git")
	}
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "repo")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("can't make a symlink here: %v", err)
	}
	writeFile(t, real, "fhir/a.json", cleanPatient)
	writeFile(t, real, "fhir/b.json", cleanPatient)
	git := func(args ...string) {
		t.Helper()
		c := exec.Command("git", append([]string{"-C", real, "-c", "user.name=t", "-c", "user.email=t@example.org", "-c", "commit.gpgsign=false"}, args...)...) //nolint:gosec // git with the test's own fixed arguments
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("add", ".")
	git("commit", "-q", "-m", "init")
	writeFile(t, real, "fhir/a.json", errPatient)

	r := runFake(t, link, "validate", "--no-color", "--since", "HEAD", "fhir")
	mustFail(t, r)
	if !strings.Contains(r.stdout, "Files: 1") || !strings.Contains(r.stdout, "Fake error found") {
		t.Errorf("the changed file must be validated through the symlink:\n%s\n%s", r.stdout, r.stderr)
	}
}
