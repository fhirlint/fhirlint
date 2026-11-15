// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package configcheck_test

import (
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/configcheck"
)

// A root that is not a mapping is a problem, not a valid file with no keys
// (#646): fhirlint cannot read it.
func TestCheck_RootMustBeAMapping(t *testing.T) {
	for _, y := range []string{"- severity: error\n", "severity error\n"} {
		issues, err := configcheck.Check(writeConfig(t, y))
		if err != nil {
			t.Fatal(err)
		}
		if len(issues) != 1 || !strings.Contains(issues[0].String(), "line 1: the config must be a mapping") {
			t.Errorf("%q: %v", y, issues)
		}
	}
}
