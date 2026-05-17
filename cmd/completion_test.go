package cmd

import (
	"testing"
)

func TestFlagCompletions_KeyFlags(t *testing.T) {
	cases := map[string][]string{
		"fhir-version":  {"4.0.1", "4.3.0", "5.0.0"},
		"fail-on":       {"error", "warning", "information", "never"},
		"best-practice": {"ignore", "hint", "warning", "error"},
		"severity":      {"information", "warning", "error"},
		"format":        {"terminal", "json", "html", "junit", "sarif"},
		"watch":         {"single", "all"},
	}
	for flag, want := range cases {
		fn, ok := validateCmd.GetFlagCompletionFunc(flag)
		if !ok {
			t.Errorf("flag completion not registered for --%s", flag)
			continue
		}
		got, _ := fn(validateCmd, nil, "")
		wantSet := make(map[string]bool, len(want))
		for _, v := range want {
			wantSet[v] = true
		}
		for _, v := range got {
			delete(wantSet, v)
		}
		for missing := range wantSet {
			t.Errorf("--%s completion missing %q, got %v", flag, missing, got)
		}
	}
}
