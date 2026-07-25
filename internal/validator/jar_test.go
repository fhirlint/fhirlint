package validator

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/cache"
)

func TestVersionFromURL_ExtractsVersion(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{
			url:  "https://github.com/hapifhir/org.hl7.fhir.core/releases/download/6.9.7/validator_cli.jar",
			want: "6.9.7",
		},
		{
			url:  "https://github.com/hapifhir/org.hl7.fhir.core/releases/download/7.0.0/validator_cli.jar",
			want: "7.0.0",
		},
		{
			url:  "https://github.com/hapifhir/org.hl7.fhir.core/releases/download/6.10.0-beta/validator_cli.jar",
			want: "6.10.0-beta",
		},
	}
	for _, tc := range cases {
		t.Run(tc.url, func(t *testing.T) {
			m := versionFromURL.FindStringSubmatch(tc.url)
			if len(m) != 2 {
				t.Fatalf("no match for URL %q", tc.url)
			}
			if m[1] != tc.want {
				t.Errorf("got %q, want %q", m[1], tc.want)
			}
		})
	}
}

func TestVersionFromURL_NoMatchForOtherURLs(t *testing.T) {
	urls := []string{
		"https://github.com/hapifhir/org.hl7.fhir.core/releases/latest",
		"https://example.com/file.jar",
		"",
	}
	for _, url := range urls {
		if m := versionFromURL.FindStringSubmatch(url); len(m) == 2 {
			t.Errorf("unexpected match %q for URL %q", m[1], url)
		}
	}
}

// writeJARWithManifest builds a minimal JAR carrying the class-path entry that
// versionFromJARManifest reads, so the fallback can be tested without depending
// on a real 185 MB download being present.
func writeJARWithManifest(t *testing.T, dir, version string) string {
	t.Helper()
	path := filepath.Join(dir, "validator_cli.jar")
	f, err := os.Create(path) //nolint:gosec // test temp dir
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("META-INF/MANIFEST.MF")
	if err != nil {
		t.Fatal(err)
	}
	manifest := "Manifest-Version: 1.0\r\nClass-Path: org.hl7.fhir.validation/" +
		version + "/org.hl7.fhir.validation-" + version + ".jar\r\n"
	if _, err := w.Write([]byte(manifest)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// With no version file present, the version must come from the JAR's own
// manifest — the case that keeps `fhirlint version` working for the Docker
// image, which bakes in a JAR without a separate version file.
func TestValidatorVersion_FallsBackToJARManifest(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(cache.DirEnvVar, dir)
	writeJARWithManifest(t, dir, "6.9.7")

	if got := ValidatorVersion(); got != "6.9.7" {
		t.Errorf("expected 6.9.7 from the JAR manifest, got %q", got)
	}

	// The fallback caches what it found, so the version file now exists.
	vp, err := cache.ValidatorVersionPath()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(vp) //nolint:gosec // test cache path
	if err != nil {
		t.Fatalf("fallback should have cached the version: %v", err)
	}
	if strings.TrimSpace(string(data)) != "6.9.7" {
		t.Errorf("cached version file says %q", strings.TrimSpace(string(data)))
	}
}

func TestValidatorVersion_UnknownWhenCacheEmpty(t *testing.T) {
	t.Setenv(cache.DirEnvVar, t.TempDir())
	if got := ValidatorVersion(); got != "unknown" {
		t.Errorf("expected \"unknown\" for an empty cache, got %q", got)
	}
}

func TestValidatorVersion_ReturnsCachedVersion(t *testing.T) {
	t.Setenv(cache.DirEnvVar, t.TempDir())
	vp, err := cache.ValidatorVersionPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(vp, []byte("9.9.9\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got := ValidatorVersion()
	if got != "9.9.9" {
		t.Errorf("expected '9.9.9', got %q", got)
	}
}

func writeJAR(t *testing.T, content []byte) string {
	t.Helper()
	f, err := os.CreateTemp("", "fake-validator-*.jar")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(content); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	t.Cleanup(func() { _ = os.Remove(f.Name()) })
	return f.Name()
}

var zipMagic = []byte{0x50, 0x4B, 0x03, 0x04}

func TestIsValidJAR_ValidZIP(t *testing.T) {
	path := writeJAR(t, append(zipMagic, []byte("rest of jar")...))
	if !isValidJAR(path) {
		t.Error("expected true for file with ZIP magic bytes")
	}
}

func TestIsValidJAR_EmptyFile(t *testing.T) {
	path := writeJAR(t, []byte{})
	if isValidJAR(path) {
		t.Error("expected false for empty file")
	}
}

func TestIsValidJAR_WrongBytes(t *testing.T) {
	path := writeJAR(t, []byte{0x00, 0x01, 0x02, 0x03})
	if isValidJAR(path) {
		t.Error("expected false for file with wrong magic bytes")
	}
}

func TestIsValidJAR_NonExistent(t *testing.T) {
	if isValidJAR("/nonexistent/file.jar") {
		t.Error("expected false for non-existent file")
	}
}

func TestEnsureJAR_Override_ExistingFile(t *testing.T) {
	path := writeJAR(t, append(zipMagic, []byte("fake jar content")...))

	got, err := EnsureJAR(path, "")
	if err != nil {
		t.Fatalf("EnsureJAR() error: %v", err)
	}
	if got != path {
		t.Errorf("expected %q, got %q", path, got)
	}
}

func TestEnsureJAR_Override_CorruptedFile_ReturnsError(t *testing.T) {
	path := writeJAR(t, []byte("not a jar"))

	_, err := EnsureJAR(path, "")
	if err == nil {
		t.Error("expected error for corrupted JAR, got nil")
	}
}

func sha256hex(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
}

func TestVerifyJARChecksum_MatchingHash(t *testing.T) {
	content := []byte("fake jar content")
	path := writeJAR(t, content)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, sha256hex(content))
	}))
	defer srv.Close()

	// Temporarily override the URL pattern via a wrapper function call
	verified, err := verifyJARChecksumURL(path, srv.URL)
	if err != nil {
		t.Errorf("expected nil for matching checksum, got: %v", err)
	}
	if !verified {
		t.Error("a matching checksum must report verified")
	}
}

func TestVerifyJARChecksum_MismatchHash(t *testing.T) {
	path := writeJAR(t, []byte("fake jar content"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "0000000000000000000000000000000000000000000000000000000000000000")
	}))
	defer srv.Close()

	verified, err := verifyJARChecksumURL(path, srv.URL)
	if err == nil {
		t.Error("expected error for mismatched checksum, got nil")
	}
	if verified {
		t.Error("a mismatch must never report verified")
	}
}

func TestVerifyJARChecksum_404_SkipsVerification(t *testing.T) {
	path := writeJAR(t, []byte("fake jar content"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	verified, err := verifyJARChecksumURL(path, srv.URL)
	if err != nil {
		t.Errorf("expected nil when checksum file not published, got: %v", err)
	}
	if verified {
		t.Error("a 404 must report unverified, not verified")
	}
}

func TestVerifyJARChecksum_GNUFormat(t *testing.T) {
	content := []byte("fake jar content")
	path := writeJAR(t, content)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// GNU coreutils sha256sum format: "hash  filename"
		fmt.Fprintf(w, "%s  validator_cli.jar\n", sha256hex(content))
	}))
	defer srv.Close()

	verified, err := verifyJARChecksumURL(path, srv.URL)
	if err != nil {
		t.Errorf("expected nil for GNU-format checksum, got: %v", err)
	}
	if !verified {
		t.Error("GNU-format checksum must report verified")
	}
}

func TestVerifyJARChecksum_EmptyVersion_Skips(t *testing.T) {
	path := writeJAR(t, []byte("content"))
	verified, err := verifyJARChecksum(path, "")
	if err != nil {
		t.Errorf("expected nil for empty version, got: %v", err)
	}
	if verified {
		t.Error("without a version there is no checksum to check, so it cannot be verified")
	}
}

func TestVerifyJARChecksum_EmptyChecksumFile_DoesNotPanic(t *testing.T) {
	// An empty or whitespace-only body used to panic on fields[0].
	for _, body := range []string{"", "   ", "\n\n"} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, body)
		}))
		path := writeJAR(t, []byte("content"))
		verified, err := verifyJARChecksumURL(path, srv.URL)
		srv.Close()
		if err != nil {
			t.Errorf("body %q: unexpected error %v", body, err)
		}
		if verified {
			t.Errorf("body %q: an unusable checksum file must report unverified", body)
		}
	}
}

func TestEnsureJAR_Override_MissingFile_ReturnsError(t *testing.T) {
	_, err := EnsureJAR("/nonexistent/validator_cli.jar", "")
	if err == nil {
		t.Error("expected error for non-existent JAR path, got nil")
	}
}

func TestJARSourceRepo_NotEmpty(t *testing.T) {
	if JARSourceRepo() == "" {
		t.Error("JARSourceRepo() should not be empty")
	}
}

func TestJARReleasesURL_ContainsReleases(t *testing.T) {
	url := JARReleasesURL()
	if url == "" {
		t.Error("JARReleasesURL() should not be empty")
	}
	if !containsStr(url, "releases") {
		t.Errorf("JARReleasesURL() %q should contain 'releases'", url)
	}
}

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{512, "0 KB"},
		{1024, "1 KB"},
		{10 * 1024 * 1024, "10.0 MB"},
		{250 * 1024 * 1024, "250.0 MB"},
		{2 * 1024 * 1024 * 1024, "2.00 GB"},
	}
	for _, tc := range cases {
		got := formatBytes(tc.n)
		if got != tc.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestProgressWriter_WritesDataAndProgress(t *testing.T) {
	var dest, out bytes.Buffer
	pw := &progressWriter{dest: &dest, out: &out, total: 1024}

	data := bytes.Repeat([]byte("x"), 512)
	n, err := pw.Write(data)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != 512 {
		t.Errorf("expected 512 bytes written, got %d", n)
	}
	if dest.Len() != 512 {
		t.Errorf("expected 512 bytes in dest, got %d", dest.Len())
	}
	progress := out.String()
	if !strings.Contains(progress, "50%") {
		t.Errorf("expected progress to contain '50%%', got %q", progress)
	}
}

func TestProgressWriter_NoTotal_ShowsBytesOnly(t *testing.T) {
	var dest, out bytes.Buffer
	pw := &progressWriter{dest: &dest, out: &out, total: -1}

	_, _ = pw.Write([]byte("hello"))
	if strings.Contains(out.String(), "%") {
		t.Errorf("expected no percentage without known total, got %q", out.String())
	}
}

// TestDownloadJARFrom_CapturesVersionFromRedirect verifies that the version is
// extracted from an intermediate redirect URL (e.g. GitHub's versioned URL)
// even when the final URL is a CDN URL that does not contain the version.
func TestDownloadJARFrom_CapturesVersionFromRedirect(t *testing.T) {
	jarContent := append(zipMagic, []byte("fake jar body")...)

	// Simulate: latest → versioned GitHub URL → CDN (final, no version in URL)
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(jarContent)
	}))
	defer cdn.Close()

	var versionedURL string
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/latest/download/validator_cli.jar":
			http.Redirect(w, r, versionedURL, http.StatusFound)
		case "/releases/download/6.9.7/validator_cli.jar":
			http.Redirect(w, r, cdn.URL+"/validator_cli.jar", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer gh.Close()

	versionedURL = gh.URL + "/releases/download/6.9.7/validator_cli.jar"

	tmp := t.TempDir()
	t.Setenv("FHIRLINT_CACHE_DIR", tmp)

	dest := tmp + "/validator_cli.jar"
	if err := downloadJARFrom(gh.URL+"/releases/latest/download/validator_cli.jar", dest, ""); err != nil {
		t.Fatalf("downloadJARFrom() error: %v", err)
	}

	vp, err := cache.ValidatorVersionPath()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(vp) //nolint:gosec // test path
	if err != nil {
		t.Fatalf("version file not written: %v", err)
	}
	got := strings.TrimSpace(string(data))
	if got != "6.9.7" {
		t.Errorf("expected version %q, got %q", "6.9.7", got)
	}
}

func TestJARURLForVersion_EmptyTracksLatest(t *testing.T) {
	got := jarURLForVersion("")
	if got != jarLatestURL {
		t.Errorf("expected the latest URL for an empty version, got %q", got)
	}
}

func TestJARURLForVersion_PinnedTargetsThatRelease(t *testing.T) {
	got := jarURLForVersion("6.9.12")
	want := "https://github.com/hapifhir/org.hl7.fhir.core/releases/download/6.9.12/validator_cli.jar"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
	// The version regex must be able to read the version back out of a pinned
	// URL, since that is what the checksum lookup keys on.
	if m := versionFromURL.FindStringSubmatch(got); len(m) != 2 || m[1] != "6.9.12" {
		t.Errorf("versionFromURL could not recover the version from %q", got)
	}
}
