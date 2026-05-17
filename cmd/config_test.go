package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func writeConfigFile(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "fhirlint.yml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func resetViper(t *testing.T) {
	t.Helper()
	viper.Reset()
	t.Cleanup(func() { viper.Reset() })
}

func TestConfigFile_SeverityFromConfig(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, "severity: warning\n")

	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig: %v", err)
	}

	if got := viper.GetString("severity"); got != "warning" {
		t.Errorf("severity = %q, want %q", got, "warning")
	}
}

func TestConfigFile_FailOnFromConfig(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, "fail-on: warning\n")

	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	_ = viper.ReadInConfig()

	if got := viper.GetString("fail-on"); got != "warning" {
		t.Errorf("fail-on = %q, want %q", got, "warning")
	}
}

func TestConfigFile_ProfilesFromConfig(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, "profile:\n  - kbv-basis\n  - mii\n")

	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	_ = viper.ReadInConfig()

	profiles := viper.GetStringSlice("profile")
	if len(profiles) != 2 || profiles[0] != "kbv-basis" || profiles[1] != "mii" {
		t.Errorf("profile = %v, want [kbv-basis mii]", profiles)
	}
}

func TestConfigFile_IGsFromConfig(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, "ig:\n  - kbv.basis#1.5.0\n")

	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	_ = viper.ReadInConfig()

	igs := viper.GetStringSlice("ig")
	if len(igs) != 1 || igs[0] != "kbv.basis#1.5.0" {
		t.Errorf("ig = %v, want [kbv.basis#1.5.0]", igs)
	}
}

func TestConfigFile_MissingFileIsIgnored(t *testing.T) {
	resetViper(t)
	viper.SetConfigFile("/nonexistent/fhirlint.yml")

	// Must not panic or return a hard error — missing config is optional
	err := viper.ReadInConfig()
	if err == nil {
		t.Skip("unexpected: config file found at /nonexistent/fhirlint.yml")
	}
	// As long as it doesn't panic, the test passes
}

func TestConfigFile_FHIRVersionFromConfig(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, "fhir-version: 4.3.0\n")

	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	_ = viper.ReadInConfig()

	if got := viper.GetString("fhir-version"); got != "4.3.0" {
		t.Errorf("fhir-version = %q, want %q", got, "4.3.0")
	}
}

func TestConfigFile_IgnoreFromConfig(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, "ignore:\n  - \"$.meta.tag\"\n  - \"$.text\"\n")

	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	_ = viper.ReadInConfig()

	ignore := viper.GetStringSlice("ignore")
	if len(ignore) != 2 {
		t.Errorf("ignore = %v, want 2 entries", ignore)
	}
}

func TestConfigFile_AllSupportedKeys(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, `
severity: warning
fail-on: error
fhir-version: 4.0.1
profile:
  - kbv-basis
ig:
  - kbv.basis#1.5.0
format:
  - terminal
ignore:
  - "$.meta.tag"
`)

	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig: %v", err)
	}

	checks := map[string]string{
		"severity":     "warning",
		"fail-on":      "error",
		"fhir-version": "4.0.1",
	}
	for key, want := range checks {
		if got := viper.GetString(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestConfigFile_TxCacheFromConfig(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, "tx-cache: .fhirlint-tx-cache/\n")

	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig: %v", err)
	}

	if got := viper.GetString("tx-cache"); got != ".fhirlint-tx-cache/" {
		t.Errorf("tx-cache = %q, want %q", got, ".fhirlint-tx-cache/")
	}
}

func TestConfigFile_LocaleFromConfig(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, "locale: de\n")

	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig: %v", err)
	}
	if got := viper.GetString("locale"); got != "de" {
		t.Errorf("locale = %q, want %q", got, "de")
	}
}

func TestConfigFile_AllowExampleURLsFromConfig(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, "allow-example-urls: true\n")

	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig: %v", err)
	}
	if got := viper.GetBool("allow-example-urls"); !got {
		t.Error("allow-example-urls should be true")
	}
}

func TestConfigFile_ExcludeFromConfig(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, "exclude:\n  - vendor/\n  - tests/invalid/**\n")

	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig: %v", err)
	}
	got := viper.GetStringSlice("exclude")
	if len(got) != 2 {
		t.Errorf("expected 2 exclude patterns, got %d: %v", len(got), got)
	}
}

func TestConfigFile_MaxWarningsFromConfig(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, "max-warnings: 10\n")

	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig: %v", err)
	}
	if got := viper.GetInt("max-warnings"); got != 10 {
		t.Errorf("max-warnings = %d, want 10", got)
	}
}

func TestConfigFile_TxLogFromConfig(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, "tx-log: tx.log\n")

	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig: %v", err)
	}
	if got := viper.GetString("tx-log"); got != "tx.log" {
		t.Errorf("tx-log = %q, want %q", got, "tx.log")
	}
}

func TestConfigFile_AllowInsecureTxFromConfig(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, "allow-insecure-tx: true\n")

	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig: %v", err)
	}
	if got := viper.GetBool("allow-insecure-tx"); !got {
		t.Error("allow-insecure-tx should be true")
	}
}

func TestConfigFile_BestPracticeFromConfig(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, "best-practice: ignore\n")

	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig: %v", err)
	}

	if got := viper.GetString("best-practice"); got != "ignore" {
		t.Errorf("best-practice = %q, want %q", got, "ignore")
	}
}

func TestConfigFile_URLFromConfig(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, "url:\n  - http://example.com/fhir/Patient/1\n  - http://example.com/fhir/Patient/2\n")

	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig: %v", err)
	}

	got := viper.GetStringSlice("url")
	want := []string{"http://example.com/fhir/Patient/1", "http://example.com/fhir/Patient/2"}
	if len(got) != len(want) {
		t.Fatalf("url len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("url[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
