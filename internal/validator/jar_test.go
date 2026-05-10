package validator

import (
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

func TestValidatorVersion_ReturnsUnknownWhenNotCached(t *testing.T) {
	// Temporarily rename the version file if it exists
	vp, err := cache.ValidatorVersionPath()
	if err != nil {
		t.Fatal(err)
	}
	tmp := vp + ".bak"
	renamed := false
	if _, err := os.Stat(vp); err == nil {
		if err := os.Rename(vp, tmp); err == nil {
			renamed = true
			defer func() { _ = os.Rename(tmp, vp) }()
		}
	}
	if !renamed {
		t.Skip("version file not present, skipping rename test")
	}

	got := ValidatorVersion()
	if got != "unknown" {
		t.Errorf("expected 'unknown' when version file missing, got %q", got)
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

func TestEnsureJAR_Override_ExistingFile(t *testing.T) {
	f, err := os.CreateTemp("", "fake-validator-*.jar")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	defer func() { _ = os.Remove(f.Name()) }()

	got, err := EnsureJAR(f.Name())
	if err != nil {
		t.Fatalf("EnsureJAR() error: %v", err)
	}
	if got != f.Name() {
		t.Errorf("expected %q, got %q", f.Name(), got)
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
