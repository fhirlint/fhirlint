package validator

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/fhirlint/fhirlint/internal/cache"
)

const (
	jarURL        = "https://github.com/hapifhir/org.hl7.fhir.core/releases/latest/download/validator_cli.jar"
	jarSourceRepo = "https://github.com/hapifhir/org.hl7.fhir.core"
)

var versionFromURL = regexp.MustCompile(`/releases/download/([^/]+)/`)

// EnsureJAR returns the path to the validator JAR.
// If override is non-empty (from --jar flag or FHIRLINT_JAR env var), that path is used directly.
// Otherwise the JAR is auto-downloaded to the cache directory on first use.
func EnsureJAR(override string) (string, error) {
	if override != "" {
		if _, err := os.Stat(override); err != nil {
			return "", fmt.Errorf("JAR not found at %q (set via --jar or FHIRLINT_JAR): %w", override, err)
		}
		return override, nil
	}

	jarPath, err := cache.JARPath()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(jarPath); os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "Downloading FHIR validator JAR (first run, ~250 MB)...")
		if err := downloadJAR(jarPath); err != nil {
			_ = os.Remove(jarPath)
			fmt.Fprintf(os.Stderr,
				"\nJAR download failed (URL: %s).\n"+
					"To work around firewall or proxy restrictions, download the JAR manually\n"+
					"from %s/releases and use:\n"+
					"  --jar /path/to/validator_cli.jar\n"+
					"  FHIRLINT_JAR=/path/to/validator_cli.jar fhirlint validate\n\n",
				jarURL, jarSourceRepo,
			)
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

// ValidatorVersion returns the cached JAR version, or "unknown" if not available.
func ValidatorVersion() string {
	versionPath, err := cache.ValidatorVersionPath()
	if err != nil {
		return "unknown"
	}
	data, err := os.ReadFile(versionPath) //nolint:gosec // known cache path
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(data))
}

func downloadJAR(dest string) error {
	resp, err := http.Get(jarURL) //nolint:gosec,noctx // known URL, no user input
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, jarURL)
	}

	// Extract version from the final URL after redirect (e.g. .../releases/download/6.9.7/...)
	if m := versionFromURL.FindStringSubmatch(resp.Request.URL.String()); len(m) == 2 {
		_ = saveValidatorVersion(m[1])
	}

	f, err := os.Create(dest) //nolint:gosec // intentional: dest is our own cache path
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(f, resp.Body)
	return err
}

func saveValidatorVersion(version string) error {
	versionPath, err := cache.ValidatorVersionPath()
	if err != nil {
		return err
	}
	return os.WriteFile(versionPath, []byte(version), 0600)
}

// JARSourceRepo returns the upstream repository URL for the validator JAR.
func JARSourceRepo() string {
	return jarSourceRepo
}

// JARReleasesURL returns the releases page for the validator JAR.
func JARReleasesURL() string {
	return filepath.ToSlash(jarSourceRepo + "/releases")
}
