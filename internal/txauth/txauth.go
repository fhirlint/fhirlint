// Package txauth carries credentials for a terminology server.
//
// Credentials come from the environment and from nowhere else. There is no flag
// and no config key holding a secret, for the reason spelled out at
// validator.ProxyAuthEnvVar: a flag lands in shell history and in CI logs, and a
// config value lands in the repository. A terminology server credential is
// usually a real production one.
package txauth

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

// The variables fhirlint reads. One per authentication mode the validator
// supports, so which mode to use never has to be configured separately — the
// variable that is set says it.
const (
	// nolint:gosec // G101: these are the names of environment variables, not
	// credentials — holding a secret in a constant is the thing this package
	// exists to avoid.
	TokenEnvVar = "FHIRLINT_TX_TOKEN" // bearer token
	// nolint:gosec // G101: an environment variable name, see above.
	APIKeyEnvVar = "FHIRLINT_TX_APIKEY" // Api-Key header
	BasicEnvVar  = "FHIRLINT_TX_AUTH"   // "username:password", like FHIRLINT_PROXY_AUTH
)

// Mode is the authentication scheme, named as the validator names it.
type Mode string

const (
	ModeNone   Mode = ""
	ModeToken  Mode = "token"
	ModeAPIKey Mode = "apikey"
	ModeBasic  Mode = "basic"
)

// Credentials is what was found in the environment.
type Credentials struct {
	Mode     Mode
	Token    string
	APIKey   string
	Username string
	Password string
}

// Empty reports whether there is nothing to authenticate with.
func (c Credentials) Empty() bool { return c.Mode == ModeNone }

// FromEnv reads the credentials. Setting more than one variable is an error
// rather than a precedence rule: two credentials in the environment means one of
// them is stale, and quietly picking a winner is how the wrong one gets used for
// months.
func FromEnv() (Credentials, error) {
	return fromLookup(os.Getenv)
}

func fromLookup(get func(string) string) (Credentials, error) {
	token := strings.TrimSpace(get(TokenEnvVar))
	apiKey := strings.TrimSpace(get(APIKeyEnvVar))
	basic := strings.TrimSpace(get(BasicEnvVar))

	var set []string
	for _, v := range []struct{ name, value string }{
		{TokenEnvVar, token}, {APIKeyEnvVar, apiKey}, {BasicEnvVar, basic},
	} {
		if v.value != "" {
			set = append(set, v.name)
		}
	}
	if len(set) > 1 {
		return Credentials{}, fmt.Errorf(
			"%s are both set — pick one: fhirlint cannot tell which credential the terminology server wants",
			strings.Join(set, " and "))
	}

	switch {
	case token != "":
		return Credentials{Mode: ModeToken, Token: token}, nil
	case apiKey != "":
		return Credentials{Mode: ModeAPIKey, APIKey: apiKey}, nil
	case basic != "":
		user, pass, ok := strings.Cut(basic, ":")
		if !ok || user == "" {
			return Credentials{}, fmt.Errorf("%s must be \"username:password\"", BasicEnvVar)
		}
		return Credentials{Mode: ModeBasic, Username: user, Password: pass}, nil
	}
	return Credentials{}, nil
}

// Header renders the credential as the HTTP header the validator would send,
// for the paths where fhirlint makes the request itself — recording with
// `fhirlint tx warm` goes through fhirlint's own proxy, not through the JAR.
//
// The names mirror ServerDetailsPOJOHTTPAuthProvider exactly, so a recording
// made through fhirlint reaches the server the same way a direct validator run
// would.
func (c Credentials) Header() (name, value string) {
	switch c.Mode {
	case ModeToken:
		return "Authorization", "Bearer " + c.Token
	case ModeAPIKey:
		return "Api-Key", c.APIKey
	case ModeBasic:
		return "Authorization", "Basic " +
			base64.StdEncoding.EncodeToString([]byte(c.Username+":"+c.Password))
	case ModeNone:
		return "", ""
	}
	return "", ""
}

// Describe names the credential in use without revealing it, for the one line
// fhirlint prints so that a run is never silently authenticated.
func (c Credentials) Describe() string {
	switch c.Mode {
	case ModeToken:
		return "bearer token from " + TokenEnvVar
	case ModeAPIKey:
		return "API key from " + APIKeyEnvVar
	case ModeBasic:
		return "basic auth as " + c.Username + ", from " + BasicEnvVar
	case ModeNone:
		return "none"
	}
	return "none"
}
