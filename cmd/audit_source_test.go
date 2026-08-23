package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

// igTestDir gives the test its own working directory and a clean viper, since
// resolveIGSource reads both fhirlint.lock (relative) and the ig: config key.
func igTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	viper.Reset()
	t.Cleanup(viper.Reset)
	return dir
}

func writeLock(t *testing.T, dir string, ids ...string) {
	t.Helper()
	body := `{"validator":"6.10.2","packages":{`
	for i, id := range ids {
		if i > 0 {
			body += ","
		}
		body += `"` + id + `":{"url":"u","sha256":"s"}`
	}
	body += `}}`
	if err := os.WriteFile(filepath.Join(dir, "fhirlint.lock"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestResolveIGSource_PrefersLockFile(t *testing.T) {
	dir := igTestDir(t)
	writeLock(t, dir, "hl7.fhir.r4.core#4.0.1")
	viper.Set("ig", []string{"kbv.basis#1.5.0"})

	src, err := resolveIGSource()
	if err != nil {
		t.Fatal(err)
	}
	if src.Label != "fhirlint.lock" {
		t.Errorf("label = %q, want fhirlint.lock", src.Label)
	}
	if src.FromConfig {
		t.Error("a lock file must not be reported as the config fallback")
	}
	if len(src.IDs) != 1 || src.IDs[0] != "hl7.fhir.r4.core#4.0.1" {
		t.Errorf("IDs = %v, want the lock file's package, not the config's", src.IDs)
	}
}

func TestResolveIGSource_FallsBackToConfig(t *testing.T) {
	igTestDir(t)
	viper.Set("ig", []string{
		"kbv.basis#1.5.0",
		"de.medizininformatikinitiative.kerndatensatz.person#2025.0.1",
	})

	src, err := resolveIGSource()
	if err != nil {
		t.Fatal(err)
	}
	if !src.FromConfig {
		t.Error("want the config fallback to be marked as such")
	}
	if len(src.IDs) != 2 {
		t.Fatalf("IDs = %v, want both entries", src.IDs)
	}
	// Sorted, so the output is stable between runs.
	if src.IDs[0] > src.IDs[1] {
		t.Errorf("IDs are not sorted: %v", src.IDs)
	}
}

// Entries that name no registry version are reported, not silently dropped.
func TestResolveIGSource_SeparatesUnpinnedEntries(t *testing.T) {
	igTestDir(t)
	viper.Set("ig", []string{
		"kbv.basis#1.5.0", // auditable
		"kbv.basis",       // no version
		"./local-ig",      // a local directory
		"../other",        // ditto
		"some.pkg#latest", // not a registry version
	})

	src, err := resolveIGSource()
	if err != nil {
		t.Fatal(err)
	}
	if len(src.IDs) != 1 || src.IDs[0] != "kbv.basis#1.5.0" {
		t.Errorf("IDs = %v, want only the pinned entry", src.IDs)
	}
	if len(src.Unpinned) != 4 {
		t.Errorf("Unpinned = %v, want the other four", src.Unpinned)
	}
}

func TestResolveIGSource_NeitherSource(t *testing.T) {
	igTestDir(t)

	src, err := resolveIGSource()
	if err != nil {
		t.Fatal(err)
	}
	if src.Label != "" || len(src.IDs) != 0 || len(src.Unpinned) != 0 {
		t.Errorf("want an empty source, got %+v", src)
	}
}

// An empty lock file is the same as none: fall through to the config.
func TestResolveIGSource_EmptyLockFileFallsThrough(t *testing.T) {
	dir := igTestDir(t)
	writeLock(t, dir)
	viper.Set("ig", []string{"kbv.basis#1.5.0"})

	src, err := resolveIGSource()
	if err != nil {
		t.Fatal(err)
	}
	if !src.FromConfig || len(src.IDs) != 1 {
		t.Errorf("want the config fallback, got %+v", src)
	}
}

func TestResolveIGSource_UnreadableLockFileIsAnError(t *testing.T) {
	dir := igTestDir(t)
	if err := os.WriteFile(filepath.Join(dir, "fhirlint.lock"), []byte("{{{"), 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := resolveIGSource(); err == nil {
		t.Error("a malformed lock file must surface, not fall through to the config")
	}
}

// auditIGPackages must not reach the registry when there is nothing to check.
func TestAuditIGPackages_NoSourceMakesNoRequest(t *testing.T) {
	igTestDir(t)

	report, src, err := auditIGPackages()
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Packages) != 0 || src.Label != "" {
		t.Errorf("want an empty result, got %d packages from %q", len(report.Packages), src.Label)
	}
}

func TestConfigFileLabel_FallsBackWhenNoConfigLoaded(t *testing.T) {
	igTestDir(t)
	if got := configFileLabel(); got != "the config" {
		t.Errorf("configFileLabel() = %q, want %q", got, "the config")
	}
}
