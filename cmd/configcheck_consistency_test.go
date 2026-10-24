package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/configcheck"
	"github.com/fhirlint/fhirlint/internal/validator"
	"github.com/spf13/pflag"
)

// cliOnlyFlags are validate flags that intentionally have no fhirlint.yml counterpart.
var cliOnlyFlags = map[string]bool{
	"lock":              true,
	"generate-baseline": true,
	"rules-file":        true,
}

// configOnlyKeys are fhirlint.yml keys that have no CLI flag equivalent.
var configOnlyKeys = map[string]bool{
	"profile-map":       true,
	"overrides":         true,
	"rules":             true,
	"lint":              true,
	"severity-override": true,
}

// TestAllValidateFlagsKnownToConfigCheck ensures every --validate flag (except
// CLI-only ones) has a corresponding entry in configcheck.topLevelKeys.
// This prevents drift when a new flag is added to validate.go without updating
// configcheck.
func TestAllValidateFlagsKnownToConfigCheck(t *testing.T) {
	known := configcheck.KnownKeys()
	validateCmd.Flags().VisitAll(func(f *pflag.Flag) {
		if cliOnlyFlags[f.Name] {
			return
		}
		if _, ok := known[f.Name]; !ok {
			t.Errorf("flag --%s is not in configcheck.topLevelKeys — add it or mark it cliOnlyFlags", f.Name)
		}
	})
}

// TestAllKnownConfigKeysHaveFlag ensures every configcheck key (except
// config-only ones) has a corresponding --flag in validateCmd.
// This prevents drift when a new config key is added to configcheck without
// also adding the flag.
func TestAllKnownConfigKeysHaveFlag(t *testing.T) {
	for key := range configcheck.KnownKeys() {
		if configOnlyKeys[key] {
			continue
		}
		// Root persistent flags count too (--validator-version). Checked via
		// rootCmd rather than validateCmd.InheritedFlags(), because that call
		// merges the persistent flags into validateCmd.Flags() as a side effect
		// and would make the flag-documentation test below fail depending on
		// which test ran first.
		if validateCmd.Flags().Lookup(key) == nil && rootCmd.PersistentFlags().Lookup(key) == nil {
			t.Errorf("configcheck key %q has no --flag in validateCmd — add it or mark it configOnlyKeys", key)
		}
	}
}

// TestAllValidateFlagsDocumented ensures every --validate flag is mentioned in
// README.md so the flag table stays in sync with the code.
func TestAllValidateFlagsDocumented(t *testing.T) {
	data, err := os.ReadFile("../README.md")
	if err != nil {
		t.Fatalf("could not read README.md: %v", err)
	}
	content := string(data)
	validateCmd.Flags().VisitAll(func(f *pflag.Flag) {
		if !strings.Contains(content, "--"+f.Name) {
			t.Errorf("flag --%s not mentioned anywhere in README.md", f.Name)
		}
	})
}

// The README's --fhir-version row and the comment in fhirlint.yml.example are
// the two places the version list is not derived from validator.FHIRVersions,
// so they are the ones that silently go stale when a release is added (#306).
func TestFHIRVersionsDocumentedInREADME(t *testing.T) {
	data, err := os.ReadFile("../README.md")
	if err != nil {
		t.Fatalf("could not read README.md: %v", err)
	}
	content := string(data)
	for _, v := range validator.FHIRVersions {
		if !strings.Contains(content, v.ID) {
			t.Errorf("FHIR version %s (%s) is supported but not mentioned in README.md", v.ID, v.Name)
		}
	}
}

// The example spells the list out in its comment, so compare it whole: a
// release dropped from the table must disappear from the example too.
func TestFHIRVersionsDocumentedInExampleConfig(t *testing.T) {
	data, err := os.ReadFile("../fhirlint.yml.example")
	if err != nil {
		t.Fatalf("could not read fhirlint.yml.example: %v", err)
	}
	const prefix = "# FHIR version to validate against: "
	want := prefix + strings.Join(validator.FHIRVersionIDs(), " | ")
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r") // a Windows checkout has CRLF
		if strings.HasPrefix(line, prefix) {
			if line != want {
				t.Errorf("fhirlint.yml.example has\n\t%s\nwant\n\t%s", line, want)
			}
			return
		}
	}
	t.Errorf("fhirlint.yml.example has no line starting with %q", prefix)
}
