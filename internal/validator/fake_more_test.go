// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package validator

import (
	"fmt"
	"strings"
	"testing"
)

// Crash recovery in RunMultiple against the fake validator (#632): a JAR that
// throws on one file writes nothing for any, so fhirlint blames the file on
// the last "Validate" line and runs the rest again in a new JVM (#471).

func TestFakeRunMultiple_RecoversFromACrash(t *testing.T) {
	opts, dir := fakeSetup(t)
	ok := put(t, dir, "a.json", `{"resourceType":"Patient"}`)
	crash := put(t, dir, "b.json", `{"resourceType":"Patient","note":"FAKE_CRASH"}`)
	bad := put(t, dir, "c.json", `{"resourceType":"Patient","note":"FAKE_ERROR"}`)

	rs, err := RunMultiple([]string{ok, crash, bad}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 3 {
		t.Fatalf("one result per input, got %d", len(rs))
	}
	if !rs[0].Valid || rs[0].DurationMs == 0 {
		t.Errorf("the file before the crash, validated again, with its time: %+v", rs[0])
	}
	if rs[1].Issues[0].MessageID != MessageIDValidatorCrash || !strings.Contains(rs[1].Issues[0].Message, "IllegalStateException: fake crash") {
		t.Errorf("the crashed file gets the exception: %+v", rs[1].Issues)
	}
	if rs[2].Valid || rs[2].Issues[0].MessageID != "FAKE_ERROR_ID" {
		t.Errorf("the file after the crash, with its own finding: %+v", rs[2].Issues)
	}
	if !rs[1].Incomplete() || rs[0].Incomplete() || rs[2].Incomplete() {
		t.Error("only the crashed file's result stands in for a verdict, and must not be cached")
	}
}

func TestFakeRunMultiple_StopsAfterTooManyCrashes(t *testing.T) {
	opts, dir := fakeSetup(t)
	var paths []string
	for i := range maxValidatorCrashes + 2 {
		paths = append(paths, put(t, dir, fmt.Sprintf("c%d.json", i), `{"resourceType":"Patient","note":"FAKE_CRASH"}`))
	}
	rs, err := RunMultiple(paths, opts)
	if err != nil {
		t.Fatal(err)
	}
	crashed, skipped := 0, 0
	for _, r := range rs {
		switch r.Issues[0].MessageID {
		case MessageIDValidatorCrash:
			crashed++
		case MessageIDNotValidated:
			skipped++
			if !strings.Contains(r.Issues[0].Message, fmt.Sprintf("crashed on %d files", maxValidatorCrashes)) {
				t.Errorf("the reason must say why: %s", r.Issues[0].Message)
			}
		}
	}
	if crashed != maxValidatorCrashes || skipped != 2 {
		t.Errorf("want %d crashed and 2 not validated, got %d and %d", maxValidatorCrashes, crashed, skipped)
	}
}

func TestFakeCheckJava(t *testing.T) {
	fakeSetup(t)
	if err := CheckJava(); err != nil {
		t.Errorf("the fake reports Java 21: %v", err)
	}
	t.Setenv("PATH", t.TempDir())
	if err := CheckJava(); err == nil || !strings.Contains(err.Error(), "java not found") {
		t.Errorf("no java on PATH: %v", err)
	}
}
