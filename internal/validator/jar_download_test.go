// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package validator

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/fhirlint/fhirlint/internal/cache"
)

// The download and install path of the validator JAR against a local release
// server (#610): what EnsureJAR, UpdateJAR and EnsureSideJAR do with what the
// verification decides. The verification ladder itself is tested in
// signature_test.go.

// release is what the fake GitHub serves for one validator version.
type release struct {
	jar    []byte // nil: the download answers 404
	sig    string // "": no .asc (404)
	digest string // "": the release API records no digest
	status int    // non-zero: the download answers this instead
}

// fakeGitHub serves releases like github.com and its API do: /latest
// redirects to /releases/download/<latest>/, which serves the JAR.
type fakeGitHub struct {
	srv       *httptest.Server
	releases  map[string]release
	latest    string
	downloads atomic.Int32
}

func newFakeGitHub(t *testing.T, latest string, releases map[string]release) *fakeGitHub {
	t.Helper()
	g := &fakeGitHub{releases: releases, latest: latest}
	g.srv = httptest.NewServer(http.HandlerFunc(g.handle))
	t.Cleanup(g.srv.Close)

	origJAR, origSig, origAPI := jarURLFor, jarSignatureURLFor, releaseAPIURLFor
	jarURLFor = func(v string) string {
		if v == "" {
			return g.srv.URL + "/releases/latest/download/validator_cli.jar"
		}
		return g.srv.URL + "/releases/download/" + v + "/validator_cli.jar"
	}
	jarSignatureURLFor = func(v string) string { return g.srv.URL + "/releases/download/" + v + "/validator_cli.jar.asc" }
	releaseAPIURLFor = func(v string) string { return g.srv.URL + "/api/releases/tags/" + v }
	t.Cleanup(func() { jarURLFor, jarSignatureURLFor, releaseAPIURLFor = origJAR, origSig, origAPI })

	t.Setenv(cache.DirEnvVar, t.TempDir())
	return g
}

func (g *fakeGitHub) handle(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	switch {
	case p == "/releases/latest/download/validator_cli.jar":
		http.Redirect(w, r, "/releases/download/"+g.latest+"/validator_cli.jar", http.StatusFound)
	case strings.HasPrefix(p, "/api/releases/tags/"):
		rel, ok := g.releases[strings.TrimPrefix(p, "/api/releases/tags/")]
		if !ok || rel.digest == "" {
			_, _ = w.Write([]byte(`{"assets":[{"name":"validator_cli.jar"}]}`))
			return
		}
		_, _ = fmt.Fprintf(w, `{"assets":[{"name":"validator_cli.jar","digest":"sha256:%s"}]}`, rel.digest)
	case strings.HasSuffix(p, "/validator_cli.jar.asc"):
		rel, ok := g.releases[versionIn(p)]
		if !ok || rel.sig == "" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(rel.sig))
	case strings.HasSuffix(p, "/validator_cli.jar"):
		rel, ok := g.releases[versionIn(p)]
		switch {
		case !ok || rel.jar == nil:
			http.NotFound(w, r)
		case rel.status != 0:
			w.WriteHeader(rel.status)
		default:
			g.downloads.Add(1)
			_, _ = w.Write(rel.jar)
		}
	default:
		http.NotFound(w, r)
	}
}

func versionIn(path string) string {
	if m := versionFromURL.FindStringSubmatch(path); len(m) == 2 {
		return m[1]
	}
	return ""
}

// jarBytes is a real ZIP whose manifest names version, as a validator JAR has.
func jarBytes(t *testing.T, version string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("META-INF/MANIFEST.MF")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fmt.Fprintf(w, "Manifest-Version: 1.0\r\nClass-Path: org.hl7.fhir.validation/%s/x.jar\r\n", version)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sha(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }

func signed(t *testing.T, signer *openpgp.Entity, version string) release {
	t.Helper()
	jar := jarBytes(t, version)
	return release{jar: jar, sig: armoredSig(t, signer, jar), digest: sha(jar)}
}

func cachedJAR(t *testing.T) string {
	t.Helper()
	p, err := cache.JARPath()
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestEnsureJAR_DownloadsLatestVerifiesAndCaches(t *testing.T) {
	signer := testSigner(t)
	g := newFakeGitHub(t, "7.0.5", map[string]release{"7.0.5": signed(t, signer, "7.0.5")})

	path, err := EnsureJAR("", "")
	if err != nil {
		t.Fatal(err)
	}
	if path != cachedJAR(t) {
		t.Errorf("path = %s, want the cache path", path)
	}
	// The version comes from the redirect, as on github.com.
	if v := ValidatorVersion(); v != "7.0.5" {
		t.Errorf("version = %q, want 7.0.5 from the /latest redirect", v)
	}
	if method, verified, known := JARVerification(); !known || !verified || !strings.Contains(method, "signature") {
		t.Errorf("verification = %q verified=%v known=%v, want a PGP signature", method, verified, known)
	}

	// A second run uses the cache.
	if _, err := EnsureJAR("", ""); err != nil {
		t.Fatal(err)
	}
	if n := g.downloads.Load(); n != 1 {
		t.Errorf("downloads = %d, want 1: a cached JAR is not fetched again", n)
	}
}

func TestEnsureJAR_BadSignatureKeepsTheCachedJAR(t *testing.T) {
	signer := testSigner(t)
	good := signed(t, signer, "7.0.4")
	tampered := signed(t, signer, "7.0.5")
	tampered.jar = jarBytes(t, "7.0.5-tampered") // the signature no longer matches
	tampered.digest = sha(tampered.jar)          // a matching digest must not launder it
	newFakeGitHub(t, "7.0.4", map[string]release{"7.0.4": good, "7.0.5": tampered})

	if _, err := EnsureJAR("", "7.0.4"); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(cachedJAR(t))

	_, err := EnsureJAR("", "7.0.5")
	if !errors.Is(err, errVerification) || !errors.Is(err, ErrSignatureMismatch) {
		t.Fatalf("a bad signature must fail verification, got %v", err)
	}
	after, _ := os.ReadFile(cachedJAR(t))
	if !bytes.Equal(before, after) || ValidatorVersion() != "7.0.4" {
		t.Errorf("the cached 7.0.4 must stay in place and keep its version, now %s", ValidatorVersion())
	}
	if _, err := os.Stat(cachedJAR(t) + ".download"); !os.IsNotExist(err) {
		t.Error("the rejected download must not be left beside the cache")
	}
	if reason, _, ok := JARRejection("7.0.5"); !ok || reason != ErrSignatureMismatch.Error() {
		t.Errorf("the rejection must be recorded for the update notice: %q %v", reason, ok)
	}
}

func TestEnsureJAR_DigestIsTheFallbackWhenNoSignature(t *testing.T) {
	testSigner(t)
	jar := jarBytes(t, "6.6.0")
	newFakeGitHub(t, "6.6.0", map[string]release{"6.6.0": {jar: jar, digest: sha(jar)}})

	if _, err := EnsureJAR("", "6.6.0"); err != nil {
		t.Fatal(err)
	}
	if method, verified, _ := JARVerification(); !verified || !strings.Contains(method, "digest") {
		t.Errorf("verification = %q verified=%v, want the weaker digest method", method, verified)
	}
}

func TestEnsureJAR_DigestMismatchIsRejected(t *testing.T) {
	testSigner(t)
	jar := jarBytes(t, "6.6.0")
	newFakeGitHub(t, "6.6.0", map[string]release{"6.6.0": {jar: jar, digest: sha([]byte("something else"))}})

	_, err := EnsureJAR("", "6.6.0")
	if !errors.Is(err, errVerification) || !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("a wrong digest must fail verification, got %v", err)
	}
	if _, err := os.Stat(cachedJAR(t)); !os.IsNotExist(err) {
		t.Error("nothing may be installed after a failed verification")
	}
	if reason, _, ok := JARRejection("6.6.0"); !ok || reason != ErrDigestMismatch.Error() {
		t.Errorf("rejection = %q %v", reason, ok)
	}
}

func TestEnsureJAR_UnverifiableIsInstalledButMarked(t *testing.T) {
	testSigner(t)
	newFakeGitHub(t, "6.5.0", map[string]release{"6.5.0": {jar: jarBytes(t, "6.5.0")}})

	if _, err := EnsureJAR("", "6.5.0"); err != nil {
		t.Fatalf("a release without signature or digest installs (with a warning): %v", err)
	}
	if _, verified, known := JARVerification(); !known || verified {
		t.Errorf("verified=%v known=%v, want known and not verified", verified, known)
	}
}

func TestEnsureJAR_DownloadFailures(t *testing.T) {
	testSigner(t)
	newFakeGitHub(t, "7.0.5", map[string]release{"7.0.5": {jar: []byte("x"), status: http.StatusInternalServerError}})

	if _, err := EnsureJAR("", "9.9.9"); !errors.Is(err, errNoSuchRelease) || !strings.Contains(err.Error(), "9.9.9") {
		t.Errorf("a pinned version upstream doesn't have must say so, got %v", err)
	}
	if _, err := EnsureJAR("", "7.0.5"); err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("a server error must fail the download, got %v", err)
	}
	if _, err := os.Stat(cachedJAR(t)); !os.IsNotExist(err) {
		t.Error("nothing may be installed after a failed download")
	}
}

func TestEnsureJAR_CorruptedCacheIsReplaced(t *testing.T) {
	signer := testSigner(t)
	g := newFakeGitHub(t, "7.0.5", map[string]release{"7.0.5": signed(t, signer, "7.0.5")})
	p := cachedJAR(t)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("not a jar"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureJAR("", ""); err != nil {
		t.Fatal(err)
	}
	if ok, _ := isValidJAR(p); !ok || g.downloads.Load() != 1 {
		t.Errorf("a corrupted cache must be re-downloaded (valid=%v, downloads=%d)", ok, g.downloads.Load())
	}
}

func TestEnsureJAR_PinChangeDownloads(t *testing.T) {
	signer := testSigner(t)
	g := newFakeGitHub(t, "7.0.5", map[string]release{"7.0.4": signed(t, signer, "7.0.4"), "7.0.5": signed(t, signer, "7.0.5")})
	for _, v := range []string{"7.0.4", "7.0.4", "7.0.5"} {
		if _, err := EnsureJAR("", v); err != nil {
			t.Fatal(err)
		}
		if got := ValidatorVersion(); got != v {
			t.Errorf("after pinning %s the cache holds %s", v, got)
		}
	}
	if n := g.downloads.Load(); n != 2 {
		t.Errorf("downloads = %d, want 2: the same pin twice is one download", n)
	}
}

func TestEnsureJAR_Override(t *testing.T) {
	dir := t.TempDir()
	if _, err := EnsureJAR(filepath.Join(dir, "missing.jar"), ""); err == nil || !strings.Contains(err.Error(), "JAR not found") {
		t.Errorf("a missing --jar: %v", err)
	}
	bad := filepath.Join(dir, "bad.jar")
	if err := os.WriteFile(bad, []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureJAR(bad, ""); err == nil || !strings.Contains(err.Error(), "not a valid JAR") {
		t.Errorf("a --jar that isn't a JAR: %v", err)
	}
	good := filepath.Join(dir, "good.jar")
	if err := os.WriteFile(good, jarBytes(t, "7.0.5"), 0o600); err != nil {
		t.Fatal(err)
	}
	// An explicit JAR wins over a pin, with a warning rather than a download.
	if got, err := EnsureJAR(good, "7.0.4"); err != nil || got != good {
		t.Errorf("--jar with a different pin: %s, %v", got, err)
	}
}

func TestUpdateJAR_MovesThePin(t *testing.T) {
	signer := testSigner(t)
	newFakeGitHub(t, "7.0.5", map[string]release{"7.0.4": signed(t, signer, "7.0.4"), "7.0.5": signed(t, signer, "7.0.5")})
	if err := UpdateJAR("7.0.4"); err != nil {
		t.Fatal(err)
	}
	if v := ValidatorVersion(); v != "7.0.4" {
		t.Errorf("after update to 7.0.4: %s", v)
	}
	if err := UpdateJAR(""); err != nil {
		t.Fatal(err)
	}
	if v := ValidatorVersion(); v != "7.0.5" {
		t.Errorf("after update to latest: %s", v)
	}
	if err := UpdateJAR("9.9.9"); err == nil {
		t.Error("an update to a release that doesn't exist must fail")
	}
}

func TestEnsureSideJAR_LeavesTheDefaultAlone(t *testing.T) {
	signer := testSigner(t)
	g := newFakeGitHub(t, "7.0.5", map[string]release{"7.0.4": signed(t, signer, "7.0.4"), "7.0.5": signed(t, signer, "7.0.5")})
	if _, err := EnsureJAR("", "7.0.5"); err != nil {
		t.Fatal(err)
	}

	side, err := EnsureSideJAR("7.0.4")
	if err != nil {
		t.Fatal(err)
	}
	if side == cachedJAR(t) || versionFromJARManifest(side) != "7.0.4" {
		t.Errorf("side JAR %s must be 7.0.4 and separate from the default", side)
	}
	if v := ValidatorVersion(); v != "7.0.5" {
		t.Errorf("the default must stay 7.0.5, now %s", v)
	}
	if _, err := EnsureSideJAR("7.0.4"); err != nil || g.downloads.Load() != 2 {
		t.Errorf("a side JAR already there is reused (downloads=%d, err=%v)", g.downloads.Load(), err)
	}
	for _, bad := range []string{"latest", "7.0", "../7.0.4"} {
		if _, err := EnsureSideJAR(bad); err == nil || !strings.Contains(err.Error(), "not a validator release") {
			t.Errorf("EnsureSideJAR(%q): %v", bad, err)
		}
	}
}

func TestRejectionIsClearedOnceTheReleaseVerifies(t *testing.T) {
	signer := testSigner(t)
	rel := signed(t, signer, "7.0.5")
	broken := rel
	broken.sig = armoredSig(t, signer, []byte("other bytes"))
	g := newFakeGitHub(t, "7.0.5", map[string]release{"7.0.5": broken})

	if _, err := EnsureJAR("", "7.0.5"); err == nil {
		t.Fatal("want a rejection first")
	}
	if _, _, ok := JARRejection("7.0.5"); !ok {
		t.Fatal("rejection not recorded")
	}
	g.releases["7.0.5"] = rel // upstream fixed the .asc
	if _, err := EnsureJAR("", "7.0.5"); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := JARRejection("7.0.5"); ok {
		t.Error("a release that now verifies must no longer be reported as rejected")
	}
}
