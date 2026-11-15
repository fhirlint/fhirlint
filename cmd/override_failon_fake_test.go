// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"strings"
	"testing"
)

// An override's fail-on other than never is refused at startup instead of
// being ignored (#672); never still keeps its files from failing the run.
func TestFakeOverrideFailOn(t *testing.T) {
	for _, tc := range []struct{ value, want string }{
		{"warning", `overrides: fail-on "warning" has no effect`},
		{"nevr", `overrides: fail-on "nevr" has no effect`},
	} {
		dir := t.TempDir()
		writeFile(t, dir, "legacy/p.json", errPatient)
		writeFile(t, dir, "fhirlint.yml", "overrides:\n  - files: ['legacy/**']\n    fail-on: "+tc.value+"\n")
		r := runFake(t, dir, "validate", "legacy")
		if r.err == nil || !strings.Contains(r.err.Error(), tc.want) {
			t.Errorf("fail-on %s: want %q, got %v", tc.value, tc.want, r.err)
		}
	}
	dir := t.TempDir()
	writeFile(t, dir, "legacy/p.json", errPatient)
	writeFile(t, dir, "fhirlint.yml", "overrides:\n  - files: ['legacy/**']\n    fail-on: never\n")
	if r := runFake(t, dir, "validate", "legacy"); r.err != nil {
		t.Errorf("fail-on never must keep the file from failing the run: %v", r.err)
	}
}
