package validator

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/fhirlint/fhirlint/internal/cache"
)

const (
	checkInterval  = 24 * time.Hour
	checkTimeout   = 3 * time.Second
	releasesAPIURL = "https://api.github.com/repos/hapifhir/org.hl7.fhir.core/releases/latest"
)

type updateCheckCache struct {
	LastChecked   time.Time `json:"last_checked"`
	LatestVersion string    `json:"latest_version"`
}

// CheckForUpdate returns a newer version string if one is available, or empty string if not.
// The result is cached for 24 hours to avoid a network call on every run.
func CheckForUpdate() string {
	current := ValidatorVersion()
	if current == "unknown" {
		return ""
	}

	cached, err := readUpdateCheckCache()
	if err == nil && time.Since(cached.LastChecked) < checkInterval {
		if cached.LatestVersion != current {
			return cached.LatestVersion
		}
		return ""
	}

	latest, err := fetchLatestVersion()
	if err != nil {
		return "" // network unavailable — fail silently
	}

	_ = writeUpdateCheckCache(updateCheckCache{
		LastChecked:   time.Now(),
		LatestVersion: latest,
	})

	if latest != current {
		return latest
	}
	return ""
}

func fetchLatestVersion() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesAPIURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

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

func readUpdateCheckCache() (updateCheckCache, error) {
	path, err := cache.UpdateCheckPath()
	if err != nil {
		return updateCheckCache{}, err
	}
	data, err := os.ReadFile(path) //nolint:gosec // known cache path
	if err != nil {
		return updateCheckCache{}, err
	}
	var c updateCheckCache
	return c, json.Unmarshal(data, &c)
}

func writeUpdateCheckCache(c updateCheckCache) error {
	path, err := cache.UpdateCheckPath()
	if err != nil {
		return err
	}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
