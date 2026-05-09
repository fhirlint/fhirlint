package validator

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/fhirlint/fhirlint/internal/cache"
)

const jarURL = "https://github.com/hapifhir/org.hl7.fhir.core/releases/latest/download/validator_cli.jar"

func EnsureJAR() (string, error) {
	jarPath, err := cache.JARPath()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(jarPath); os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "Downloading FHIR validator JAR (first run, ~250 MB)...")
		if err := downloadJAR(jarPath); err != nil {
			_ = os.Remove(jarPath)
			return "", fmt.Errorf("downloading JAR: %w", err)
		}
		fmt.Fprintln(os.Stderr, "Download complete.")
	}
	return jarPath, nil
}

func UpdateJAR() error {
	jarPath, err := cache.JARPath()
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Updating FHIR validator JAR...")
	if err := downloadJAR(jarPath); err != nil {
		return fmt.Errorf("updating JAR: %w", err)
	}
	fmt.Fprintln(os.Stderr, "Update complete.")
	return nil
}

func downloadJAR(dest string) error {
	resp, err := http.Get(jarURL) //nolint:noctx
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, jarURL)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(f, resp.Body)
	return err
}
