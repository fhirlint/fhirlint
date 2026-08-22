package updatecheck

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fhirlint/fhirlint/internal/cache"
)

// serveRelease stands in for the GitHub API and counts how often it was asked,
// so "the cache was used" can be asserted rather than assumed.
func serveRelease(t *testing.T, tag string) *atomic.Int32 {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		fmt.Fprintf(w, `{"tag_name":%q}`, tag)
	}))
	t.Cleanup(srv.Close)

	orig := apiBase
	apiBase = srv.URL
	t.Cleanup(func() { apiBase = orig })
	return &calls
}

// isolate points the cache at a temp dir. Without it these tests read and write
// the user's real ~/.fhirlint, which is how an earlier version of this suite
// deleted a developer's cached update check mid-run.
func isolate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(cache.DirEnvVar, dir)
	return dir
}

func seed(t *testing.T, f cacheFile) {
	t.Helper()
	path, err := cache.UpdateCheckPath()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestLatest_FetchesAndCaches(t *testing.T) {
	isolate(t)
	calls := serveRelease(t, "v2.0.0")

	if got := Latest("owner/repo"); got != "v2.0.0" {
		t.Fatalf("Latest() = %q, want v2.0.0", got)
	}
	if got := Latest("owner/repo"); got != "v2.0.0" {
		t.Fatalf("second Latest() = %q, want v2.0.0", got)
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("the API was called %d times; the second lookup must come from the cache", n)
	}
}

func TestLatest_StaleCacheRefetches(t *testing.T) {
	isolate(t)
	calls := serveRelease(t, "v3.0.0")
	seed(t, cacheFile{Repos: map[string]entry{
		"owner/repo": {LastChecked: time.Now().Add(-2 * Interval), LatestVersion: "v1.0.0"},
	}})

	if got := Latest("owner/repo"); got != "v3.0.0" {
		t.Errorf("Latest() = %q, want the refetched v3.0.0", got)
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("a stale entry must trigger exactly one lookup, got %d", n)
	}
}

func TestLatest_FreshCacheIsUsedWithoutNetwork(t *testing.T) {
	isolate(t)
	calls := serveRelease(t, "v9.9.9")
	seed(t, cacheFile{Repos: map[string]entry{
		"owner/repo": {LastChecked: time.Now(), LatestVersion: "v1.2.3"},
	}})

	if got := Latest("owner/repo"); got != "v1.2.3" {
		t.Errorf("Latest() = %q, want the cached v1.2.3", got)
	}
	if n := calls.Load(); n != 0 {
		t.Errorf("a fresh entry must not hit the network, got %d calls", n)
	}
}

// Two repos share one file; updating one must not drop the other.
func TestLatest_KeepsOtherReposOnWrite(t *testing.T) {
	isolate(t)
	serveRelease(t, "v2.0.0")
	seed(t, cacheFile{Repos: map[string]entry{
		"other/repo": {LastChecked: time.Now(), LatestVersion: "keep-me"},
	}})

	Latest("owner/repo")

	if got, ok := readEntry("other/repo"); !ok || got.LatestVersion != "keep-me" {
		t.Errorf("the untouched repo's entry was lost: %+v (ok=%v)", got, ok)
	}
	if got, ok := readEntry("owner/repo"); !ok || got.LatestVersion != "v2.0.0" {
		t.Errorf("the looked-up repo was not recorded: %+v (ok=%v)", got, ok)
	}
}

// The pre-#360 file had no "repos" key. It must read as "nothing cached"
// rather than crashing or being mistaken for a fresh entry.
func TestLatest_LegacyCacheFormatIsIgnored(t *testing.T) {
	dir := isolate(t)
	calls := serveRelease(t, "v4.0.0")
	legacy := `{"last_checked":"` + time.Now().Format(time.RFC3339) + `","latest_version":"6.9.7"}`
	if err := os.WriteFile(filepath.Join(dir, "update_check.json"), []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}

	if got := Latest("owner/repo"); got != "v4.0.0" {
		t.Errorf("Latest() = %q, want a fresh lookup", got)
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("the legacy format must produce one lookup, got %d", n)
	}
}

func TestLatest_UnreadableCacheStillWorks(t *testing.T) {
	dir := isolate(t)
	serveRelease(t, "v5.0.0")
	if err := os.WriteFile(filepath.Join(dir, "update_check.json"), []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := Latest("owner/repo"); got != "v5.0.0" {
		t.Errorf("Latest() = %q; a corrupt cache must not break the lookup", got)
	}
}

func TestLatest_NetworkFailureIsSilent(t *testing.T) {
	isolate(t)
	orig := apiBase
	apiBase = "http://127.0.0.1:9" // discard port: refuses immediately
	t.Cleanup(func() { apiBase = orig })

	if got := Latest("owner/repo"); got != "" {
		t.Errorf("Latest() = %q, want empty on a network failure", got)
	}
}

func TestFetchLatest_ParsesTagName(t *testing.T) {
	serveRelease(t, "7.1.0")
	got, err := FetchLatest("owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	if got != "7.1.0" {
		t.Errorf("FetchLatest() = %q, want 7.1.0", got)
	}
}

func TestFetchLatest_SendsTokenWhenPresent(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		fmt.Fprint(w, `{"tag_name":"v1.0.0"}`)
	}))
	defer srv.Close()
	orig := apiBase
	apiBase = srv.URL
	t.Cleanup(func() { apiBase = orig })

	t.Setenv("GITHUB_TOKEN", "secret")
	if _, err := FetchLatest("owner/repo"); err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer secret" {
		t.Errorf("Authorization = %q, want %q", auth, "Bearer secret")
	}
}

func TestAPIURL(t *testing.T) {
	if got := apiURL("owner/repo"); got != apiBase+"/repos/owner/repo/releases/latest" {
		t.Errorf("apiURL() = %q", got)
	}
}
