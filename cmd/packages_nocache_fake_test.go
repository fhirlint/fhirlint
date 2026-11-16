// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

// A fresh machine has no ~/.fhir/packages. --json must still print JSON there,
// so `fhirlint packages --json | jq` works on a new CI runner (#706).
func TestFakePackagesJSONWithoutCache(t *testing.T) {
	r := runFake(t, t.TempDir(), "packages", "--json")
	mustPass(t, r)
	var doc struct {
		Packages []any `json:"packages"`
		Count    *int  `json:"count"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &doc); err != nil || doc.Packages == nil || doc.Count == nil || *doc.Count != 0 {
		t.Errorf("want an empty package list as JSON, got %v:\n%s", err, r.stdout)
	}

	// Without --json the hint stays: it is what a person needs to read.
	r = runFakeKeepEnv(t, t.TempDir(), "packages")
	mustPass(t, r)
	if !strings.Contains(r.stdout, "No FHIR package cache yet") {
		t.Errorf("the terminal output must explain the missing cache:\n%s", r.stdout)
	}
}
