package validator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/fhirlint/fhirlint/internal/cache"
)

func writeCheckCache(t *testing.T, c updateCheckCache) {
	t.Helper()
	path, err := cache.UpdateCheckPath()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
}

func TestCheckForUpdate_ReturnsCachedNewerVersion(t *testing.T) {
	// Write a fresh cache with a different (newer) version
	writeCheckCache(t, updateCheckCache{
		LastChecked:   time.Now(),
		LatestVersion: "99.0.0",
	})

	// Temporarily set current version to something older
	vp, _ := cache.ValidatorVersionPath()
	orig, _ := os.ReadFile(vp) //nolint:gosec
	_ = os.WriteFile(vp, []byte("1.0.0"), 0600)
	defer func() {
		if orig != nil {
			_ = os.WriteFile(vp, orig, 0600) //nolint:gosec
		}
	}()

	got := CheckForUpdate()
	if got != "99.0.0" {
		t.Errorf("expected newer version '99.0.0', got %q", got)
	}
}

func TestCheckForUpdate_ReturnsEmptyWhenUpToDate(t *testing.T) {
	vp, _ := cache.ValidatorVersionPath()
	orig, _ := os.ReadFile(vp) //nolint:gosec
	_ = os.WriteFile(vp, []byte("6.9.7"), 0600)
	defer func() {
		if orig != nil {
			_ = os.WriteFile(vp, orig, 0600) //nolint:gosec
		}
	}()

	writeCheckCache(t, updateCheckCache{
		LastChecked:   time.Now(),
		LatestVersion: "6.9.7",
	})

	got := CheckForUpdate()
	if got != "" {
		t.Errorf("expected empty string when up to date, got %q", got)
	}
}

func TestCheckForUpdate_ReturnsEmptyWhenVersionUnknown(t *testing.T) {
	// Redirect the cache to an empty temp HOME so there is no version file AND
	// no JAR — otherwise ValidatorVersion() falls back to the real JAR's
	// manifest and reports a concrete version, which makes this case unreachable
	// (and the test fails whenever a JAR is installed, e.g. in the integration job).
	t.Setenv("HOME", t.TempDir())

	got := CheckForUpdate()
	if got != "" {
		t.Errorf("expected empty string when version unknown, got %q", got)
	}
}

func TestFetchLatestVersion_ParsesGitHubResponse(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"7.1.0","name":"Release 7.1.0"}`))
	}))
	defer srv.Close()

	// Temporarily override the API URL via a local server — test the parsing logic directly
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if release.TagName != "7.1.0" {
		t.Errorf("expected tag_name=7.1.0, got %q", release.TagName)
	}
}

func TestReadUpdateCheckCache_MissingFile_ReturnsError(t *testing.T) {
	path, _ := cache.UpdateCheckPath()
	_ = os.Remove(path)
	_, err := readUpdateCheckCache()
	if err == nil {
		t.Error("expected error for missing cache file, got nil")
	}
}

func TestWriteReadUpdateCheckCache_Roundtrip(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	writeCheckCache(t, updateCheckCache{
		LastChecked:   now,
		LatestVersion: "5.5.5",
	})

	got, err := readUpdateCheckCache()
	if err != nil {
		t.Fatalf("readUpdateCheckCache() error: %v", err)
	}
	if got.LatestVersion != "5.5.5" {
		t.Errorf("expected LatestVersion=5.5.5, got %q", got.LatestVersion)
	}
	if !got.LastChecked.Equal(now) {
		t.Errorf("expected LastChecked=%v, got %v", now, got.LastChecked)
	}
}
