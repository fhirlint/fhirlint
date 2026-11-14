// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractAction(t *testing.T) {
	in := `name: x
runs:
  using: composite
  steps:
    - shell: bash
      run: |
        echo "${{ inputs.path }}"
    - shell: pwsh
      run: Write-Host hi
    - uses: actions/checkout@v1
`
	got, err := extract("action.yml", []byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d scripts, want 1 (bash only): %q", len(got), got)
	}
	if !strings.HasPrefix(got[0], "#!/usr/bin/env bash\n") || !strings.Contains(got[0], `echo "${TEMPLATE}"`) {
		t.Errorf("script = %q", got[0])
	}
}

func TestExtractGitLab(t *testing.T) {
	in := `spec:
  inputs:
    path: {}
---
job:
  before_script:
    - echo before
  script:
    - |
      if [ -n "$[[ inputs.path ]]" ]; then echo y; fi
    - echo two
variables:
  A: b
`
	got, err := extract("templates/fhirlint.yml", []byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d scripts, want 2: %q", len(got), got)
	}
	joined := strings.Join(got, "\n")
	for _, want := range []string{"#!/usr/bin/env sh", "echo before", `[ -n "${TEMPLATE}" ]`, "echo two"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %q", want, joined)
		}
	}
}

func TestRunWritesFiles(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "action.yml")
	if err := os.WriteFile(src, []byte("runs:\n  steps:\n    - shell: bash\n      run: echo hi\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	if code := run([]string{"-o", out, src}, os.Stderr); code != 0 {
		t.Fatalf("exit %d", code)
	}
	entries, err := os.ReadDir(out)
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries %v, err %v", entries, err)
	}
	if name := entries[0].Name(); strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".sh") {
		t.Errorf("script name %q must match *.sh", name)
	}
	if code := run([]string{"-o", out}, os.Stderr); code != 2 {
		t.Errorf("no files: exit %d, want 2", code)
	}
}
