package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/jarsettings"
	"github.com/fhirlint/fhirlint/internal/txauth"
	"github.com/fhirlint/fhirlint/internal/validator"
)

func readSettings(t *testing.T, path string) []jarsettings.Server {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // path produced by the code under test
	if err != nil {
		t.Fatalf("reading settings: %v", err)
	}
	var file struct {
		Servers []jarsettings.Server `json:"servers"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("settings are not valid JSON: %v\n%s", err, data)
	}
	return file.Servers
}

func TestApplyTerminologyAuth_WritesTheCredential(t *testing.T) {
	t.Setenv(txauth.TokenEnvVar, "s3cr3t")
	opts := validator.Options{TerminologyServer: "https://tx.example.org/r4"}
	var out bytes.Buffer

	cleanup, err := applyTerminologyAuth(&opts, nil, &out)
	if err != nil {
		t.Fatalf("applyTerminologyAuth: %v", err)
	}
	defer cleanup()

	if opts.FHIRSettings == "" {
		t.Fatal("no settings file was generated")
	}
	servers := readSettings(t, opts.FHIRSettings)
	if len(servers) != 1 {
		t.Fatalf("got %d server entries, want 1", len(servers))
	}
	if servers[0].URL != "https://tx.example.org/r4" {
		t.Errorf("url = %q, want the configured terminology server", servers[0].URL)
	}
	if servers[0].AuthenticationType != string(txauth.ModeToken) || servers[0].Token != "s3cr3t" {
		t.Errorf("entry = %+v, want a token credential", servers[0])
	}

	// The run says it authenticated, without printing the secret.
	if !strings.Contains(out.String(), "Authenticating to https://tx.example.org/r4") {
		t.Errorf("output = %q, want the run to say it authenticated", out.String())
	}
	if strings.Contains(out.String(), "s3cr3t") {
		t.Errorf("output = %q, leaks the credential", out.String())
	}
}

// The default terminology server is the public tx.fhir.org. A credential that
// happens to be in the environment must not travel there.
func TestApplyTerminologyAuth_OnlyForAnExplicitServer(t *testing.T) {
	t.Setenv(txauth.TokenEnvVar, "s3cr3t")

	for _, opts := range []validator.Options{
		{},                          // no server named: the JAR default
		{NoTerminologyServer: true}, // terminology switched off
		{TerminologyServer: "https://tx.example.org", FHIRSettings: "/already/set.json"}, // replay owns the file
	} {
		got := opts
		cleanup, err := applyTerminologyAuth(&got, nil, io.Discard)
		if err != nil {
			t.Fatalf("applyTerminologyAuth(%+v): %v", opts, err)
		}
		cleanup()
		if got.FHIRSettings != opts.FHIRSettings {
			t.Errorf("opts %+v gained a settings file at %q", opts, got.FHIRSettings)
		}
	}
}

// A bearer token over plain HTTP is a leaked bearer token.
func TestApplyTerminologyAuth_RefusesPlainHTTP(t *testing.T) {
	t.Setenv(txauth.TokenEnvVar, "s3cr3t")
	opts := validator.Options{TerminologyServer: "http://tx.internal/r4"}

	_, err := applyTerminologyAuth(&opts, nil, io.Discard)
	if err == nil {
		t.Fatal("err = nil for credentials over plain HTTP, want a refusal")
	}
	if !strings.Contains(err.Error(), "plain HTTP") {
		t.Errorf("err = %q, want it to name the reason", err)
	}
}

// Both fhirlint and the user would be writing the same -fhir-settings.
func TestApplyTerminologyAuth_RejectsAUserSettingsFile(t *testing.T) {
	t.Setenv(txauth.TokenEnvVar, "s3cr3t")
	opts := validator.Options{TerminologyServer: "https://tx.example.org"}

	_, err := applyTerminologyAuth(&opts, []string{"-fhir-settings", "mine.json"}, io.Discard)
	if err == nil {
		t.Fatal("err = nil when the user passes their own -fhir-settings")
	}
}

func TestApplyTerminologyAuth_NoCredentialsIsNotAnError(t *testing.T) {
	opts := validator.Options{TerminologyServer: "https://tx.example.org"}
	cleanup, err := applyTerminologyAuth(&opts, nil, io.Discard)
	if err != nil {
		t.Fatalf("applyTerminologyAuth: %v", err)
	}
	cleanup()
	if opts.FHIRSettings != "" {
		t.Errorf("settings file written without credentials: %q", opts.FHIRSettings)
	}
}
