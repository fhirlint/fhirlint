package cmd

import (
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
	"github.com/spf13/cobra"
)

func TestFlagCompletions_KeyFlags(t *testing.T) {
	cases := map[string][]string{
		"fhir-version":  {"4.0.1", "4.3.0", "5.0.0"},
		"fail-on":       {"error", "warning", "information", "never"},
		"best-practice": {"ignore", "hint", "warning", "error"},
		"severity":      {"information", "warning", "error"},
		"format":        {"terminal", "json", "html", "junit", "sarif", "markdown", "codeclimate"},
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

// Every --fhir-version flag in the tree must take its choices and its help text
// from validator.FHIRVersions. Before #306 each command spelled the list out
// itself, so a new release meant finding all of them; this fails if one starts
// spelling it out again.
func TestFHIRVersionFlags_ComeFromTheTable(t *testing.T) {
	want := validator.FHIRVersionIDs()
	checked := 0

	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if f := c.Flags().Lookup("fhir-version"); f != nil {
			checked++
			if !strings.Contains(f.Usage, validator.FHIRVersionList()) {
				t.Errorf("%s: --fhir-version help %q does not list %s",
					c.CommandPath(), f.Usage, validator.FHIRVersionList())
			}
			if fn, ok := c.GetFlagCompletionFunc("fhir-version"); ok {
				got, _ := fn(c, nil, "")
				if strings.Join(got, ",") != strings.Join(want, ",") {
					t.Errorf("%s: completion = %v, want %v", c.CommandPath(), got, want)
				}
			}
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(rootCmd)

	// A guard that checks nothing is worse than no guard: if the flag is renamed
	// or the walk stops finding commands, say so instead of passing.
	if checked < 6 {
		t.Errorf("only found %d --fhir-version flags, expected the whole command tree", checked)
	}
}

// The default has to be a version the flag would accept, on every command that
// defaults it — a default outside the table fails only at validation time.
func TestFHIRVersionFlags_DefaultsAreSupported(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if f := c.Flags().Lookup("fhir-version"); f != nil && f.DefValue != "" {
			if _, ok := validator.LookupFHIRVersion(f.DefValue); !ok {
				t.Errorf("%s: --fhir-version default %q is not a supported release",
					c.CommandPath(), f.DefValue)
			}
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(rootCmd)
}
