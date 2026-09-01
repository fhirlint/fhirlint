package validator

import (
	"archive/zip"
	"errors"
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
	jarLatestURL  = "https://github.com/hapifhir/org.hl7.fhir.core/releases/latest/download/validator_cli.jar"
	jarSourceRepo = "https://github.com/hapifhir/org.hl7.fhir.core"
)

var versionFromURL = regexp.MustCompile(`/releases/download/([^/]+)/`)

// errNoSuchRelease marks a pinned version that upstream does not have, so the
// caller can skip the firewall/proxy advice that does not apply to a typo.
var errNoSuchRelease = errors.New("no validator release")

// errVerification marks a JAR that downloaded completely and then failed its
// signature or digest check. The distinction matters for what the caller tells
// the user: nothing about the network went wrong, so firewall advice is noise,
// and the one remedy that advice offers (--jar) would bypass the check that
// just fired.
var errVerification = errors.New("JAR verification failed")

// jarURLForVersion returns the download URL for a specific validator release,
// or the "latest" URL when version is empty.
func jarURLForVersion(version string) string {
	if version == "" {
		return jarLatestURL
	}
	return jarSourceRepo + "/releases/download/" + version + "/validator_cli.jar"
}

// EnsureJAR returns the path to the validator JAR.
// If override is non-empty (from --jar flag or FHIRLINT_JAR env var), that path is used directly.
// Otherwise the JAR is auto-downloaded to the cache directory on first use.
//
// A non-empty version pins the JAR to that upstream release. The cache holds one
// JAR at a time, so switching between pinned versions re-downloads; that keeps the
// cache path stable for the Docker image, which bakes the JAR in at build time.
func EnsureJAR(override, version string) (string, error) {
	if override != "" {
		if _, err := os.Stat(override); err != nil {
			return "", fmt.Errorf("JAR not found at %q (set via --jar or FHIRLINT_JAR): %w", override, err)
		}
		valid, readErr := isValidJAR(override)
		if readErr != nil {
			return "", fmt.Errorf("JAR at %q cannot be read (set via --jar or FHIRLINT_JAR): %w", override, readErr)
		}
		if !valid {
			return "", fmt.Errorf("JAR at %q appears to be corrupted or is not a valid JAR file", override)
		}
		// An explicit JAR path wins over a pin: the user is pointing at a
		// specific file, and silently re-downloading over it would be worse
		// than the mismatch. Say so rather than letting it pass unnoticed.
		if version != "" {
			if got := versionFromJARManifest(override); got != "" && got != version {
				fmt.Fprintf(os.Stderr,
					"warning: --jar points at validator %s but %s is pinned — using %s\n",
					got, version, got)
			}
		}
		return override, nil
	}

	jarPath, err := cache.JARPath()
	if err != nil {
		return "", err
	}

	_, statErr := os.Stat(jarPath)
	needsDownload := os.IsNotExist(statErr)

	// A stat that fails for any other reason — no permission on the cache
	// directory, a broken mount — is fatal here. Carrying on would return a path
	// we already know we cannot reach, and the run would fail several steps later
	// as a validator failure, blaming the validator for a problem with
	// our own cache (#316).
	if statErr != nil && !os.IsNotExist(statErr) {
		return "", fmt.Errorf(
			"cannot access the cached validator JAR at %s: %w"+
				" — check the permissions on the cache directory, or set %s to a readable location",
			jarPath, statErr, cache.DirEnvVar)
	}

	if statErr == nil {
		valid, readErr := isValidJAR(jarPath)
		switch {
		case readErr != nil:
			// Not "corrupted": the bytes may be perfectly fine, we just cannot
			// read them. Re-downloading still repairs it, because the replacement
			// is written as a fresh file.
			fmt.Fprintf(os.Stderr, "cached validator JAR cannot be read (%v) — re-downloading...\n", readErr)
			needsDownload = true
		case !valid:
			fmt.Fprintln(os.Stderr, "validator JAR appears to be corrupted — re-downloading...")
			needsDownload = true
		}
	}

	// A pin that does not match what is cached means fetching the pinned release.
	// The cached JAR stays in place until the replacement is on disk: a typo in
	// the version or an offline runner must not leave the user with no validator.
	if !needsDownload && version != "" && ValidatorVersion() != version {
		fmt.Fprintf(os.Stderr, "Cached validator is %s, %s is pinned — downloading...\n",
			ValidatorVersion(), version)
		needsDownload = true
	}

	if needsDownload {
		if version == "" {
			fmt.Fprintln(os.Stderr, "Downloading FHIR validator JAR (first run, ~250 MB)...")
		} else {
			fmt.Fprintf(os.Stderr, "Downloading FHIR validator JAR %s (~250 MB)...\n", version)
		}
		if err := downloadJAR(jarPath, version); err != nil {
			// No os.Remove here: downloadJAR only replaces jarPath once the new
			// file is complete and checksum-checked, so whatever was cached
			// before is still usable.
			switch {
			case errors.Is(err, errVerification):
				printVerificationAdvice(os.Stderr, version)
			case errors.Is(err, errNoSuchRelease):
				// A typo in a pinned version. The error names it already.
			default:
				fmt.Fprintf(os.Stderr,
					"\nJAR download failed (URL: %s).\n"+
						"To work around firewall or proxy restrictions, download the JAR manually\n"+
						"from %s/releases and use:\n"+
						"  --jar /path/to/validator_cli.jar\n"+
						"  FHIRLINT_JAR=/path/to/validator_cli.jar fhirlint validate\n\n",
					jarURLForVersion(version), jarSourceRepo,
				)
			}
			if errors.Is(err, errVerification) {
				// The download is not what went wrong; saying so would send the
				// reader looking in the wrong place.
				return "", err
			}
			return "", fmt.Errorf("downloading JAR: %w", err)
		}
		fmt.Fprintln(os.Stderr, "Download complete.")
	}
	return jarPath, nil
}

// isValidJAR checks that the file starts with the ZIP magic bytes (PK\x03\x04).
// JAR files are ZIP archives; a missing or wrong header indicates a corrupt or
// incomplete download.
//
// The second return separates "could not read the file at all" from "read it,
// and it is not a JAR". They call for different messages and, for the cached
// JAR, they are different problems: one is a permission or mount fault, the
// other a bad download (#316).
func isValidJAR(path string) (bool, error) {
	f, err := os.Open(path) //nolint:gosec // path is our own cache file or user-supplied --jar
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	magic := make([]byte, 4)
	n, err := f.Read(magic)
	if err != nil && n < 4 {
		// A truncated file reads short without being unreadable, so only a real
		// read failure counts as one; anything else is simply not a JAR.
		if n == 0 && !errors.Is(err, io.EOF) {
			return false, err
		}
		return false, nil
	}
	if n < 4 {
		return false, nil
	}
	return magic[0] == 0x50 && magic[1] == 0x4B && magic[2] == 0x03 && magic[3] == 0x04, nil
}

// UpdateJAR re-downloads the validator JAR. An empty version fetches the latest
// release; a non-empty one fetches exactly that release, which is how a pin is
// moved deliberately rather than by drift.
func UpdateJAR(version string) error {
	jarPath, err := cache.JARPath()
	if err != nil {
		return err
	}
	if version == "" {
		fmt.Fprintln(os.Stderr, "Updating FHIR validator JAR...")
	} else {
		fmt.Fprintf(os.Stderr, "Updating FHIR validator JAR to %s...\n", version)
	}
	if err := downloadJAR(jarPath, version); err != nil {
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

// EffectiveValidatorVersion reports the version a run will actually use: the
// pin when --validator-version is set, otherwise whatever is cached.
//
// ValidatorVersion alone is not enough for that question — it reports the
// cached JAR, which is not the one a pinned run executes.
func EffectiveValidatorVersion(pinned string) string {
	if pinned != "" {
		return pinned
	}
	return ValidatorVersion()
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

func downloadJAR(dest, pinned string) error {
	return downloadJARFrom(jarURLForVersion(pinned), dest, pinned)
}

// downloadJARFrom fetches the JAR at url into dest. pinned is the release the
// caller asked for, empty when tracking latest.
func downloadJARFrom(url, dest, pinned string) error {
	// GitHub redirects /releases/latest/download/ → /releases/download/VERSION/ → CDN.
	// The final URL is a CDN URL without the version; capture it from the intermediate redirect.
	//
	// A pinned URL is already /releases/download/VERSION/, so it redirects straight
	// to the CDN and the callback never sees a URL carrying the version — seed it
	// from the pin instead, or the checksum lookup below has nothing to work with.
	version := pinned
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
		if resp.StatusCode == http.StatusNotFound && pinned != "" {
			return fmt.Errorf(
				"%w %q (HTTP 404 from %s) — check the pinned version against %s/releases",
				errNoSuchRelease, pinned, url, jarSourceRepo)
		}
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	// Download beside the destination and move it into place only once the file
	// is complete and checksum-checked, so a failed or tampered download leaves
	// any previously cached JAR untouched and usable.
	tmp := dest + ".download"
	f, err := os.Create(tmp) //nolint:gosec // intentional: our own cache path
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
		_ = os.Remove(tmp) // no-op once the rename below has succeeded
	}()
	pw := &progressWriter{dest: f, out: os.Stderr, total: resp.ContentLength}
	if _, err = io.Copy(pw, resp.Body); err != nil {
		_, _ = fmt.Fprintln(os.Stderr)
		return err
	}
	_, _ = fmt.Fprintln(os.Stderr)
	if err := f.Close(); err != nil {
		return err
	}

	result, err := verifyJAR(tmp, version)
	if err != nil {
		return fmt.Errorf("%w for %s: %w", errVerification, describeVersion(version), err)
	}

	if err := os.Rename(tmp, dest); err != nil {
		return err
	}
	// Recorded only now: a version file written ahead of a failed download would
	// label the still-cached older JAR as the version that never arrived.
	if version != "" {
		_ = saveValidatorVersion(version)
	}
	_ = saveVerifyStatus(result.method)
	if !result.ok() {
		// Not fatal — releases before 6.6.0 carry neither a signature nor a
		// digest — but never silent: an attacker able to tamper with the
		// download can usually also make these requests fail (#260, #358).
		fmt.Fprintf(os.Stderr,
			"\nwarning: could not verify the downloaded validator JAR (%s)\n"+
				"  %s.\n"+
				"  The JAR is installed but unverified. Run 'fhirlint update' on a\n"+
				"  working connection to retry, and see SECURITY.md.\n", version, result.reason)
	}
	return nil
}

// saveVerifyStatus records how the cached JAR was verified, so `fhirlint
// version` can report it rather than leaving users to guess. The method is
// recorded, not just a boolean, because a signature and a digest are worth
// different amounts and a future format change must not read an old file as
// stronger than it was.
func saveVerifyStatus(method verifyMethod) error {
	path, err := cache.ChecksumStatusPath()
	if err != nil {
		return err
	}
	value := "unverified"
	if method != verifiedNot {
		value = "verified:" + string(method)
	}
	return os.WriteFile(path, []byte(value), 0600)
}

// JARVerification reports how the cached JAR was verified when it was
// downloaded. known is false when nothing was recorded — a JAR cached by an
// older fhirlint, or none at all.
func JARVerification() (method string, verified, known bool) {
	path, err := cache.ChecksumStatusPath()
	if err != nil {
		return "", false, false
	}
	data, err := os.ReadFile(path) //nolint:gosec // known cache path
	if err != nil {
		return "", false, false
	}
	switch value := strings.TrimSpace(string(data)); {
	case value == "unverified":
		return verifiedNot.describe(), false, true
	case value == "verified":
		// Written by a fhirlint that verified against the upstream .sha256 and
		// recorded only that it had. Trusted, but it cannot say how (#358).
		return verifiedLegacy.describe(), true, true
	case strings.HasPrefix(value, "verified:"):
		m := verifyMethod(strings.TrimPrefix(value, "verified:"))
		switch m {
		case verifiedSignature, verifiedDigest, verifiedLegacy:
			return m.describe(), true, true
		}
		// A method written by a newer fhirlint than this one. Do not claim a
		// verification whose meaning is unknown here.
		return "", false, false
	default:
		return "", false, false
	}
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

// describeVersion names a release for a message, for the case where the
// download tracked "latest" and never revealed which release that was.
func describeVersion(version string) string {
	if version == "" {
		return "the latest validator release"
	}
	return "validator " + version
}

// printVerificationAdvice explains a failed signature or digest check and says
// what to do about it.
//
// Deliberately does not offer --jar. It is the escape hatch the generic
// download advice reaches for, and here it would mean installing by hand the
// very file that just failed its check. Pinning a release that does verify
// keeps the check switched on, so that is what this suggests.
func printVerificationAdvice(w io.Writer, version string) {
	_, _ = fmt.Fprintf(w,
		"\nThe JAR downloaded completely and then failed verification, so this is not\n"+
			"a network or firewall problem. Two things produce it:\n"+
			"\n"+
			"  - upstream attached a JAR that does not match the signature it published\n"+
			"  - the download was altered between the release and this machine\n"+
			"\n"+
			"fhirlint cannot tell those apart, so it refuses the JAR either way and keeps\n"+
			"whatever was cached before.\n\n")

	if cached := ValidatorVersion(); cached != "unknown" && cached != version {
		_, _ = fmt.Fprintf(w,
			"Your cached validator %s is untouched and still in use. To stay on it and\n"+
				"stop retrying the failing release:\n"+
				"  --validator-version %s\n"+
				"  FHIRLINT_VALIDATOR_VERSION=%s\n\n", cached, cached, cached)
	} else {
		// No version named here on purpose. A number baked into shipped code
		// goes stale silently, which is how the .sha256 URL survived for months
		// in #358. The releases link below is always current.
		_, _ = fmt.Fprintf(w,
			"There is no cached JAR to fall back on. Pin an earlier release that verifies:\n"+
				"  --validator-version <release>\n"+
				"  FHIRLINT_VALIDATOR_VERSION=<release>\n\n")
	}

	_, _ = fmt.Fprintf(w,
		"Releases: %s/releases\n"+
			"If the release itself is at fault, it is worth reporting upstream — check\n"+
			"whether the .asc was uploaded before the .jar it is meant to sign.\n\n",
		jarSourceRepo)
}
