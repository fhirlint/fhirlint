package cmd

import (
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

// resetSince clears the package-level state --since builds up, so tests do not
// leak into each other.
func resetSince(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		sinceChanged = nil
		sinceExcluded = nil
		flagSince = ""
	})
	sinceChanged = nil
	sinceExcluded = nil
	flagSince = ""
}

func TestFilterSincePaths_Inactive(t *testing.T) {
	resetSince(t)

	paths := []string{"a.json", "b.json"}
	got := filterSincePaths(paths)

	if len(got) != 2 {
		t.Errorf("without --since every path must survive, got %v", got)
	}
	if sinceExcluded != nil {
		t.Errorf("nothing should be recorded as excluded, got %v", sinceExcluded)
	}
}

func TestFilterSincePaths_KeepsChangedRecordsRest(t *testing.T) {
	resetSince(t)
	flagSince = "main"

	changed, err := filepath.Abs("changed.json")
	if err != nil {
		t.Fatal(err)
	}
	sinceChanged = map[string]bool{changed: true}

	got := filterSincePaths([]string{"changed.json", "untouched.json"})

	if len(got) != 1 || got[0] != "changed.json" {
		t.Errorf("expected only changed.json, got %v", got)
	}
	// The dropped file must be remembered — --check-references indexes it so
	// references pointing into the unchanged part still resolve.
	if len(sinceExcluded) != 1 || sinceExcluded[0] != "untouched.json" {
		t.Errorf("expected untouched.json to be recorded as excluded, got %v", sinceExcluded)
	}
}

func TestFilterSincePaths_NoneChanged(t *testing.T) {
	resetSince(t)
	flagSince = "main"
	sinceChanged = map[string]bool{}

	got := filterSincePaths([]string{"a.json", "b.json"})

	if len(got) != 0 {
		t.Errorf("expected no paths to survive, got %v", got)
	}
	if len(sinceExcluded) != 2 {
		t.Errorf("both paths should be recorded as excluded, got %v", sinceExcluded)
	}
}

func TestConfigFile_SinceFromConfig(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, "since: main\n")

	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig: %v", err)
	}

	if got := viper.GetString("since"); got != "main" {
		t.Errorf("since = %q, want %q", got, "main")
	}
}
