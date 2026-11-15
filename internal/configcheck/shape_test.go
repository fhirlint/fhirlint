// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package configcheck_test

import (
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/configcheck"
)

// A config whose lists have the wrong shape: each mistake is reported with
// its line, instead of being skipped or crashing the check (#616).
func TestCheck_WrongShapes(t *testing.T) {
	for _, tc := range []struct {
		name, yaml, want string
	}{
		{"suppress not a list", "suppress: messageId:dom-6\n", `"suppress" must be a list`},
		{"suppress item a number", "suppress:\n  - 42\n  - [a]\n", "suppress rule must be a string or map"},
		{"rules not a list", "rules: {id: x}\n", `"rules" must be a list`},
		{"rule a string", "rules:\n  - just-text\n", "rule must be a map with id and assert"},
		{"overrides not a list", "overrides: x\n", `"overrides" must be a list`},
		{"override a string", "overrides:\n  - x\n", "override entry must be a map"},
		{"format list with a bad item", "format: [terminal, pdf]\n", `invalid value "pdf" for "format"`},
		{"format a bad scalar", "format: pdf\n", `invalid value "pdf" for "format"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issues, err := configcheck.Check(writeConfig(t, tc.yaml))
			if err != nil {
				t.Fatal(err)
			}
			var all []string
			for _, i := range issues {
				all = append(all, i.String())
			}
			joined := strings.Join(all, "\n")
			if !strings.Contains(joined, tc.want) || !strings.Contains(joined, "line ") {
				t.Errorf("want %q with a line number, got:\n%s", tc.want, joined)
			}
		})
	}
}

func TestIssueString(t *testing.T) {
	if got := (configcheck.Issue{Line: 3, Message: "m"}).String(); got != "line 3: m" {
		t.Errorf("with a line: %q", got)
	}
	if got := (configcheck.Issue{Message: "m"}).String(); got != "m" {
		t.Errorf("without a line: %q", got)
	}
}

func TestKnownKeysCoverTheDocumentedOnes(t *testing.T) {
	known := configcheck.KnownKeys()
	for _, k := range []string{"severity", "fail-on", "suppress", "overrides", "profile-map", "lint", "rules"} {
		if _, ok := known[k]; !ok {
			t.Errorf("KnownKeys lacks %q", k)
		}
	}
}
