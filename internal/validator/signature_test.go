package validator

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/packet"

	"github.com/fhirlint/fhirlint/internal/cache"
)

// --- the pinned key itself -------------------------------------------------

func TestPinnedKey_ParsesAndHasExpectedFingerprint(t *testing.T) {
	ring, err := hl7Keyring()
	if err != nil {
		t.Fatalf("the embedded HL7 signing key must parse: %v", err)
	}
	got := strings.ToUpper(hex.EncodeToString(ring[0].PrimaryKey.Fingerprint))
	if got != hl7SigningKeyFingerprint {
		t.Errorf("fingerprint = %s, want %s", got, hl7SigningKeyFingerprint)
	}
}

// A pin that has already lapsed silently stops verifying anything, which is the
// failure this whole file exists to prevent. Fail the build instead.
func TestPinnedKey_NotExpired(t *testing.T) {
	expiry, ok := PinnedSigningKeyExpiry()
	if !ok {
		return // a key without an expiry cannot lapse
	}
	if time.Now().After(expiry) {
		t.Fatalf("the pinned HL7 signing key expired on %s — rotate the pin",
			expiry.Format("2006-01-02"))
	}
}

func TestPinnedKey_CanSign(t *testing.T) {
	ring, err := hl7Keyring()
	if err != nil {
		t.Fatalf("keyring: %v", err)
	}
	if _, ok := ring[0].SigningKey(time.Now()); !ok {
		t.Error("the pinned key carries no signing-capable key, so it can never verify a release")
	}
}

// --- helpers ---------------------------------------------------------------

// testSigner generates a throwaway key and installs it as the accepted keyring
// for the duration of the test.
func testSigner(t *testing.T) *openpgp.Entity {
	t.Helper()
	// Curve25519 by default: fast enough to generate per test.
	e, err := openpgp.NewEntity("Test Signer", "", "test@example.invalid", nil)
	if err != nil {
		t.Fatalf("generating a test key: %v", err)
	}
	orig := signatureKeyring
	signatureKeyring = func() (openpgp.EntityList, error) { return openpgp.EntityList{e}, nil }
	t.Cleanup(func() { signatureKeyring = orig })
	return e
}

func armoredSig(t *testing.T, signer *openpgp.Entity, content []byte) string {
	t.Helper()
	var buf bytes.Buffer
	if err := openpgp.ArmoredDetachSign(&buf, signer, bytes.NewReader(content), nil); err != nil {
		t.Fatalf("signing: %v", err)
	}
	return buf.String()
}

func writeTempJAR(t *testing.T, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "validator_cli.jar")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func serve(t *testing.T, status int, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func digestJSON(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf(`{"assets":[{"name":"validator_cli.jar","digest":"sha256:%x"}]}`, sum)
}

// --- signature verification ------------------------------------------------

func TestVerifySignature_Good(t *testing.T) {
	content := []byte("fake jar content")
	signer := testSigner(t)
	path := writeTempJAR(t, content)

	got, err := verifyJARSignatureURL(path, serve(t, 200, armoredSig(t, signer, content)))
	if err != nil {
		t.Fatalf("a good signature must not error: %v", err)
	}
	if got.method != verifiedSignature {
		t.Errorf("method = %q, want %q (reason: %s)", got.method, verifiedSignature, got.reason)
	}
}

// The case the whole feature is for: the signature is real but the bytes are not.
func TestVerifySignature_TamperedJAR_IsFatal(t *testing.T) {
	signer := testSigner(t)
	sig := armoredSig(t, signer, []byte("the original jar"))
	path := writeTempJAR(t, []byte("a substituted jar"))

	got, err := verifyJARSignatureURL(path, serve(t, 200, sig))
	if err == nil {
		t.Fatal("a signature that does not verify must be a fatal error, got nil")
	}
	if got.ok() {
		t.Error("a failed verification must never report verified")
	}
}

// An unknown signer reads as a key rotation, which must not brick the download.
func TestVerifySignature_UnknownIssuer_IsNotFatal(t *testing.T) {
	content := []byte("fake jar content")
	other, err := openpgp.NewEntity("Someone Else", "", "other@example.invalid", nil)
	if err != nil {
		t.Fatal(err)
	}
	testSigner(t) // installs a *different* key as the accepted one
	path := writeTempJAR(t, content)

	got, err := verifyJARSignatureURL(path, serve(t, 200, armoredSig(t, other, content)))
	if err != nil {
		t.Fatalf("an unknown issuer must not be fatal: %v", err)
	}
	if got.ok() {
		t.Error("a signature from an unpinned key must not count as verified")
	}
	if !strings.Contains(got.reason, "rotated") {
		t.Errorf("reason should explain the rotation possibility, got %q", got.reason)
	}
}

func TestVerifySignature_404_IsNotFatal(t *testing.T) {
	testSigner(t)
	path := writeTempJAR(t, []byte("content"))

	got, err := verifyJARSignatureURL(path, serve(t, 404, ""))
	if err != nil {
		t.Fatalf("a release without a signature must not be fatal: %v", err)
	}
	if got.ok() {
		t.Error("a 404 must report unverified")
	}
}

func TestVerifySignature_GarbageBody_IsNotFatal(t *testing.T) {
	testSigner(t)
	path := writeTempJAR(t, []byte("content"))

	got, err := verifyJARSignatureURL(path, serve(t, 200, "not a pgp signature at all"))
	if err != nil {
		t.Fatalf("an unparseable signature file must not be fatal: %v", err)
	}
	if got.ok() {
		t.Error("an unparseable signature must report unverified")
	}
}

func TestVerifySignature_ExpiredKey_IsNotFatalAndSaysSo(t *testing.T) {
	content := []byte("fake jar content")
	// Made two days ago with a one-day life: valid when it signed, expired now.
	past := time.Now().Add(-48 * time.Hour)
	pastCfg := &packet.Config{
		KeyLifetimeSecs: 24 * 60 * 60,
		Time:            func() time.Time { return past },
	}
	e, err := openpgp.NewEntity("Expired", "", "expired@example.invalid", pastCfg)
	if err != nil {
		t.Fatal(err)
	}
	var sigBuf bytes.Buffer
	if err := openpgp.ArmoredDetachSign(&sigBuf, e, bytes.NewReader(content), pastCfg); err != nil {
		t.Fatalf("signing while the key was still valid: %v", err)
	}
	sig := sigBuf.String()
	orig := signatureKeyring
	signatureKeyring = func() (openpgp.EntityList, error) { return openpgp.EntityList{e}, nil }
	defer func() { signatureKeyring = orig }()

	got, err := verifyJARSignatureURL(writeTempJAR(t, content), serve(t, 200, sig))
	if err != nil {
		t.Fatalf("an expired pin must not be fatal: %v", err)
	}
	if got.ok() {
		t.Error("an expired pin must not report verified")
	}
	if !strings.Contains(got.reason, "expired") {
		t.Errorf("reason should name the expiry, got %q", got.reason)
	}
}

// --- digest verification ---------------------------------------------------

func TestVerifyDigest_Match(t *testing.T) {
	content := []byte("fake jar content")
	got, err := verifyJARDigestURL(writeTempJAR(t, content), serve(t, 200, digestJSON(content)))
	if err != nil {
		t.Fatalf("a matching digest must not error: %v", err)
	}
	if got.method != verifiedDigest {
		t.Errorf("method = %q, want %q (reason: %s)", got.method, verifiedDigest, got.reason)
	}
}

func TestVerifyDigest_Mismatch_IsFatal(t *testing.T) {
	body := digestJSON([]byte("the original jar"))
	got, err := verifyJARDigestURL(writeTempJAR(t, []byte("a substituted jar")), serve(t, 200, body))
	if err == nil {
		t.Fatal("a digest mismatch must be a fatal error, got nil")
	}
	if got.ok() {
		t.Error("a mismatch must never report verified")
	}
}

// Assets uploaded before GitHub recorded digests have no digest field.
func TestVerifyDigest_NoDigestRecorded_IsNotFatal(t *testing.T) {
	body := `{"assets":[{"name":"validator_cli.jar","digest":""}]}`
	got, err := verifyJARDigestURL(writeTempJAR(t, []byte("content")), serve(t, 200, body))
	if err != nil {
		t.Fatalf("an asset without a digest must not be fatal: %v", err)
	}
	if got.ok() {
		t.Error("a missing digest must report unverified")
	}
}

func TestVerifyDigest_MalformedOrMissing_IsNotFatal(t *testing.T) {
	for name, body := range map[string]string{
		"not json":       "<html>rate limited</html>",
		"no assets":      `{"assets":[]}`,
		"other asset":    `{"assets":[{"name":"something-else.zip","digest":"sha256:ab"}]}`,
		"empty document": `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := verifyJARDigestURL(writeTempJAR(t, []byte("content")), serve(t, 200, body))
			if err != nil {
				t.Fatalf("must not be fatal: %v", err)
			}
			if got.ok() {
				t.Error("must report unverified")
			}
		})
	}
}

// --- the ladder ------------------------------------------------------------

func TestVerifyJARAt_FallsBackToDigestWhenNoSignature(t *testing.T) {
	content := []byte("fake jar content")
	testSigner(t)

	got, err := verifyJARAt(writeTempJAR(t, content), serve(t, 404, ""), serve(t, 200, digestJSON(content)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.method != verifiedDigest {
		t.Errorf("method = %q, want %q (reason: %s)", got.method, verifiedDigest, got.reason)
	}
}

// A signature is the stronger check; when it verifies, the digest is not needed.
func TestVerifyJARAt_SignatureWins(t *testing.T) {
	content := []byte("fake jar content")
	signer := testSigner(t)

	got, err := verifyJARAt(writeTempJAR(t, content),
		serve(t, 200, armoredSig(t, signer, content)), serve(t, 500, ""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.method != verifiedSignature {
		t.Errorf("method = %q, want %q", got.method, verifiedSignature)
	}
}

// A bad signature is proof the JAR is wrong. A matching digest must not launder it.
func TestVerifyJARAt_BadSignatureNeverFallsBackToDigest(t *testing.T) {
	signer := testSigner(t)
	substituted := []byte("a substituted jar")
	sig := armoredSig(t, signer, []byte("the original jar"))

	_, err := verifyJARAt(writeTempJAR(t, substituted),
		serve(t, 200, sig), serve(t, 200, digestJSON(substituted)))
	if err == nil {
		t.Fatal("a bad signature must stay fatal even when the digest matches")
	}
}

func TestVerifyJARAt_NeitherAvailable(t *testing.T) {
	testSigner(t)
	got, err := verifyJARAt(writeTempJAR(t, []byte("content")), serve(t, 404, ""), serve(t, 404, ""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ok() {
		t.Error("must report unverified")
	}
	if got.reason == "" {
		t.Error("an unverified result must always carry a reason")
	}
}

func TestVerifyJAR_EmptyVersion_Skips(t *testing.T) {
	got, err := verifyJAR(writeTempJAR(t, []byte("content")), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ok() {
		t.Error("without a version there is nothing to verify against")
	}
}

// --- the recorded status ---------------------------------------------------

func TestVerifyStatus_RoundTrip(t *testing.T) {
	for _, tc := range []struct {
		method     verifyMethod
		wantVerify bool
		wantDesc   string
	}{
		{verifiedSignature, true, "PGP signature"},
		{verifiedDigest, true, "GitHub release digest"},
		{verifiedNot, false, "not verified"},
	} {
		t.Run(string(tc.method), func(t *testing.T) {
			t.Setenv(cache.DirEnvVar, t.TempDir())
			if err := saveVerifyStatus(tc.method); err != nil {
				t.Fatal(err)
			}
			desc, verified, known := JARVerification()
			if !known {
				t.Fatal("a status just written must be known")
			}
			if verified != tc.wantVerify {
				t.Errorf("verified = %v, want %v", verified, tc.wantVerify)
			}
			if desc != tc.wantDesc {
				t.Errorf("description = %q, want %q", desc, tc.wantDesc)
			}
		})
	}
}

// A cache written by a fhirlint that only recorded a boolean.
func TestVerifyStatus_LegacyVerified(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(cache.DirEnvVar, dir)
	if err := os.WriteFile(filepath.Join(dir, "validator_checksum_status.txt"), []byte("verified"), 0600); err != nil {
		t.Fatal(err)
	}
	desc, verified, known := JARVerification()
	if !known || !verified {
		t.Fatalf("legacy 'verified' must stay trusted, got known=%v verified=%v", known, verified)
	}
	if !strings.Contains(desc, "earlier fhirlint") {
		t.Errorf("legacy status should say it cannot name the method, got %q", desc)
	}
}

// A status written by a newer fhirlint must not be reported as verified here:
// we cannot vouch for a method whose meaning this build does not know.
func TestVerifyStatus_UnknownMethod_IsNotTrusted(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(cache.DirEnvVar, dir)
	for _, body := range []string{"verified:attestation", "verified:", "garbage", ""} {
		if err := os.WriteFile(filepath.Join(dir, "validator_checksum_status.txt"), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, verified, known := JARVerification(); verified || known {
			t.Errorf("%q: want unknown and unverified, got verified=%v known=%v", body, verified, known)
		}
	}
}

func TestVerifyStatus_NoFile(t *testing.T) {
	t.Setenv(cache.DirEnvVar, t.TempDir())
	if _, _, known := JARVerification(); known {
		t.Error("with no status file nothing is known")
	}
}
