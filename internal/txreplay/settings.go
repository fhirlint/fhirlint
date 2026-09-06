package txreplay

import (
	"fmt"

	"github.com/fhirlint/fhirlint/internal/jarsettings"
)

// ReplayServerEntry describes fhirlint's own replay server for the validator's
// settings file. The exemption itself is not replay-specific; see
// jarsettings.InsecureServer for why it is scoped to one URL.
func ReplayServerEntry(baseURL string) (jarsettings.Server, error) {
	entry, err := jarsettings.InsecureServer(baseURL)
	if err != nil {
		return jarsettings.Server{}, fmt.Errorf("invalid replay server URL %q: %w", baseURL, err)
	}
	return entry, nil
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
