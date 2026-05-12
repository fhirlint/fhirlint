package validator

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestValidatorVersion_FallsBackToJARManifest(t *testing.T) {
	// Temporarily rename the version file so it is absent.
	vp, err := cache.ValidatorVersionPath()
	if err != nil {
		t.Fatal(err)
	}
	tmp := vp + ".bak"
	renamed := false
	if _, err := os.Stat(vp); err == nil {
		if err := os.Rename(vp, tmp); err == nil {
			renamed = true
			defer func() {
				_ = os.Remove(vp) // clean up the re-created file
				_ = os.Rename(tmp, vp)
			}()
		}
	}
	if !renamed {
		t.Skip("version file not present, skipping rename test")
	}

	// If the local JAR is present the fallback must return a non-empty, non-"unknown" version.
	jp, err := cache.JARPath()
	if err != nil {
		t.Skip("cannot determine JAR path")
	}
	if _, err := os.Stat(jp); os.IsNotExist(err) {
		t.Skip("local JAR not present, skipping manifest-fallback test")
	}

	got := ValidatorVersion()
	if got == "unknown" || got == "" {
		t.Errorf("expected a version from JAR manifest, got %q", got)
	}
}

func TestValidatorVersion_ReturnsCachedVersion(t *testing.T) {
	vp, err := cache.ValidatorVersionPath()
	if err != nil {
		t.Fatal(err)
	}
	// Save original if exists
	original, readErr := os.ReadFile(vp) //nolint:gosec // test cache path
	if readErr == nil {
		defer func() { _ = os.WriteFile(vp, original, 0600) }() //nolint:gosec // restoring test fixture
	} else {
		defer func() { _ = os.Remove(vp) }()
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

	got, err := EnsureJAR(path)
	if err != nil {
		t.Fatalf("EnsureJAR() error: %v", err)
	}
	if got != path {
		t.Errorf("expected %q, got %q", path, got)
	}
}

func TestEnsureJAR_Override_CorruptedFile_ReturnsError(t *testing.T) {
	path := writeJAR(t, []byte("not a jar"))

	_, err := EnsureJAR(path)
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
	if err := verifyJARChecksumURL(path, srv.URL); err != nil {
		t.Errorf("expected nil for matching checksum, got: %v", err)
	}
}

func TestVerifyJARChecksum_MismatchHash(t *testing.T) {
	path := writeJAR(t, []byte("fake jar content"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "0000000000000000000000000000000000000000000000000000000000000000")
	}))
	defer srv.Close()

	err := verifyJARChecksumURL(path, srv.URL)
	if err == nil {
		t.Error("expected error for mismatched checksum, got nil")
	}
}

func TestVerifyJARChecksum_404_SkipsVerification(t *testing.T) {
	path := writeJAR(t, []byte("fake jar content"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	if err := verifyJARChecksumURL(path, srv.URL); err != nil {
		t.Errorf("expected nil when checksum file not published, got: %v", err)
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

	if err := verifyJARChecksumURL(path, srv.URL); err != nil {
		t.Errorf("expected nil for GNU-format checksum, got: %v", err)
	}
}

func TestVerifyJARChecksum_EmptyVersion_Skips(t *testing.T) {
	path := writeJAR(t, []byte("content"))
	if err := verifyJARChecksum(path, ""); err != nil {
		t.Errorf("expected nil for empty version, got: %v", err)
	}
}

func TestEnsureJAR_Override_MissingFile_ReturnsError(t *testing.T) {
	_, err := EnsureJAR("/nonexistent/validator_cli.jar")
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
