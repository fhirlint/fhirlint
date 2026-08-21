package txreplay

import (
	"fmt"
	"net/url"

	"github.com/fhirlint/fhirlint/internal/jarsettings"
)

// ReplayServerEntry describes fhirlint's own replay server for the validator's
// settings file.
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
// CVE-2026-34361 was fixed, so a host-only entry no longer covers a
// port-bearing URL — which is why the file is generated per run rather than
// shipped.
func ReplayServerEntry(baseURL string) (jarsettings.Server, error) {
	if _, err := url.Parse(baseURL); err != nil {
		return jarsettings.Server{}, fmt.Errorf("invalid replay server URL %q: %w", baseURL, err)
	}
	return jarsettings.Server{
		URL:                 baseURL,
		AllowHTTP:           true,
		AllowPrivateNetwork: true,
	}, nil
}

// WriteJARSettings writes a settings file whose only entry is the replay
// server. Callers that also have terminology credentials build the list
// themselves and call jarsettings.Write, because the validator reads one file.
func WriteJARSettings(baseURL string) (path string, cleanup func(), err error) {
	entry, err := ReplayServerEntry(baseURL)
	if err != nil {
		return "", nil, err
	}
	return jarsettings.Write([]jarsettings.Server{entry})
}
