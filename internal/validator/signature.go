package validator

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	_ "embed"

	"github.com/ProtonMail/go-crypto/openpgp"
	pgperrors "github.com/ProtonMail/go-crypto/openpgp/errors"
)

// hl7SigningKey is the public key HL7 signs the validator releases with, pinned
// in-tree rather than fetched at runtime: a key downloaded over the same channel
// as the JAR proves nothing about the JAR. Pinning also makes a key rotation a
// reviewable commit instead of a silent change.
//
// The same key signs the Maven Central releases of
// ca.uhn.hapi.fhir:org.hl7.fhir.validation, an upload path with separate
// credentials, which is how it was corroborated beyond the keyserver copy.
//
//go:embed hl7_signing_key.asc
var hl7SigningKey []byte

// hl7SigningKeyFingerprint is asserted against the embedded key at startup of
// the verification, so swapping the .asc file without meaning to cannot go
// unnoticed.
const hl7SigningKeyFingerprint = "85D1C17CF1152107B272386C8FDFA68281399B5D"

// verifyMethod records how a cached JAR was verified. It is persisted, so the
// values are part of the on-disk format.
type verifyMethod string

const (
	verifiedNot       verifyMethod = ""
	verifiedSignature verifyMethod = "signature"
	verifiedDigest    verifyMethod = "digest"
	// verifiedLegacy marks a JAR recorded as verified by a fhirlint that did not
	// yet distinguish methods. It is trusted, but it cannot say how.
	verifiedLegacy verifyMethod = "legacy"
)

// jarVerification is the outcome of checking a downloaded JAR.
//
// The contract is deliberately asymmetric, because the two failures are not
// alike. A non-nil error from verifyJAR means the JAR is actively wrong — a
// signature that does not verify, a digest that does not match — and it must be
// discarded. A nil error with method verifiedNot means verification could not be
// performed at all; reason says why, and the caller surfaces it without failing.
type jarVerification struct {
	method verifyMethod
	reason string
}

func (v jarVerification) ok() bool { return v.method != verifiedNot }

// describe names the method for a user-facing message.
func (v verifyMethod) describe() string {
	switch v {
	case verifiedSignature:
		return "PGP signature"
	case verifiedDigest:
		return "GitHub release digest"
	case verifiedLegacy:
		return "an earlier fhirlint"
	default:
		return "not verified"
	}
}

// The two upstream URLs are indirected through vars so unit tests can point
// the verification at a local server instead of reaching github.com.
var (
	jarSignatureURLFor = jarSignatureURL
	releaseAPIURLFor   = releaseAPIURL
)

func jarSignatureURL(version string) string {
	return jarSourceRepo + "/releases/download/" + version + "/validator_cli.jar.asc"
}

func releaseAPIURL(version string) string {
	return "https://api.github.com/repos/hapifhir/org.hl7.fhir.core/releases/tags/" + version
}

// verifyJAR checks a freshly downloaded JAR against what upstream publishes.
//
// It tries two things, strongest first:
//
//  1. The detached PGP signature (validator_cli.jar.asc), against the pinned HL7
//     key. This binds the JAR to a maintainer's key and is the only check that
//     survives a compromised release channel.
//  2. The SHA-256 digest GitHub records for the release asset. This travels the
//     same channel as the download, so it proves the transfer was not corrupted
//     or truncated, not that the release itself is genuine. It is a fallback,
//     and it is recorded as the weaker method it is.
//
// Releases before 6.6.0 carry neither, so they verify as unverified.
func verifyJAR(jarPath, version string) (jarVerification, error) {
	if version == "" {
		// Nothing to look up: the download never revealed which release it was.
		return jarVerification{reason: "the release version could not be determined"}, nil
	}

	return verifyJARAt(jarPath, jarSignatureURLFor(version), releaseAPIURLFor(version))
}

// verifyJARAt is verifyJAR with the two upstream URLs supplied, so the fallback
// ladder can be exercised against test servers.
func verifyJARAt(jarPath, sigURL, apiURL string) (jarVerification, error) {
	sigResult, err := verifyJARSignatureURL(jarPath, sigURL)
	if err != nil {
		// A signature that is present and does not verify is the one case where
		// the JAR is known bad. Never fall back from it.
		return jarVerification{}, err
	}
	if sigResult.ok() {
		return sigResult, nil
	}

	digestResult, err := verifyJARDigestURL(jarPath, apiURL)
	if err != nil {
		return jarVerification{}, err
	}
	if digestResult.ok() {
		return digestResult, nil
	}

	// Neither worked. Report the signature's reason: it is the check that
	// matters, so its absence is the more useful thing to tell the user.
	return jarVerification{reason: sigResult.reason + "; " + digestResult.reason}, nil
}

// signatureKeyring returns the keys a JAR signature is accepted from. It is a
// var so tests can pin a throwaway key instead of HL7's.
var signatureKeyring = hl7Keyring

// hl7Keyring parses the pinned key and checks it is the one we think it is.
func hl7Keyring() (openpgp.EntityList, error) {
	ring, err := openpgp.ReadArmoredKeyRing(strings.NewReader(string(hl7SigningKey)))
	if err != nil {
		return nil, fmt.Errorf("parsing the pinned HL7 signing key: %w", err)
	}
	if len(ring) != 1 {
		return nil, fmt.Errorf("pinned HL7 signing key holds %d entities, want exactly 1", len(ring))
	}
	got := strings.ToUpper(hex.EncodeToString(ring[0].PrimaryKey.Fingerprint))
	if got != hl7SigningKeyFingerprint {
		return nil, fmt.Errorf(
			"pinned HL7 signing key has fingerprint %s, want %s", got, hl7SigningKeyFingerprint)
	}
	return ring, nil
}

// PinnedSigningKeyExpiry returns when the pinned HL7 signing key expires, and
// whether it carries an expiry at all. Exported so CI can warn before the pin
// lapses, rather than discovering it when verification silently stops (#358).
func PinnedSigningKeyExpiry() (time.Time, bool) {
	ring, err := hl7Keyring()
	if err != nil {
		return time.Time{}, false
	}
	return keyExpiry(ring)
}

// keyExpiry reports when the ring's primary key expires.
func keyExpiry(ring openpgp.EntityList) (time.Time, bool) {
	if len(ring) == 0 {
		return time.Time{}, false
	}
	ident := ring[0].PrimaryIdentity()
	if ident == nil || ident.SelfSignature == nil || ident.SelfSignature.KeyLifetimeSecs == nil {
		return time.Time{}, false
	}
	life := *ident.SelfSignature.KeyLifetimeSecs
	if life == 0 {
		// RFC 4880: a zero lifetime means the key never expires. Adding it to the
		// creation time would instead read as "expired the moment it was made".
		return time.Time{}, false
	}
	return ring[0].PrimaryKey.CreationTime.Add(time.Duration(life) * time.Second), true
}

// verifyJARSignatureURL verifies the JAR against the detached signature at
// sigURL. A non-nil error means the signature is present and does not verify.
func verifyJARSignatureURL(jarPath, sigURL string) (jarVerification, error) {
	sig, reason := fetchAll(sigURL, "no PGP signature published for this release")
	if sig == nil {
		return jarVerification{reason: reason}, nil
	}

	ring, err := signatureKeyring()
	if err != nil {
		// A broken embedded key is a build defect, not a bad download. Refusing
		// the JAR over it would turn our own mistake into everyone's outage.
		return jarVerification{reason: err.Error()}, nil
	}

	// KeysByIdUsage silently skips an expired key, which surfaces as
	// ErrUnknownIssuer and reads like a key rotation. Check expiry up front so
	// the message says what actually happened.
	if expiry, ok := keyExpiry(ring); ok && time.Now().After(expiry) {
		return jarVerification{reason: fmt.Sprintf(
			"the pinned signing key expired on %s — upgrade fhirlint",
			expiry.Format("2006-01-02"))}, nil
	}

	f, err := os.Open(jarPath) //nolint:gosec // our own cache path
	if err != nil {
		return jarVerification{}, err
	}
	defer func() { _ = f.Close() }()

	_, err = openpgp.CheckArmoredDetachedSignature(ring, f, strings.NewReader(string(sig)), nil)
	switch {
	case err == nil:
		return jarVerification{method: verifiedSignature}, nil

	case errors.Is(err, pgperrors.ErrUnknownIssuer):
		// Signed by a key we do not pin. Most likely an upstream key rotation;
		// it could also be a substituted JAR and signature. We cannot tell the
		// two apart, so this is the same "unverified" state as no signature at
		// all — no worse than before, and never silent. The digest fallback runs
		// next and may still catch a substitution.
		return jarVerification{reason: fmt.Sprintf(
			"the signature is not from the pinned HL7 key (%s) — upstream may have rotated it",
			hl7SigningKeyFingerprint)}, nil

	case isUnreadableSignature(err):
		// The signature file is corrupt or is not a signature at all. That says
		// nothing about the JAR, and treating it as proof of tampering would let
		// anyone who can garble one response fail the download outright. Same
		// class as no signature at all.
		return jarVerification{reason: "the PGP signature file could not be read: " + err.Error()}, nil

	default:
		// A readable signature that does not verify. The JAR is not what was
		// signed; this is the one outcome that condemns the download.
		return jarVerification{}, fmt.Errorf(
			"the PGP signature does not verify against the pinned HL7 key (%s): %w",
			hl7SigningKeyFingerprint, err)
	}
}

// isUnreadableSignature separates "this is not a usable signature file" from
// "this signature is usable and the data does not match it". Only the latter
// may fail a download.
func isUnreadableSignature(err error) bool {
	var (
		structural  pgperrors.StructuralError
		unsupported pgperrors.UnsupportedError
		invalid     pgperrors.InvalidArgumentError
	)
	return errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.As(err, &structural) ||
		errors.As(err, &unsupported) ||
		errors.As(err, &invalid)
}

// verifyJARDigestURL compares the JAR against the SHA-256 digest GitHub records
// for the release asset. A non-nil error means the digest is present and does
// not match.
func verifyJARDigestURL(jarPath, apiURL string) (jarVerification, error) {
	const missing = "GitHub publishes no digest for this release asset"

	body, reason := fetchAll(apiURL, missing)
	if body == nil {
		return jarVerification{reason: reason}, nil
	}

	var release struct {
		Assets []struct {
			Name   string `json:"name"`
			Digest string `json:"digest"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return jarVerification{reason: missing}, nil
	}
	var want string
	for _, a := range release.Assets {
		if a.Name == "validator_cli.jar" {
			want = strings.TrimPrefix(a.Digest, "sha256:")
			break
		}
	}
	// Assets uploaded before GitHub started recording digests have none.
	if want == "" {
		return jarVerification{reason: missing}, nil
	}

	got, err := fileSHA256(jarPath)
	if err != nil {
		return jarVerification{}, err
	}
	if !strings.EqualFold(got, want) {
		return jarVerification{}, fmt.Errorf(
			"SHA-256 mismatch against the GitHub release asset: got %s, want %s", got, want)
	}
	return jarVerification{method: verifiedDigest}, nil
}

// fetchAll GETs url and returns its body, or nil plus a user-facing reason when
// it could not be had. A network failure and a 404 are deliberately the same
// outcome: anyone able to tamper with the download can usually also make these
// requests fail, so neither may be read as a pass.
func fetchAll(url, missingReason string) (body []byte, reason string) {
	req, err := http.NewRequest(http.MethodGet, url, nil) //nolint:noctx // known URL, no user input
	if err != nil {
		return nil, missingReason
	}
	// CI runners share outbound IPs and hit the 60/hour unauthenticated GitHub
	// API limit as a group. Use the token when the environment offers one.
	if strings.HasPrefix(url, "https://api.github.com/") {
		if tok := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "it could not be fetched: " + err.Error()
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, missingReason
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "it could not be fetched: " + err.Error()
	}
	return data, ""
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // our own cache path
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
