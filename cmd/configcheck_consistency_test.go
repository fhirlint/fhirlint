package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/configcheck"
	"github.com/spf13/pflag"
)

// cliOnlyFlags are validate flags that intentionally have no fhirlint.yml counterpart.
var cliOnlyFlags = map[string]bool{
	"lock":              true,
	"generate-baseline": true,
}

// configOnlyKeys are fhirlint.yml keys that have no CLI flag equivalent.
var configOnlyKeys = map[string]bool{
	"profile-map": true,
	"overrides":   true,
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
		if validateCmd.Flags().Lookup(key) == nil {
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
