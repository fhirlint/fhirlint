//go:build integration

package cmd

import (
	"fmt"
	"os"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

// TestMain puts the requested validator release in the cache before the first
// integration test in this package runs.
//
// These tests call validator.Run and friends without a pin, which means
// "whatever is cached, else latest". That is fine locally and wrong in CI,
// where the point is to test against a known release — the same reasoning as
// the TestMain in internal/validator, and the same environment variable.
//
// The package is deliberately self-sufficient rather than relying on the
// validator package's tests having run first: `go test -tags integration
// ./cmd/...` on a machine with no cached JAR must honour the pin too. A pin
// that is not a release tag fails in EnsureJAR, which reports the download URL
// it could not fetch.
func TestMain(m *testing.M) {
	if _, err := validator.EnsureJAR("", os.Getenv("FHIRLINT_VALIDATOR_VERSION")); err != nil {
		fmt.Fprintf(os.Stderr, "integration tests: preparing the validator JAR: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "integration tests: running against validator %s\n", validator.ValidatorVersion())
	os.Exit(m.Run())
}
