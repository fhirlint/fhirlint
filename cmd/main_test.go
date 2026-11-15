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

// runCLIEnv makes the test binary run the fhirlint CLI instead of the tests.
// upgrade-check runs its own binary twice (os.Executable), which in a test is
// this one (#629).
const runCLIEnv = "FHIRLINT_TEST_RUN_CLI"

func TestMain(m *testing.M) {
	if os.Getenv(runCLIEnv) == "1" {
		Execute() // exits itself on an error
		os.Exit(0)
	}
	if err := setUpTests(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	tearDownTests(code)
	os.Exit(code)
}
