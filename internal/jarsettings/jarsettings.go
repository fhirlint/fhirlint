// Package jarsettings writes the fhir-settings.json the HL7 validator reads
// with -fhir-settings.
//
// The file is how per-server behaviour reaches the JAR: credentials, and the
// exemptions that let a run reach a plain-HTTP or private-network address. The
// validator takes exactly one such file, so everything that needs an entry has
// to go through one writer — a terminology server with a token and fhirlint's
// own loopback replay server can both be in play in the same run.
package jarsettings

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

// Authentication modes the validator understands, from
// ServerDetailsPOJOHTTPAuthProvider. Anything else is read as "no
// authentication", which is why AuthNone is spelled out rather than left empty:
// the upstream switch dereferences the field without a nil check, so an entry
// that omits it can take the validator down.
const (
	AuthNone   = "none"
	AuthToken  = "token"  // Authorization: Bearer <token>
	AuthBasic  = "basic"  // Authorization: Basic base64(user:password)
	AuthAPIKey = "apikey" // Api-Key: <key>
)

// Server is one entry in the settings file. Matching is by URL prefix, first
// match wins, so the most specific URL belongs first.
type Server struct {
	URL                 string            `json:"url"`
	AuthenticationType  string            `json:"authenticationType"`
	Token               string            `json:"token,omitempty"`
	APIKey              string            `json:"apikey,omitempty"`
	Username            string            `json:"username,omitempty"`
	Password            string            `json:"password,omitempty"`
	AllowHTTP           bool              `json:"allowHttp,omitempty"`
	AllowPrivateNetwork bool              `json:"allowPrivateNetwork,omitempty"`
	Headers             map[string]string `json:"headers,omitempty"`
}

type settings struct {
	Servers []Server `json:"servers"`
}

// Write renders the servers into a fresh fhir-settings.json in a temp directory
// and returns its path plus a cleanup function.
//
// The file is written per run rather than kept: it can hold credentials, and a
// URL match is prefix-based and port-sensitive since CVE-2026-34361, so a
// stored file would be wrong as soon as a port changed.
//
// Mode 0600, and the directory 0700 by way of os.MkdirTemp: a token in a
// world-readable temp file would be a poor trade for saving a chmod.
func Write(servers []Server) (path string, cleanup func(), err error) {
	if len(servers) == 0 {
		return "", nil, fmt.Errorf("no servers to write")
	}
	for i := range servers {
		if servers[i].AuthenticationType == "" {
			servers[i].AuthenticationType = AuthNone
		}
	}

	dir, err := os.MkdirTemp("", "fhirlint-jar-settings-")
	if err != nil {
		return "", nil, fmt.Errorf("creating validator settings directory: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(dir) }

	data, err := json.MarshalIndent(settings{Servers: servers}, "", "  ")
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("encoding validator settings: %w", err)
	}

	path = filepath.Join(dir, "fhir-settings.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("writing validator settings: %w", err)
	}
	return path, cleanup, nil
}

// InsecureServer describes a terminology server the run may reach over plain
// HTTP or on a private network.
//
// Validator 6.10.0 added SSRF protection that refuses both, which is right by
// default and wrong for the server someone is deliberately pointing at: a
// loopback replay server, or a Blaze/HAPI/Ontoserver instance on the local
// network. The exemption names one URL. Turning the protection off outright
// (-ssrf-protection-enabled false) would also work and is much worse, because
// it lifts the check for every other request the run makes.
//
// The URL must carry its port. Prefix matching was tightened when
// CVE-2026-34361 was fixed, so a host-only entry no longer covers a
// port-bearing URL, which is why these files are generated per run.
func InsecureServer(rawURL string) (Server, error) {
	if _, err := url.Parse(rawURL); err != nil {
		return Server{}, fmt.Errorf("invalid server URL %q: %w", rawURL, err)
	}
	return Server{
		URL:                 rawURL,
		AllowHTTP:           true,
		AllowPrivateNetwork: true,
	}, nil
}
