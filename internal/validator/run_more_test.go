// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package validator

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/cache"
)

// The corners of run.go (#691).

func TestBuildArgs_FHIRSettings(t *testing.T) {
	args := buildArgs("v.jar", []string{"p.json"}, "", Options{FHIRVersion: "4.0.1", FHIRSettings: "settings.json"})
	i := slices.Index(args, "-fhir-settings")
	if i < 0 || args[i+1] != "settings.json" {
		t.Errorf("want -fhir-settings settings.json in %q", args)
	}
}

// RunWatch refuses what RunMultiple refuses, before any JVM starts.
func TestRunWatch_RefusesMoreUpFront(t *testing.T) {
	opts, dir := fakeSetup(t)
	p := put(t, dir, "p.json", `{"resourceType":"Patient"}`)
	o := opts
	o.ExpansionParameters = filepath.Join(dir, "missing-params.json")
	if err := RunWatch([]string{p}, o, WatchConfig{Mode: "all"}); err == nil || !strings.Contains(err.Error(), "--expansion-parameters") {
		t.Errorf("a missing expansion-parameters file: %v", err)
	}
	t.Setenv(cache.DirEnvVar, t.TempDir())
	o = opts
	o.JARPath, o.Offline = "", true
	if err := RunWatch([]string{p}, o, WatchConfig{Mode: "all"}); err == nil || !strings.Contains(err.Error(), "no validator JAR in the cache") {
		t.Errorf("--offline without a cached JAR: %v", err)
	}
	o = opts
	o.JARPath = writeJARWithManifest(t, t.TempDir(), "6.9.0")
	o.Offline = true
	if err := RunWatch([]string{p}, o, WatchConfig{Mode: "all"}); err == nil || !strings.Contains(err.Error(), "--offline") {
		t.Errorf("--offline with a validator too old to block its HTTP: %v", err)
	}
}

func TestParseOutput_MalformedShapes(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"OperationOutcome with a bad issue list", `{"resourceType":"OperationOutcome","issue":"oops"}`, "parsing OperationOutcome"},
		{"Bundle with a bad entry list", `{"resourceType":"Bundle","entry":{"x":1}}`, "parsing Bundle"},
	} {
		if _, err := parseOutputWith([]byte(tc.body), []string{"p.json"}, "", nil); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want %q", tc.name, err, tc.want)
		}
	}
}
