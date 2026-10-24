package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func setRootFlag(t *testing.T, name, value string) {
	t.Helper()
	f := rootCmd.PersistentFlags().Lookup(name)
	old, oldChanged := f.Value.String(), f.Changed
	if err := f.Value.Set(value); err != nil {
		t.Fatal(err)
	}
	f.Changed = true
	t.Cleanup(func() {
		_ = f.Value.Set(old)
		f.Changed = oldChanged
	})
}

// A pull request's own fhirlint.yml and .fhirlintignore are ignored under
// --no-repo-config, with a note; a --config file is still read (#499).
func TestNoRepoConfig(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	t.Chdir(dir)
	for name, body := range map[string]string{
		"fhirlint.yml":    "severity: error\nfail-on: never\n",
		".fhirlintignore": "**/*.json\n",
		"trusted.yml":     "severity: warning\n",
	} {
		if err := os.WriteFile(name, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	setRootFlag(t, "no-repo-config", "true")
	stderr := captureStderr(t, initConfig)
	if viper.ConfigFileUsed() != "" || viper.IsSet("fail-on") {
		t.Errorf("the repository's fhirlint.yml was read: %s, fail-on set %v", viper.ConfigFileUsed(), viper.IsSet("fail-on"))
	}
	if !strings.Contains(stderr, "fhirlint.yml") || !strings.Contains(stderr, "--no-repo-config") {
		t.Errorf("no note about the ignored config:\n%s", stderr)
	}

	var patterns []string
	stderr = captureStderr(t, func() {
		var err error
		if patterns, err = loadRepoIgnoreFile(); err != nil {
			t.Fatal(err)
		}
	})
	if len(patterns) != 0 || !strings.Contains(stderr, ".fhirlintignore") {
		t.Errorf("the repository's .fhirlintignore applied: %q, note %q", patterns, stderr)
	}

	if err := runConfigCheck(nil, nil); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("config check under --no-repo-config: %v, want it to find no config", err)
	}

	// An explicit --config is the caller's file and is read.
	resetViper(t)
	setRootFlag(t, "config", filepath.Join(dir, "trusted.yml"))
	initConfig()
	if got := viper.GetString("severity"); got != "warning" {
		t.Errorf("severity = %q from --config, want warning", got)
	}
}

// Without the flag nothing changes: both files apply.
func TestRepoConfigReadByDefault(t *testing.T) {
	resetViper(t)
	t.Chdir(t.TempDir())
	if err := os.WriteFile("fhirlint.yml", []byte("fail-on: never\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".fhirlintignore", []byte("x/**\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	initConfig()
	if viper.GetString("fail-on") != "never" {
		t.Errorf("fail-on = %q, want the repository's never", viper.GetString("fail-on"))
	}
	if patterns, err := loadRepoIgnoreFile(); err != nil || len(patterns) != 1 {
		t.Errorf("loadRepoIgnoreFile = %q, %v", patterns, err)
	}
}
