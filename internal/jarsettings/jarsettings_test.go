// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package jarsettings

import (
	"encoding/json"
	"os"
	"runtime"
	"testing"
)

// The fhir-settings.json the validator reads (#686).
func TestWrite(t *testing.T) {
	path, cleanup, err := Write([]Server{
		{URL: "https://tx.example.org/fhir", AuthenticationType: AuthToken, Token: "s3cret"},
		{URL: "http://127.0.0.1:40797", AllowHTTP: true, AllowPrivateNetwork: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path) //nolint:gosec // the file under test
	if err != nil {
		t.Fatal(err)
	}
	var got settings
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Servers) != 2 || got.Servers[0].Token != "s3cret" || got.Servers[0].AuthenticationType != AuthToken {
		t.Errorf("servers, in order and with their credentials: %+v", got.Servers)
	}
	// The validator dereferences the type without a nil check, so an entry
	// without one is written as "none".
	if got.Servers[1].AuthenticationType != AuthNone || !got.Servers[1].AllowHTTP || !got.Servers[1].AllowPrivateNetwork {
		t.Errorf("an entry without credentials: %+v", got.Servers[1])
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("a file that can hold a token must be 0600: %v %v", info.Mode(), err)
		}
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("cleanup must remove the file: %v", err)
	}

	if _, _, err := Write(nil); err == nil {
		t.Error("no servers is an error, not an empty file")
	}
}

func TestInsecureServer(t *testing.T) {
	s, err := InsecureServer("http://127.0.0.1:40797")
	if err != nil || s.URL != "http://127.0.0.1:40797" || !s.AllowHTTP || !s.AllowPrivateNetwork || s.Token != "" {
		t.Errorf("an exemption for one URL, and nothing else: %+v %v", s, err)
	}
	if _, err := InsecureServer("http://[::1"); err == nil {
		t.Error("a URL that doesn't parse must be refused")
	}
}
