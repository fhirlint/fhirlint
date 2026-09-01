package validator

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/cache"
)

func TestDescribeVersion(t *testing.T) {
	if got := describeVersion(""); got != "the latest validator release" {
		t.Errorf("describeVersion(\"\") = %q", got)
	}
	if got := describeVersion("6.10.3"); got != "validator 6.10.3" {
		t.Errorf("describeVersion(6.10.3) = %q", got)
	}
}

// The advice must not send someone around the check that just fired. --jar
// installs a JAR unverified, which is the wrong answer to a bad signature.
func TestVerificationAdvice_DoesNotSuggestBypassingTheCheck(t *testing.T) {
	t.Setenv(cache.DirEnvVar, t.TempDir())
	var b bytes.Buffer
	printVerificationAdvice(&b, "6.10.3")

	for _, forbidden := range []string{"--jar ", "FHIRLINT_JAR"} {
		if strings.Contains(b.String(), forbidden) {
			t.Errorf("advice offers %q, which installs an unverified JAR:\n%s", forbidden, b.String())
		}
	}
	// Nor should it blame the network, which demonstrably worked.
	if strings.Contains(strings.ToLower(b.String()), "firewall or proxy restrictions") {
		t.Errorf("advice blames the network for a completed download:\n%s", b.String())
	}
}

func TestVerificationAdvice_NoCachedJAR(t *testing.T) {
	t.Setenv(cache.DirEnvVar, t.TempDir())
	var b bytes.Buffer
	printVerificationAdvice(&b, "6.10.3")
	out := b.String()

	if !strings.Contains(out, "no cached JAR to fall back on") {
		t.Errorf("want the no-fallback wording, got:\n%s", out)
	}
	if !strings.Contains(out, "--validator-version") {
		t.Errorf("want a pinning suggestion, got:\n%s", out)
	}
}

// With a working JAR already cached, the advice should name it rather than
// suggest a version the user has never heard of.
func TestVerificationAdvice_NamesTheCachedVersion(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(cache.DirEnvVar, dir)
	if err := os.WriteFile(filepath.Join(dir, "validator_version.txt"), []byte("6.10.2"), 0600); err != nil {
		t.Fatal(err)
	}

	var b bytes.Buffer
	printVerificationAdvice(&b, "6.10.3")
	out := b.String()

	if !strings.Contains(out, "cached validator 6.10.2 is untouched") {
		t.Errorf("want the cached version named, got:\n%s", out)
	}
	if strings.Contains(out, "no cached JAR") {
		t.Errorf("claimed there is no cached JAR while one exists:\n%s", out)
	}
	if !strings.Contains(out, "--validator-version 6.10.2") {
		t.Errorf("want the cached version offered as the pin, got:\n%s", out)
	}
}

// A failed verification must be distinguishable from every other download
// failure, or the caller cannot suppress the advice that does not apply.
func TestVerificationError_IsIdentifiable(t *testing.T) {
	wrapped := errors.Join(errVerification, errors.New("RSA verification failure"))
	if !errors.Is(wrapped, errVerification) {
		t.Error("errVerification must survive wrapping")
	}
	if errors.Is(wrapped, errNoSuchRelease) {
		t.Error("a verification failure must not read as a missing release")
	}
}
