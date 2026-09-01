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
	// Rejected records releases that were downloaded and refused, keyed by repo
	// and then by version. Without it the update notice can only ask whether a
	// newer release exists, not whether it can be installed, and goes on
	// recommending one that fails (#377).
	Rejected map[string]map[string]Rejection `json:"rejected,omitempty"`
}

// Rejection is a release that failed verification on this machine.
type Rejection struct {
	// Reason is short and user-facing; it goes straight into the notice.
	Reason string    `json:"reason"`
	When   time.Time `json:"when"`
}

// RecordRejection remembers that a release was refused, so the notice can say
// so instead of recommending it. Errors are dropped: failing to write this must
// never turn into a second failure on top of the one being reported.
func RecordRejection(repo, version, reason string) {
	if version == "" {
		return // nothing to key on
	}
	mutate(func(f *cacheFile) {
		if f.Rejected == nil {
			f.Rejected = map[string]map[string]Rejection{}
		}
		if f.Rejected[repo] == nil {
			f.Rejected[repo] = map[string]Rejection{}
		}
		f.Rejected[repo][version] = Rejection{Reason: reason, When: time.Now()}
	})
}

// ClearRejection forgets a rejection, called when that version installs
// successfully. An upstream fix therefore stops being annotated on its own,
// without anyone editing the cache by hand.
func ClearRejection(repo, version string) {
	if version == "" {
		return
	}
	mutate(func(f *cacheFile) {
		if f.Rejected[repo] == nil {
			return
		}
		delete(f.Rejected[repo], version)
		if len(f.Rejected[repo]) == 0 {
			delete(f.Rejected, repo)
		}
	})
}

// LookupRejection reports whether a release was refused here.
func LookupRejection(repo, version string) (Rejection, bool) {
	f, err := readCache()
	if err != nil {
		return Rejection{}, false
	}
	r, ok := f.Rejected[repo][version]
	return r, ok
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

// readCache loads the whole file. A missing or unparseable file reads as empty
// rather than as an error: this is a cache, and losing it costs one lookup.
func readCache() (cacheFile, error) {
	path, err := cache.UpdateCheckPath()
	if err != nil {
		return cacheFile{}, err
	}
	data, err := os.ReadFile(path) //nolint:gosec // known cache path
	if err != nil {
		return cacheFile{}, err
	}
	var f cacheFile
	if err := json.Unmarshal(data, &f); err != nil {
		return cacheFile{}, err
	}
	return f, nil
}

// mutate applies fn to the cache file and writes it back, leaving every part it
// does not touch in place.
func mutate(fn func(*cacheFile)) {
	path, err := cache.UpdateCheckPath()
	if err != nil {
		return
	}
	f, _ := readCache() // an unreadable cache starts over rather than blocking
	fn(&f)
	data, err := json.Marshal(f)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0600)
}

func readEntry(repo string) (entry, bool) {
	f, err := readCache()
	if err != nil {
		return entry{}, false
	}
	e, ok := f.Repos[repo]
	return e, ok
}

// writeEntry updates one repo's entry, leaving the others in place. Errors are
// dropped: failing to cache a courtesy lookup is not worth interrupting a run.
func writeEntry(repo string, e entry) {
	mutate(func(f *cacheFile) {
		if f.Repos == nil {
			f.Repos = map[string]entry{}
		}
		f.Repos[repo] = e
	})
}
