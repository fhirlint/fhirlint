//go:build integration

package validator

import (
	"fmt"
	"os"
	"regexp"
	"testing"
)

// A pin reaches EnsureJAR as part of a download URL and a cache file name, so
// keep it to something that can only be a release tag.
var pinnedVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$`)

// TestMain puts the requested validator release in the cache before any
// integration test runs, and says which one the run used.
//
// The tests call Run/RunFHIRPath/RunCompare/StartServer without a pin, which
// means "whatever is cached, else latest" — fine locally, but it leaves CI
// testing against an arbitrary release (#310). Downloading up front makes the
// choice explicit for every test in the package.
//
// FHIRLINT_VALIDATOR_VERSION is the same variable the CLI honours. Unset leaves
// the cache as it is, so a runner without a cached JAR gets latest — which is
// what the weekly canary run wants.
func TestMain(m *testing.M) {
	pinned := os.Getenv("FHIRLINT_VALIDATOR_VERSION")
	if pinned != "" && !pinnedVersionPattern.MatchString(pinned) {
		fmt.Fprintf(os.Stderr, "integration tests: FHIRLINT_VALIDATOR_VERSION=%q is not a release tag\n", pinned)
		os.Exit(1)
	}
	if _, err := EnsureJAR("", pinned); err != nil {
		fmt.Fprintf(os.Stderr, "integration tests: preparing the validator JAR: %v\n", err)
		os.Exit(1)
	}
	// Printed before and after so a failure buried in test output still has the
	// version next to it.
	fmt.Fprintf(os.Stderr, "integration tests: running against validator %s\n", ValidatorVersion())
	code := m.Run()
	if code != 0 {
		fmt.Fprintf(os.Stderr, "integration tests: failed against validator %s\n", ValidatorVersion())
	}
	os.Exit(code)
}
