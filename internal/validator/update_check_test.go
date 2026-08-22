package validator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fhirlint/fhirlint/internal/cache"
)

// seedUpdateCache writes a fresh cache entry for the JAR repo, so CheckForUpdate
// answers from it instead of reaching the network.
//
// Every test here isolates the cache directory first. An earlier version of
// this suite wrote to and deleted from the developer's real ~/.fhirlint.
func seedUpdateCache(t *testing.T, latest string) {
	t.Helper()
	path, err := cache.UpdateCheckPath()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{
		"repos": map[string]any{
			jarRepo: map[string]any{
				"last_checked":   time.Now().Format(time.RFC3339Nano),
				"latest_version": latest,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func setInstalledVersion(t *testing.T, version string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(cache.DirEnvVar, dir)
	if err := os.WriteFile(filepath.Join(dir, "validator_version.txt"), []byte(version), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCheckForUpdate_ReturnsCachedNewerVersion(t *testing.T) {
	setInstalledVersion(t, "1.0.0")
	seedUpdateCache(t, "99.0.0")
	stubLatest(t, "99.0.0")

	if got := CheckForUpdate(); got != "99.0.0" {
		t.Errorf("CheckForUpdate() = %q, want 99.0.0", got)
	}
}

func TestCheckForUpdate_ReturnsEmptyWhenUpToDate(t *testing.T) {
	setInstalledVersion(t, "6.9.7")
	seedUpdateCache(t, "6.9.7")
	stubLatest(t, "6.9.7")

	if got := CheckForUpdate(); got != "" {
		t.Errorf("CheckForUpdate() = %q, want empty when up to date", got)
	}
}

func TestCheckForUpdate_ReturnsEmptyWhenVersionUnknown(t *testing.T) {
	// An empty cache dir means no version file and no JAR, so ValidatorVersion()
	// cannot fall back to reading a manifest and reports "unknown".
	t.Setenv(cache.DirEnvVar, t.TempDir())
	stubLatest(t, "99.0.0")

	if got := CheckForUpdate(); got != "" {
		t.Errorf("CheckForUpdate() = %q, want empty when the installed version is unknown", got)
	}
}

// A lookup that fails yields no version; the notice must simply not appear.
func TestCheckForUpdate_FailedLookupIsSilent(t *testing.T) {
	setInstalledVersion(t, "6.9.7")
	stubLatest(t, "")

	if got := CheckForUpdate(); got != "" {
		t.Errorf("CheckForUpdate() = %q, want empty when the lookup fails", got)
	}
}

// stubLatest replaces the release lookup, keeping these tests off the network.
func stubLatest(t *testing.T, latest string) {
	t.Helper()
	orig := latestJARRelease
	latestJARRelease = func(string) string { return latest }
	t.Cleanup(func() { latestJARRelease = orig })
}
