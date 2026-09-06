package cache

import (
	"os"
	"path/filepath"
	"strings"
)

// DirEnvVar overrides where fhirlint keeps the validator JAR, the version file
// and the result cache. Useful for containers with a read-only home, CI runners
// that cache a mounted volume, and tests that must not touch the real cache.
const DirEnvVar = "FHIRLINT_CACHE_DIR"

func Dir() (string, error) {
	if custom := strings.TrimSpace(os.Getenv(DirEnvVar)); custom != "" {
		//nolint:gosec // G703: the path comes from the user's own environment and
		// names their own cache directory — choosing it is the whole point of the
		// variable, and it crosses no privilege boundary.
		return custom, os.MkdirAll(custom, 0750)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".fhirlint")
	return dir, os.MkdirAll(dir, 0750)
}

// ResultCacheDir is where --cache stores per-file validation results.
func ResultCacheDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "result-cache"), nil
}

func JARPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "validator_cli.jar"), nil
}

func ValidatorVersionPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "validator_version.txt"), nil
}

func UpdateCheckPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "update_check.json"), nil
}

// AdvisoryCheckPath is where the last advisory lookup is remembered, so the
// notice printed after every run does not fetch the advisory list each time.
func AdvisoryCheckPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "advisory_check.json"), nil
}

// ChecksumStatusPath is where the outcome of the JAR checksum verification is
// recorded, so it can be reported later without re-downloading.
func ChecksumStatusPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "validator_checksum_status.txt"), nil
}
