package validator

import (
	"github.com/fhirlint/fhirlint/internal/updatecheck"
)

// jarRepo is the upstream repository the validator JAR is released from.
const jarRepo = "hapifhir/org.hl7.fhir.core"

// latestJARRelease is a var so tests can drive the comparison without a network
// call or a cache file.
var latestJARRelease = updatecheck.Latest

// CheckForUpdate returns a newer validator version if one is available, or an
// empty string if not. The lookup is cached for a day; see
// internal/updatecheck.
//
// The comparison is a plain inequality rather than a version ordering: JAR tags
// are bare "6.10.2" strings, not semver, and the only question here is whether
// what upstream calls latest differs from what is installed.
func CheckForUpdate() string {
	current := ValidatorVersion()
	if current == "unknown" {
		return ""
	}
	latest := latestJARRelease(jarRepo)
	if latest != "" && latest != current {
		return latest
	}
	return ""
}

// FetchLatestVersion queries the GitHub API for the latest JAR release version,
// bypassing the cache. `fhirlint audit` uses it to report against what upstream
// ships right now rather than what was last seen.
func FetchLatestVersion() (string, error) {
	return updatecheck.FetchLatest(jarRepo)
}
