package validator

import (
	"archive/zip"
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

// ValidatorVersion returns the validator JAR version.
// It reads the cached version file written at download time. If that file is
// absent (e.g. Docker images where the JAR is bundled without a separate
// version file, or before the first validate run), it falls back to reading
// the version from the JAR's own MANIFEST.MF class-path entries.
func ValidatorVersion() string {
	versionPath, err := cache.ValidatorVersionPath()
	if err == nil {
		if data, err := os.ReadFile(versionPath); err == nil { //nolint:gosec // known cache path
			if v := strings.TrimSpace(string(data)); v != "" {
				return v
			}
		}
	}
	// Fallback: read version from the bundled JAR manifest.
	if jarPath, err := cache.JARPath(); err == nil {
		if v := versionFromJARManifest(jarPath); v != "" {
			_ = saveValidatorVersion(v) // cache for next time
			return v
		}
	}
	return "unknown"
}

// versionFromJARManifest extracts the validator version from the JAR's
// MANIFEST.MF class-path, which contains entries like
// "org.hl7.fhir.validation/6.9.7/org.hl7.fhir.validation-6.9.7.jar".
var versionInClassPath = regexp.MustCompile(`org\.hl7\.fhir\.validation/([0-9]+\.[0-9]+\.[0-9]+)/`)

func versionFromJARManifest(jarPath string) string {
	r, err := zip.OpenReader(jarPath)
	if err != nil {
		return ""
	}
	defer func() { _ = r.Close() }()
	for _, f := range r.File {
		if f.Name != "META-INF/MANIFEST.MF" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return ""
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return ""
		}
		if m := versionInClassPath.FindSubmatch(data); len(m) == 2 {
			return string(m[1])
		}
		return ""
	}
	return ""
}

// progressWriter wraps a destination writer and prints download progress to out.
type progressWriter struct {
	dest    io.Writer
	out     io.Writer
	total   int64
	written int64
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.dest.Write(b)
	p.written += int64(n)
	if p.total > 0 {
		pct := int(100 * p.written / p.total)
		_, _ = fmt.Fprintf(p.out, "\rDownloading validator_cli.jar (%s / %s)... %d%%",
			formatBytes(p.written), formatBytes(p.total), pct)
	} else {
		_, _ = fmt.Fprintf(p.out, "\rDownloading validator_cli.jar (%s)...", formatBytes(p.written))
	}
	return n, err
}

func formatBytes(n int64) string {
	const (
		mb = 1 << 20
		gb = 1 << 30
	)
	switch {
	case n >= gb:
		return fmt.Sprintf("%.2f GB", float64(n)/float64(gb))
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mb))
	default:
		return fmt.Sprintf("%d KB", n/1024)
	}
}

func downloadJAR(dest string) error {
	return downloadJARFrom(jarURL, dest)
}

func downloadJARFrom(url, dest string) error {
	// GitHub redirects /releases/latest/download/ → /releases/download/VERSION/ → CDN.
	// The final URL is a CDN URL without the version; capture it from the intermediate redirect.
	var version string
	client := &http.Client{
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			if m := versionFromURL.FindStringSubmatch(req.URL.String()); len(m) == 2 {
				version = m[1]
			}
			return nil
		},
	}
	resp, err := client.Get(url) //nolint:gosec,noctx // known URL, no user input
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	if version != "" {
		_ = saveValidatorVersion(version)
	}

	f, err := os.Create(dest) //nolint:gosec // intentional: dest is our own cache path
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	pw := &progressWriter{dest: f, out: os.Stderr, total: resp.ContentLength}
	if _, err = io.Copy(pw, resp.Body); err != nil {
		_, _ = fmt.Fprintln(os.Stderr)
		return err
	}
	_, _ = fmt.Fprintln(os.Stderr)
	if err := f.Close(); err != nil {
		return err
	}

	verified, err := verifyJARChecksum(dest, version)
	if err != nil {
		_ = os.Remove(dest)
		return fmt.Errorf(
			"JAR checksum mismatch — download may be corrupted or tampered with.\n"+
				"File deleted. Re-run fhirlint to attempt a fresh download.\n"+
				"Details: %w", err)
	}
	_ = saveChecksumStatus(verified)
	if !verified {
		// Not fatal — upstream does not always publish a checksum — but never
		// silent: an attacker able to tamper with the download can usually also
		// make the checksum request fail (#260).
		fmt.Fprintf(os.Stderr,
			"\nwarning: could not verify the checksum of the downloaded validator JAR\n"+
				"  no checksum published for %s, or it could not be fetched.\n"+
				"  The JAR is installed but unverified. Run 'fhirlint update' on a\n"+
				"  working connection to retry, and see SECURITY.md.\n", version)
	}
	return nil
}

// saveChecksumStatus records whether the cached JAR was checksum-verified, so
// `fhirlint version` can report it rather than leaving users to guess.
func saveChecksumStatus(verified bool) error {
	path, err := cache.ChecksumStatusPath()
	if err != nil {
		return err
	}
	value := "unverified"
	if verified {
		value = "verified"
	}
	return os.WriteFile(path, []byte(value), 0600)
}

// JARChecksumVerified reports whether the cached JAR passed checksum
// verification when it was downloaded. The second return is false when nothing
// was recorded — a JAR cached by an older version, or none at all.
func JARChecksumVerified() (verified, known bool) {
	path, err := cache.ChecksumStatusPath()
	if err != nil {
		return false, false
	}
	data, err := os.ReadFile(path) //nolint:gosec // known cache path
	if err != nil {
		return false, false
	}
	switch strings.TrimSpace(string(data)) {
	case "verified":
		return true, true
	case "unverified":
		return false, true
	default:
		return false, false
	}
}

// verifyJARChecksum fetches the .sha256 file for the given release version and
// compares it against the downloaded JAR.
//
// It returns verified=false with a nil error when the checksum could not be
// obtained at all — an unreachable or unpublished checksum file. That case is
// not fatal, because upstream does not always publish one, but it must never
// pass unnoticed: anyone able to tamper with the JAR download can usually also
// make the checksum request fail. Callers are expected to surface it (#260).
//
// A genuine mismatch is returned as an error and must be treated as fatal.
func verifyJARChecksum(jarPath, version string) (verified bool, err error) {
	if version == "" {
		// Without a version there is no checksum URL to fetch.
		return false, nil
	}
	url := fmt.Sprintf(
		"https://github.com/hapifhir/org.hl7.fhir.core/releases/download/%s/validator_cli.jar.sha256",
		version,
	)
	return verifyJARChecksumURL(jarPath, url)
}

func verifyJARChecksumURL(jarPath, checksumURL string) (bool, error) {
	resp, err := http.Get(checksumURL) //nolint:gosec,noctx // known URL pattern, version from trusted source
	if err != nil {
		return false, nil // network error fetching checksum
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return false, nil // no checksum file published for this release
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, nil
	}
	// SHA256 files may be "hash  filename" (GNU coreutils format) or just "hash".
	fields := strings.Fields(strings.TrimSpace(string(body)))
	if len(fields) == 0 {
		// An empty or whitespace-only checksum file. Indexing here used to panic.
		return false, nil
	}
	expected := strings.ToLower(fields[0])

	f, err := os.Open(jarPath) //nolint:gosec // our own cache path
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	actual := fmt.Sprintf("%x", h.Sum(nil))

	if actual != expected {
		return false, fmt.Errorf("expected %s, got %s", expected, actual)
	}
	return true, nil
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
