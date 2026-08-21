package txauth

import (
	"strings"
	"testing"
)

func lookup(vars map[string]string) func(string) string {
	return func(name string) string { return vars[name] }
}

func TestFromEnv_EachMode(t *testing.T) {
	cases := []struct {
		name       string
		vars       map[string]string
		wantMode   Mode
		wantHeader [2]string
	}{
		{
			name:       "bearer token",
			vars:       map[string]string{TokenEnvVar: "abc123"},
			wantMode:   ModeToken,
			wantHeader: [2]string{"Authorization", "Bearer abc123"},
		},
		{
			name:       "api key",
			vars:       map[string]string{APIKeyEnvVar: "k-42"},
			wantMode:   ModeAPIKey,
			wantHeader: [2]string{"Api-Key", "k-42"},
		},
		{
			// base64("user:pass") — the validator sends exactly this.
			name:       "basic",
			vars:       map[string]string{BasicEnvVar: "user:pass"},
			wantMode:   ModeBasic,
			wantHeader: [2]string{"Authorization", "Basic dXNlcjpwYXNz"},
		},
		{
			name:     "nothing set",
			vars:     nil,
			wantMode: ModeNone,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := fromLookup(lookup(tc.vars))
			if err != nil {
				t.Fatalf("fromLookup: %v", err)
			}
			if got.Mode != tc.wantMode {
				t.Fatalf("mode = %q, want %q", got.Mode, tc.wantMode)
			}
			if tc.wantMode == ModeNone {
				if !got.Empty() {
					t.Error("Empty() = false for an empty environment")
				}
				return
			}
			name, value := got.Header()
			if name != tc.wantHeader[0] || value != tc.wantHeader[1] {
				t.Errorf("header = %q: %q, want %q: %q", name, value, tc.wantHeader[0], tc.wantHeader[1])
			}
		})
	}
}

// Two credentials in the environment means one of them is stale. Picking a
// winner silently is how the wrong one gets used for months.
func TestFromEnv_RefusesMoreThanOne(t *testing.T) {
	_, err := fromLookup(lookup(map[string]string{
		TokenEnvVar:  "abc",
		APIKeyEnvVar: "k-42",
	}))
	if err == nil {
		t.Fatal("err = nil with two credentials set, want a refusal")
	}
	for _, want := range []string{TokenEnvVar, APIKeyEnvVar} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %q, want it to name %q", err, want)
		}
	}
}

func TestFromEnv_BasicNeedsAColon(t *testing.T) {
	if _, err := fromLookup(lookup(map[string]string{BasicEnvVar: "userwithoutpassword"})); err == nil {
		t.Error("err = nil for a basic credential without a colon")
	}
	// An empty password is legitimate; an empty username is not.
	got, err := fromLookup(lookup(map[string]string{BasicEnvVar: "user:"}))
	if err != nil {
		t.Fatalf("fromLookup: %v", err)
	}
	if got.Username != "user" || got.Password != "" {
		t.Errorf("got %q/%q, want user with an empty password", got.Username, got.Password)
	}
}

func TestFromEnv_IgnoresWhitespaceOnlyValues(t *testing.T) {
	got, err := fromLookup(lookup(map[string]string{TokenEnvVar: "   "}))
	if err != nil {
		t.Fatalf("fromLookup: %v", err)
	}
	if !got.Empty() {
		t.Errorf("mode = %q, want a whitespace-only value treated as unset", got.Mode)
	}
}

// Describe goes to stderr on every authenticated run, so it must name the
// source without printing the secret.
func TestDescribe_DoesNotLeakTheCredential(t *testing.T) {
	cases := []Credentials{
		{Mode: ModeToken, Token: "s3cr3t-token"},
		{Mode: ModeAPIKey, APIKey: "s3cr3t-key"},
		{Mode: ModeBasic, Username: "alice", Password: "s3cr3t-pass"},
	}
	for _, c := range cases {
		got := c.Describe()
		if strings.Contains(got, "s3cr3t") {
			t.Errorf("Describe() = %q, leaks the credential", got)
		}
		if got == "" || got == "none" {
			t.Errorf("Describe() = %q, want it to name the source", got)
		}
	}
	if got := (Credentials{}).Describe(); got != "none" {
		t.Errorf("Describe() = %q for empty credentials, want %q", got, "none")
	}
}
