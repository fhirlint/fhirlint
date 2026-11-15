// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package configcheck_test

import (
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/configcheck"
)

// Each kind of key with a value of the wrong shape, reported on its line
// (#634).
func TestCheck_WrongValueKinds(t *testing.T) {
	for _, tc := range []struct{ yaml, want string }{
		{"terminology-server: [a]\n", `line 1: "terminology-server" must be a string`},
		{"max-warnings: [1]\n", `line 1: "max-warnings" must be an integer`},
		{"max-warnings: x\n", `"max-warnings" must be an integer, got "x"`},
		{"severity: [error]\n", `line 1: "severity" must be a string`},
		{"quiet: [true]\n", `"quiet" must be a boolean`},
		{"ig: {a: b}\n", `line 1: "ig" must be a list of strings`},
		{"ig:\n  - [a]\n", `line 2: "ig" items must be strings`},
		{"format:\n  - [x]\n", `line 2: "format" items must be strings`},
		{"rules:\n  - {assert: x}\n", "line 2: rule must have an id"},
		{"rules:\n  - {id: x}\n", "line 2: rule must have an assert expression"},
		{"suppress:\n  - {messageId: x, severity: bogus}\n", `invalid value "bogus" for suppress severity`},
		{"lint: [x]\n", `"lint" must be a mapping of rule -> severity`},
		{"lint:\n  canonical-url-patern: error\n", `unknown lint rule "canonical-url-patern" (did you mean "canonical-url-pattern"?)`},
		{"lint:\n  canonical-url-pattern: error\n", `requires parameter "base" — use the map form`},
		{"lint:\n  canonical-url-pattern: {severity: error}\n", `requires parameter "base"`},
		{"lint:\n  canonical-url-pattern: {severity: loud, base: x}\n", `invalid severity "loud"`},
		{"lint:\n  canonical-url-pattern: [x]\n", "must be a severity string or a map"},
		{"profile-map: x\n", `"profile-map" must map resource types or filename globs to profiles`},
		{"profile-map:\n  Patient:\n    - [x]\n", "line 3: profile-map \"Patient\": an entry must be a profile URL"},
		{"profile-map:\n  Patient: {profile: p, when: [x]}\n", `"when" must be a FHIRPath expression`},
		{"profile-map:\n  Patient: {profile: p, when: 'name.('}\n", `"when" "name.(" does not compile`},
		{"profile-map:\n  Patient: {when: x}\n", `an entry with keys needs a "profile"`},
		{"profile-map:\n  Patient: {profile: p, other: x}\n", `unknown key "other" (allowed: profile, when)`},
		{"key: [unclosed\n", "YAML parse error"},
	} {
		issues, err := configcheck.Check(writeConfig(t, tc.yaml))
		if err != nil {
			t.Fatal(err)
		}
		var all []string
		for _, i := range issues {
			all = append(all, i.String())
		}
		if joined := strings.Join(all, "\n"); !strings.Contains(joined, tc.want) {
			t.Errorf("%q: want %q, got:\n%s", tc.yaml, tc.want, joined)
		}
	}
}

// The forms that are fine produce nothing.
func TestCheck_AcceptedForms(t *testing.T) {
	for _, y := range []string{
		"profile-map: ~\n",
		"lint:\n  canonical-url-pattern: {severity: warning, base: https://example.org}\n",
		"ig: kbv.basis#1.7.0\n", // one string for a list
		"format: json\n",
		"",
	} {
		issues, err := configcheck.Check(writeConfig(t, y))
		if err != nil || len(issues) != 0 {
			t.Errorf("%q: %v %v", y, issues, err)
		}
	}
}
