package cache

import (
	"os"
	"path/filepath"
)

func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".fhirlint")
	return dir, os.MkdirAll(dir, 0750)
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

// ChecksumStatusPath is where the outcome of the JAR checksum verification is
// recorded, so it can be reported later without re-downloading.
func ChecksumStatusPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "validator_checksum_status.txt"), nil
}
