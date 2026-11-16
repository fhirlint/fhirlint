// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package validator

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
)

// The corners of JAR verification (#691).

// A broken embedded key is our build defect, not a bad download: the JAR is
// not refused over it, and the reason says what is wrong.
func TestVerifySignature_BrokenKeyringIsNotFatal(t *testing.T) {
	content := []byte("fake jar content")
	signer := testSigner(t)
	sig := armoredSig(t, signer, content)
	signatureKeyring = func() (openpgp.EntityList, error) { return nil, errors.New("pinned key unreadable") }
	got, err := verifyJARSignatureURL(writeTempJAR(t, content), serve(t, 200, sig))
	if err != nil || got.ok() || got.reason != "pinned key unreadable" {
		t.Errorf("got %+v, %v", got, err)
	}
}

// A JAR that isn't there can't be verified either way: that is an error, not a
// "not verified" that would let the caller install it.
func TestVerify_MissingJARIsAnError(t *testing.T) {
	content := []byte("fake jar content")
	signer := testSigner(t)
	missing := filepath.Join(t.TempDir(), "gone.jar")
	if _, err := verifyJARSignatureURL(missing, serve(t, 200, armoredSig(t, signer, content))); err == nil {
		t.Error("signature check of a missing JAR must fail")
	}
	if _, err := verifyJARDigestURL(missing, serve(t, 200, digestJSON(content))); err == nil {
		t.Error("digest check of a missing JAR must fail")
	}
}

// A request that fails on the network reads as unavailable and says why.
func TestFetchAll_NetworkFailure(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	body, reason := fetchAll(url, "missing")
	if body != nil || !strings.HasPrefix(reason, "it could not be fetched: ") {
		t.Errorf("got %q, %q", body, reason)
	}
	if body, reason := fetchAll("http://bad host/x", "missing"); body != nil || reason != "missing" {
		t.Errorf("an unparsable URL is just unavailable: %q, %q", body, reason)
	}
}

func entityWithLifetime(t *testing.T, secs uint32) *openpgp.Entity {
	t.Helper()
	e, err := openpgp.NewEntity("Test", "", "t@example.invalid", &packet.Config{KeyLifetimeSecs: secs})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestKeyExpiry(t *testing.T) {
	if _, ok := keyExpiry(nil); ok {
		t.Error("an empty ring has no expiry")
	}
	// No lifetime recorded at all: the key never expires.
	n := entityWithLifetime(t, 3600)
	n.PrimaryIdentity().SelfSignature.KeyLifetimeSecs = nil
	if _, ok := keyExpiry(openpgp.EntityList{n}); ok {
		t.Error("a key without a lifetime never expires")
	}
	e := entityWithLifetime(t, 3600)
	at, ok := keyExpiry(openpgp.EntityList{e})
	if !ok || !at.Equal(e.PrimaryKey.CreationTime.Add(time.Hour)) {
		t.Errorf("expiry = %v, %v; want creation + 1h", at, ok)
	}
	// RFC 4880: an explicit zero lifetime also means "never", not "expired at once".
	z := entityWithLifetime(t, 3600)
	var zero uint32
	z.PrimaryIdentity().SelfSignature.KeyLifetimeSecs = &zero
	if _, ok := keyExpiry(openpgp.EntityList{z}); ok {
		t.Error("a zero lifetime means the key never expires")
	}
}
