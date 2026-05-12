package validator

import (
	"crypto/sha256"
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
		if !isValidJAR(override) {
			return "", fmt.Errorf("JAR at %q appears to be corrupted or is not a valid JAR file", override)
		}
		return override, nil
	}

	jarPath, err := cache.JARPath()
	if err != nil {
		return "", err
	}

	_, statErr := os.Stat(jarPath)
	if statErr == nil && !isValidJAR(jarPath) {
		fmt.Fprintln(os.Stderr, "validator JAR appears to be corrupted — re-downloading...")
		_ = os.Remove(jarPath)
		statErr = os.ErrNotExist
	}

	if os.IsNotExist(statErr) {
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

// isValidJAR checks that the file starts with the ZIP magic bytes (PK\x03\x04).
// JAR files are ZIP archives; a missing or wrong header indicates a corrupt/incomplete download.
func isValidJAR(path string) bool {
	f, err := os.Open(path) //nolint:gosec // path is our own cache file or user-supplied --jar
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	magic := make([]byte, 4)
	n, err := f.Read(magic)
	if err != nil || n < 4 {
		return false
	}
	return magic[0] == 0x50 && magic[1] == 0x4B && magic[2] == 0x03 && magic[3] == 0x04
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
	var version string
	if m := versionFromURL.FindStringSubmatch(resp.Request.URL.String()); len(m) == 2 {
		version = m[1]
		_ = saveValidatorVersion(version)
	}

	f, err := os.Create(dest) //nolint:gosec // intentional: dest is our own cache path
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if _, err = io.Copy(f, resp.Body); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	if err := verifyJARChecksum(dest, version); err != nil {
		_ = os.Remove(dest)
		return fmt.Errorf(
			"JAR checksum mismatch — download may be corrupted or tampered with.\n"+
				"File deleted. Re-run fhirlint to attempt a fresh download.\n"+
				"Details: %w", err)
	}
	return nil
}

// verifyJARChecksum fetches the .sha256 file for the given release version and compares it
// against the downloaded JAR. If no checksum file is published, verification is skipped silently.
func verifyJARChecksum(jarPath, version string) error {
	if version == "" {
		return nil
	}
	url := fmt.Sprintf(
		"https://github.com/hapifhir/org.hl7.fhir.core/releases/download/%s/validator_cli.jar.sha256",
		version,
	)
	return verifyJARChecksumURL(jarPath, url)
}

func verifyJARChecksumURL(jarPath, checksumURL string) error {
	resp, err := http.Get(checksumURL) //nolint:gosec,noctx // known URL pattern, version from trusted source
	if err != nil {
		return nil // network error fetching checksum — skip silently
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil // no checksum file published for this release — skip
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	// SHA256 files may be "hash  filename" (GNU coreutils format) or just "hash"
	expected := strings.ToLower(strings.Fields(strings.TrimSpace(string(body)))[0])

	f, err := os.Open(jarPath) //nolint:gosec // our own cache path
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actual := fmt.Sprintf("%x", h.Sum(nil))

	if actual != expected {
		return fmt.Errorf("expected %s, got %s", expected, actual)
	}
	return nil
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
