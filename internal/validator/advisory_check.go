package validator

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/fhirlint/fhirlint/internal/cache"
)

// advisoryCheckInterval matches the release lookup's. An advisory is published
// long after the release it describes, so a project that pinned months ago can
// become vulnerable without anything changing on its side (#385). Once a day is
// often enough to catch that and rare enough to be free.
const advisoryCheckInterval = 24 * time.Hour

// advisoryFetcher is a var so tests can drive the summary without a network
// call or a cache file.
var advisoryFetcher = fetchAdvisories

// AdvisorySummary is what the update notice needs to know: how many published
// advisories affect the installed validator, and how bad the worst one is.
type AdvisorySummary struct {
	// Version is the validator the counts were computed for. Cached against it,
	// so pinning a different release re-checks instead of reusing an answer
	// about a version no longer in use.
	Version     string    `json:"version"`
	Count       int       `json:"count"`
	Highest     string    `json:"highest"`
	LastChecked time.Time `json:"last_checked"`
}

// AffectingAdvisories reports the advisories that affect the installed
// validator, from a lookup cached for a day.
//
// ok is false when there is nothing to say: no JAR, an unreachable API, or a
// version the advisory ranges cannot be evaluated against. A courtesy check
// never delays or breaks a run, so a failure here is silence rather than an
// error.
func AffectingAdvisories() (summary AdvisorySummary, ok bool) {
	current := ValidatorVersion()
	if current == "unknown" {
		return AdvisorySummary{}, false
	}

	if cached, found := readAdvisoryCache(); found &&
		cached.Version == current &&
		time.Since(cached.LastChecked) < advisoryCheckInterval {
		return cached, true
	}

	advisories, err := advisoryFetcher()
	if err != nil {
		return AdvisorySummary{}, false
	}

	out := AdvisorySummary{Version: current, LastChecked: time.Now()}
	for _, a := range advisories {
		if a.AffectsVersion(current) {
			out.Count++
			out.Highest = worseSeverity(out.Highest, a.Severity)
		}
	}
	writeAdvisoryCache(out)
	return out, true
}

// severityRank orders GitHub's advisory severities. Anything unrecognised
// ranks lowest rather than being reported as the worst thing found.
var severityRank = map[string]int{"low": 1, "medium": 2, "moderate": 2, "high": 3, "critical": 4}

func worseSeverity(a, b string) string {
	if severityRank[strings.ToLower(b)] > severityRank[strings.ToLower(a)] {
		return strings.ToLower(b)
	}
	return a
}

func readAdvisoryCache() (AdvisorySummary, bool) {
	path, err := cache.AdvisoryCheckPath()
	if err != nil {
		return AdvisorySummary{}, false
	}
	data, err := os.ReadFile(path) //nolint:gosec // known cache path
	if err != nil {
		return AdvisorySummary{}, false
	}
	var s AdvisorySummary
	if err := json.Unmarshal(data, &s); err != nil {
		return AdvisorySummary{}, false
	}
	return s, true
}

// writeAdvisoryCache drops errors: failing to cache a courtesy lookup is not
// worth interrupting a run over.
func writeAdvisoryCache(s AdvisorySummary) {
	path, err := cache.AdvisoryCheckPath()
	if err != nil {
		return
	}
	data, err := json.Marshal(s)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0600)
}
