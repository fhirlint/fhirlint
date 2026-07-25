package validator

import (
	"net/url"
	"os"
	"strings"
)

// ProxyAuthEnvVar carries proxy credentials as "username:password".
//
// Deliberately environment-only: there is no --proxy-auth flag, because a
// credential passed on fhirlint's own command line lands in shell history and
// CI job logs, and no fhirlint.yml key, because that file is meant to be
// committed. See proxyArgs for what fhirlint still cannot protect.
const ProxyAuthEnvVar = "FHIRLINT_PROXY_AUTH"

// ProxyConfig holds the proxy settings handed to the validator JAR.
type ProxyConfig struct {
	Proxy      string // http proxy, "host:port" (-proxy)
	HTTPSProxy string // https proxy, "host:port" (-https-proxy)
	Auth       string // "username:password" (-auth)
}

// proxyEnvVars lists the environment variables consulted for each setting, in
// order. Both cases are checked because the lowercase spelling is the older
// convention and still the more common one in container images.
var (
	httpProxyEnv  = []string{"HTTP_PROXY", "http_proxy"}
	httpsProxyEnv = []string{"HTTPS_PROXY", "https_proxy"}
)

func firstEnv(names []string) string {
	for _, n := range names {
		if v := strings.TrimSpace(os.Getenv(n)); v != "" {
			return v
		}
	}
	return ""
}

// splitProxyURL turns a proxy value into the "host:port" the JAR expects plus
// any credentials embedded in it.
//
// Go's own HTTP client reads HTTPS_PROXY as a URL ("http://user:pass@host:3128"),
// but the validator wants a bare address and takes credentials separately via
// -auth. Without this conversion, exporting the proxy variables that already
// work for every other tool would hand the JAR a value it cannot parse.
func splitProxyURL(raw string) (hostPort, auth string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	// A bare "host:port" has no scheme; url.Parse would read "host" as the
	// scheme and "port" as an opaque path, so give it something to chew on.
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		// Not a URL we understand — pass it through untouched rather than
		// silently dropping a setting the user meant.
		return strings.TrimPrefix(raw, "http://"), ""
	}
	if u.User != nil {
		pass, _ := u.User.Password()
		auth = u.User.Username()
		if pass != "" {
			auth += ":" + pass
		}
	}
	return u.Host, auth
}

// resolveProxy fills in anything not set explicitly from the environment, so the
// standard proxy variables just work without a second, fhirlint-specific way to
// say the same thing.
func resolveProxy(cfg ProxyConfig) ProxyConfig {
	out := ProxyConfig{Auth: strings.TrimSpace(cfg.Auth)}
	if out.Auth == "" {
		out.Auth = strings.TrimSpace(os.Getenv(ProxyAuthEnvVar))
	}

	httpRaw, httpsRaw := cfg.Proxy, cfg.HTTPSProxy
	if strings.TrimSpace(httpRaw) == "" {
		httpRaw = firstEnv(httpProxyEnv)
	}
	if strings.TrimSpace(httpsRaw) == "" {
		httpsRaw = firstEnv(httpsProxyEnv)
	}

	var derivedAuth string
	out.Proxy, derivedAuth = splitProxyURL(httpRaw)
	if out.Auth == "" {
		out.Auth = derivedAuth
	}
	var httpsAuth string
	out.HTTPSProxy, httpsAuth = splitProxyURL(httpsRaw)
	if out.Auth == "" {
		out.Auth = httpsAuth
	}
	return out
}

// proxyArgs renders the proxy settings as validator JAR arguments.
//
// Note what this cannot fix: -auth is a command-line argument of the java child
// process, so the credential is visible in `ps` to other users on the same host.
// Reading it from the environment keeps it out of shell history and CI logs,
// which is worth doing, but it is not a secret-safe channel. A proxy that does
// not require basic auth is the better answer where one is available.
func proxyArgs(cfg ProxyConfig) []string {
	p := resolveProxy(cfg)
	var args []string
	if p.Proxy != "" {
		args = append(args, "-proxy", p.Proxy)
	}
	if p.HTTPSProxy != "" {
		args = append(args, "-https-proxy", p.HTTPSProxy)
	}
	// Credentials are pointless without a proxy to send them to, and passing
	// them alone would put a secret in argv for no benefit.
	if p.Auth != "" && (p.Proxy != "" || p.HTTPSProxy != "") {
		args = append(args, "-auth", p.Auth)
	}
	return args
}
