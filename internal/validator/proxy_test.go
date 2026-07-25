package validator

import (
	"strings"
	"testing"
)

func TestSplitProxyURL(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		hostPort string
		auth     string
	}{
		{"empty", "", "", ""},
		{"bare host:port", "proxy.example.org:3128", "proxy.example.org:3128", ""},
		{"http URL", "http://proxy.example.org:3128", "proxy.example.org:3128", ""},
		{"https URL", "https://proxy.example.org:3128", "proxy.example.org:3128", ""},
		{"credentials in URL", "http://user:pass@proxy.example.org:3128", "proxy.example.org:3128", "user:pass"},
		{"username only", "http://user@proxy.example.org:3128", "proxy.example.org:3128", "user"},
		{"no port", "http://proxy.example.org", "proxy.example.org", ""},
		{"surrounding space", "  http://proxy.example.org:3128  ", "proxy.example.org:3128", ""},
		{"IPv6", "http://[2001:db8::1]:3128", "[2001:db8::1]:3128", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host, auth := splitProxyURL(tc.raw)
			if host != tc.hostPort {
				t.Errorf("host = %q, want %q", host, tc.hostPort)
			}
			if auth != tc.auth {
				t.Errorf("auth = %q, want %q", auth, tc.auth)
			}
		})
	}
}

// A password with characters that are special in a URL must survive the round
// trip, or the credential silently arrives mangled at the proxy.
func TestSplitProxyURL_EncodedPassword(t *testing.T) {
	_, auth := splitProxyURL("http://user:p%40ss%3Aword@proxy.example.org:3128")
	if auth != "user:p@ss:word" {
		t.Errorf("auth = %q, want %q", auth, "user:p@ss:word")
	}
}

func TestProxyArgs_ExplicitSettingsWin(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://from-env:3128")
	args := proxyArgs(ProxyConfig{HTTPSProxy: "explicit:8080"})
	mustContainPair(t, args, "-https-proxy", "explicit:8080")
	if strings.Contains(strings.Join(args, " "), "from-env") {
		t.Error("an explicit setting must not be overridden by the environment")
	}
}

func TestProxyArgs_FallsBackToEnvironment(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://http-proxy:3128")
	t.Setenv("HTTPS_PROXY", "http://https-proxy:3129")
	args := proxyArgs(ProxyConfig{})
	mustContainPair(t, args, "-proxy", "http-proxy:3128")
	mustContainPair(t, args, "-https-proxy", "https-proxy:3129")
}

// The lowercase spelling is the older convention and still common in container
// images, so it has to work too.
func TestProxyArgs_LowercaseEnvironment(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("https_proxy", "http://lower:3128")
	args := proxyArgs(ProxyConfig{})
	mustContainPair(t, args, "-https-proxy", "lower:3128")
}

func TestProxyArgs_AuthFromEnvVar(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://proxy:3128")
	t.Setenv(ProxyAuthEnvVar, "user:secret")
	args := proxyArgs(ProxyConfig{})
	mustContainPair(t, args, "-auth", "user:secret")
}

func TestProxyArgs_AuthDerivedFromProxyURL(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://user:secret@proxy:3128")
	args := proxyArgs(ProxyConfig{})
	mustContainPair(t, args, "-https-proxy", "proxy:3128")
	mustContainPair(t, args, "-auth", "user:secret")
}

// An explicit credential must beat one embedded in the proxy URL.
func TestProxyArgs_ExplicitAuthWinsOverURL(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://url-user:url-pass@proxy:3128")
	args := proxyArgs(ProxyConfig{Auth: "explicit:pass"})
	mustContainPair(t, args, "-auth", "explicit:pass")
	if strings.Contains(strings.Join(args, " "), "url-user") {
		t.Error("the URL credential must not win over an explicit one")
	}
}

func TestProxyArgs_NoneWhenUnset(t *testing.T) {
	for _, v := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", ProxyAuthEnvVar} {
		t.Setenv(v, "")
	}
	if args := proxyArgs(ProxyConfig{}); len(args) != 0 {
		t.Errorf("expected no proxy arguments, got %v", args)
	}
}

// Credentials without a proxy would put a secret in the child process's argv
// for no benefit at all.
func TestProxyArgs_AuthAloneIsDropped(t *testing.T) {
	for _, v := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy"} {
		t.Setenv(v, "")
	}
	args := proxyArgs(ProxyConfig{Auth: "user:secret"})
	if len(args) != 0 {
		t.Errorf("auth without a proxy must be dropped, got %v", args)
	}
}

func TestBuildArgs_IncludesProxy(t *testing.T) {
	for _, v := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", ProxyAuthEnvVar} {
		t.Setenv(v, "")
	}
	args := buildArgs("jar", []string{"in.json"}, "out.json", Options{
		FHIRVersion: "4.0.1",
		Proxy:       ProxyConfig{HTTPSProxy: "proxy:3128"},
	})
	mustContainPair(t, args, "-https-proxy", "proxy:3128")
}
