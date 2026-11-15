// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"
	"testing"
)

// setUpTests and tearDownTests are set in init by the build-tagged test
// files: the fake validator for the unit tests, the real JAR for -tags
// integration. One untagged TestMain serves both, because gobco type-checks
// every file regardless of build tags and would see two (#617).
var (
	setUpTests    = func() error { return nil }
	tearDownTests = func(code int) {}
)

func TestMain(m *testing.M) {
	if err := setUpTests(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	tearDownTests(code)
	os.Exit(code)
}
