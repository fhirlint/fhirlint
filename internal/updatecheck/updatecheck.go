// Package updatecheck looks up the newest release of a GitHub repository and
// remembers the answer, so fhirlint can mention a pending update without a
// network call on every run.
//
// It is deliberately repo-agnostic: fhirlint tracks two things that go out of
// date independently — the validator JAR and fhirlint itself (#360).
package updatecheck

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/fhirlint/fhirlint/internal/cache"
)

const (
	// Interval is how long a looked-up release stays good. A day is short
	// enough to be useful and long enough that the API's unauthenticated
	// 60/hour limit is never in play.
	Interval = 24 * time.Hour

	// timeout keeps an unreachable API from delaying the command the user
	// actually ran. The check is a courtesy; it never blocks work.
	timeout = 3 * time.Second
)

type entry struct {
	LastChecked   time.Time `json:"last_checked"`
	LatestVersion string    `json:"latest_version"`
}

// cacheFile is the on-disk shape. The single-repo format written before #360
// unmarshals into this as an empty map, which reads as "nothing cached" and
// costs one extra lookup — no migration needed, and nothing is lost that a
// single API call does not replace.
type cacheFile struct {
	Repos map[string]entry `json:"repos"`
}

// Latest returns the newest release tag for repo ("owner/name"), or "" when it
// could not be determined. A network failure is not an error the caller has to
// handle: there is nothing useful to say about it, and saying nothing is the
// right behaviour for a courtesy check.
func Latest(repo string) string {
	if cached, ok := readEntry(repo); ok && time.Since(cached.LastChecked) < Interval {
		return cached.LatestVersion
	}

	latest, err := FetchLatest(repo)
	if err != nil {
		return "" // network unavailable — fail silently
	}
	writeEntry(repo, entry{LastChecked: time.Now(), LatestVersion: latest})
	return latest
}

// FetchLatest queries the GitHub API for repo's latest release tag, bypassing
// the cache.
func FetchLatest(repo string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL(repo), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	// CI runners share outbound IPs and hit the unauthenticated rate limit as a
	// group. Use the token when the environment offers one.
	if tok := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	return release.TagName, nil
}

// apiBase is a var so tests can point the lookup at a local server.
var apiBase = "https://api.github.com"

func apiURL(repo string) string {
	return apiBase + "/repos/" + repo + "/releases/latest"
}

func readEntry(repo string) (entry, bool) {
	path, err := cache.UpdateCheckPath()
	if err != nil {
		return entry{}, false
	}
	data, err := os.ReadFile(path) //nolint:gosec // known cache path
	if err != nil {
		return entry{}, false
	}
	var f cacheFile
	if err := json.Unmarshal(data, &f); err != nil {
		return entry{}, false
	}
	e, ok := f.Repos[repo]
	return e, ok
}

// writeEntry updates one repo's entry, leaving the others in place. Errors are
// dropped: failing to cache a courtesy lookup is not worth interrupting a run.
func writeEntry(repo string, e entry) {
	path, err := cache.UpdateCheckPath()
	if err != nil {
		return
	}
	var f cacheFile
	if data, readErr := os.ReadFile(path); readErr == nil { //nolint:gosec // known cache path
		_ = json.Unmarshal(data, &f)
	}
	if f.Repos == nil {
		f.Repos = map[string]entry{}
	}
	f.Repos[repo] = e

	data, err := json.Marshal(f)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0600)
}
