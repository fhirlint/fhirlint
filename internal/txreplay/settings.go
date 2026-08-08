package txreplay

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

// jarSettings mirrors the parts of the validator's fhir-settings.json we need.
// Only the server exemption list matters here; the file is written fresh for
// each run, so nothing else has to be preserved.
type jarSettings struct {
	Servers []jarServer `json:"servers"`
}

type jarServer struct {
	URL                 string `json:"url"`
	AllowHTTP           bool   `json:"allowHttp"`
	AllowPrivateNetwork bool   `json:"allowPrivateNetwork"`
}

// WriteJARSettings writes a fhir-settings.json that lets the validator talk to
// our local replay server, and returns its path plus a cleanup function.
//
// Validator 6.10.0 added SSRF protection that refuses plain-HTTP and
// private-network destinations. Our server is both: loopback, over HTTP.
// Without an exemption the validator rejects the terminology endpoint before
// validating anything ("Refusing to fetch from non-https URL"), which breaks
// replay entirely on 6.10.0 and later.
//
// The exemption is scoped to exactly this server. Disabling SSRF protection
// wholesale (-ssrf-protection-enabled false) would also work and is a great
// deal worse: it would lift the protection for every request the run makes.
//
// The URL must carry the port. Prefix matching was tightened when
// CVE-2026-34361 was fixed, so a host-only entry no longer covers a port-bearing
// URL — which is why this file is generated per run rather than shipped.
func WriteJARSettings(baseURL string) (path string, cleanup func(), err error) {
	if _, perr := url.Parse(baseURL); perr != nil {
		return "", nil, fmt.Errorf("invalid replay server URL %q: %w", baseURL, perr)
	}
	dir, err := os.MkdirTemp("", "fhirlint-tx-settings-")
	if err != nil {
		return "", nil, fmt.Errorf("creating validator settings directory: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(dir) }

	data, err := json.MarshalIndent(jarSettings{Servers: []jarServer{{
		URL:                 baseURL,
		AllowHTTP:           true,
		AllowPrivateNetwork: true,
	}}}, "", "  ")
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
