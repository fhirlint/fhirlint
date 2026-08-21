package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fhirlint/fhirlint/internal/suppress"
	"github.com/fhirlint/fhirlint/internal/validator"
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

func TestConfigFile_JurisdictionFromConfig(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, "jurisdiction: urn:iso:std:iso:3166#DE\n")

	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig: %v", err)
	}
	if got := viper.GetString("jurisdiction"); got != "urn:iso:std:iso:3166#DE" {
		t.Errorf("jurisdiction = %q, want %q", got, "urn:iso:std:iso:3166#DE")
	}
}

func TestConfigFile_DisplayIssuesAreWarningsFromConfig(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, "display-issues-are-warnings: true\n")

	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig: %v", err)
	}
	if got := viper.GetBool("display-issues-are-warnings"); !got {
		t.Error("display-issues-are-warnings should be true")
	}
}

func TestConfigFile_POFromConfig(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, "po:\n  - validator-messages-de.po\n")

	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig: %v", err)
	}
	po := viper.GetStringSlice("po")
	if len(po) != 1 || po[0] != "validator-messages-de.po" {
		t.Errorf("po = %v, want [validator-messages-de.po]", po)
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

// --- overrides ---

func TestLoadOverrides_ParsesIGsAndProfiles(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, `
overrides:
  - files: ["src/kbv/**"]
    ig:
      - kbv.basis#1.5.0
    profile:
      - http://example.com/StructureDefinition/MyProfile
`)
	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig: %v", err)
	}

	ovs, err := loadOverrides()
	if err != nil {
		t.Fatalf("loadOverrides: %v", err)
	}
	if len(ovs) != 1 {
		t.Fatalf("expected 1 override, got %d", len(ovs))
	}
	ov := ovs[0]
	if len(ov.Files) != 1 || ov.Files[0] != "src/kbv/**" {
		t.Errorf("files = %v, want [src/kbv/**]", ov.Files)
	}
	if len(ov.IGs) != 1 || ov.IGs[0] != "kbv.basis#1.5.0" {
		t.Errorf("ig = %v, want [kbv.basis#1.5.0]", ov.IGs)
	}
	if len(ov.Profiles) != 1 {
		t.Errorf("profile = %v, want 1 entry", ov.Profiles)
	}
}

func TestLoadOverrides_ParsesPostProcessingFields(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, `
overrides:
  - files: ["tests/fixtures/legacy/**"]
    severity: error
    fail-on: never
    best-practice: ignore
    suppress:
      - messageId: UNKNOWN_CODESYSTEM
`)
	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig: %v", err)
	}

	ovs, err := loadOverrides()
	if err != nil {
		t.Fatalf("loadOverrides: %v", err)
	}
	if len(ovs) != 1 {
		t.Fatalf("expected 1 override, got %d", len(ovs))
	}
	ov := ovs[0]
	if ov.Severity != "error" {
		t.Errorf("severity = %q, want %q", ov.Severity, "error")
	}
	if ov.FailOn != "never" {
		t.Errorf("fail-on = %q, want %q", ov.FailOn, "never")
	}
	if ov.BestPractice != "ignore" {
		t.Errorf("best-practice = %q, want %q", ov.BestPractice, "ignore")
	}
	if len(ov.Suppress) != 1 {
		t.Errorf("suppress = %v, want 1 rule", ov.Suppress)
	}
}

func TestLoadOverrides_EmptyWhenNotSet(t *testing.T) {
	resetViper(t)
	if ovs, err := loadOverrides(); err != nil || len(ovs) != 0 {
		t.Errorf("expected empty overrides, got %v", ovs)
	}
}

func TestLoadOverrides_SkipsEntryWithoutFiles(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, "overrides:\n  - ig: [kbv.basis#1.5.0]\n")
	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	_ = viper.ReadInConfig()
	if ovs, err := loadOverrides(); err != nil || len(ovs) != 0 {
		t.Errorf("expected override without files to be skipped, got %v", ovs)
	}
}

func TestMatchingOverrides_ReturnsMatchingEntries(t *testing.T) {
	ovs := []configOverride{
		{Files: []string{"src/kbv/**"}, IGs: []string{"kbv.basis#1.5.0"}},
		{Files: []string{"tests/invalid/**"}, FailOn: "never"},
		{Files: []string{"src/**"}, BestPractice: "ignore"},
	}
	matched := matchingOverrides("src/kbv/patient.json", ovs)
	if len(matched) != 2 {
		t.Fatalf("expected 2 matches, got %d: %v", len(matched), matched)
	}
	if matched[0].IGs[0] != "kbv.basis#1.5.0" {
		t.Errorf("first match should be kbv override")
	}
	if matched[1].BestPractice != "ignore" {
		t.Errorf("second match should be src/** override")
	}
}

func TestMatchingOverrides_NoMatchReturnsNil(t *testing.T) {
	ovs := []configOverride{{Files: []string{"src/kbv/**"}}}
	if matched := matchingOverrides("other/patient.json", ovs); len(matched) != 0 {
		t.Errorf("expected no matches, got %v", matched)
	}
}

func TestMergeOverrideOpts_AppendsIGsAndProfiles(t *testing.T) {
	base := validator.Options{
		IGs:      []string{"base.ig#1.0"},
		Profiles: []string{"http://base.profile"},
	}
	matched := []configOverride{
		{IGs: []string{"extra.ig#2.0"}, Profiles: []string{"http://extra.profile"}},
	}
	merged := mergeOverrideOpts(base, matched)
	if len(merged.IGs) != 2 || merged.IGs[1] != "extra.ig#2.0" {
		t.Errorf("IGs = %v, want base + extra", merged.IGs)
	}
	if len(merged.Profiles) != 2 || merged.Profiles[1] != "http://extra.profile" {
		t.Errorf("Profiles = %v, want base + extra", merged.Profiles)
	}
}

func TestMergeOverrideOpts_LaterBestPracticeWins(t *testing.T) {
	base := validator.Options{BestPractice: "warning"}
	matched := []configOverride{
		{BestPractice: "hint"},
		{BestPractice: "ignore"},
	}
	merged := mergeOverrideOpts(base, matched)
	if merged.BestPractice != "ignore" {
		t.Errorf("BestPractice = %q, want %q", merged.BestPractice, "ignore")
	}
}

func TestApplyOverridePostProcessing_SeverityFilter(t *testing.T) {
	ovs := []configOverride{{
		Files:    []string{"tests/**"},
		Severity: "error",
	}}
	r := &validator.Result{
		Filename: "tests/patient.json",
		Issues: []validator.Issue{
			{Severity: "information"},
			{Severity: "warning"},
			{Severity: "error"},
		},
	}
	applyOverridePostProcessing([]*validator.Result{r}, ovs)
	if len(r.Issues) != 1 || r.Issues[0].Severity != "error" {
		t.Errorf("issues after severity filter = %v, want [error]", r.Issues)
	}
}

func TestApplyOverridePostProcessing_FailOnNever(t *testing.T) {
	ovs := []configOverride{{
		Files:  []string{"tests/invalid/**"},
		FailOn: "never",
	}}
	r := &validator.Result{
		Filename: "tests/invalid/bad.json",
		Issues:   []validator.Issue{{Severity: "error"}},
	}
	neverFail := applyOverridePostProcessing([]*validator.Result{r}, ovs)
	if _, ok := neverFail["tests/invalid/bad.json"]; !ok {
		t.Error("expected tests/invalid/bad.json in neverFailPaths")
	}
}

func TestApplyOverridePostProcessing_SuppressRules(t *testing.T) {
	ovs := []configOverride{{
		Files:    []string{"tests/**"},
		Suppress: mustParseSuppressRules(t, []interface{}{"messageId:dom-6"}),
	}}
	r := &validator.Result{
		Filename: "tests/patient.json",
		Issues:   []validator.Issue{{Severity: "warning", MessageID: "dom-6"}},
	}
	applyOverridePostProcessing([]*validator.Result{r}, ovs)
	if len(r.Issues) != 0 {
		t.Errorf("expected 0 active issues after suppress, got %d", len(r.Issues))
	}
	if len(r.Suppressed) != 1 {
		t.Errorf("expected 1 suppressed issue, got %d", len(r.Suppressed))
	}
}

func TestCheckExitCode_NeverFailPaths_Excluded(t *testing.T) {
	flagFailOn = "error"
	results := []*validator.Result{
		{Filename: "src/ok.json", Issues: []validator.Issue{{Severity: "error"}}},
		{Filename: "tests/invalid/bad.json", Issues: []validator.Issue{{Severity: "error"}}},
	}
	neverFail := map[string]struct{}{"tests/invalid/bad.json": {}}
	if err := checkExitCode(results, neverFail); err == nil {
		t.Error("expected error from src/ok.json which is not excluded")
	}
	neverFail["src/ok.json"] = struct{}{}
	if err := checkExitCode(results, neverFail); err != nil {
		t.Errorf("expected nil when all failing files are excluded, got: %v", err)
	}
}

func mustParseSuppressRules(t *testing.T, raw []interface{}) []suppress.Rule {
	t.Helper()
	rules, err := parseSuppressFromConfig(raw)
	if err != nil {
		t.Fatalf("parseSuppressFromConfig: %v", err)
	}
	return rules
}

func TestConfigFile_ValidatorArgFromConfig(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, "validator-arg:\n  - -some-new-flag\n  - value\n")

	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	_ = viper.ReadInConfig()

	got := viper.GetStringSlice("validator-arg")
	if len(got) != 2 || got[0] != "-some-new-flag" || got[1] != "value" {
		t.Errorf("validator-arg = %v, want [-some-new-flag value]", got)
	}
}

func TestLoadOverrides_InvalidSuppressRuleIsAnError(t *testing.T) {
	// Regression guard for #254: these used to be discarded in silence, so a
	// typo simply made the override stop working with no indication why.
	cases := []struct {
		name string
		yaml string
	}{
		{"malformed expires", "overrides:\n  - files: \"*.json\"\n    suppress:\n      - constraint: dom-6\n        expires: 31.12.2026\n"},
		{"rule with no type", "overrides:\n  - files: \"*.json\"\n    suppress:\n      - reason: \"no type\"\n"},
		{"invalid pattern", "overrides:\n  - files: \"*.json\"\n    suppress:\n      - pattern: \"[unclosed\"\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetViper(t)
			dir := t.TempDir()
			writeConfigFile(t, dir, tc.yaml)
			viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
			_ = viper.ReadInConfig()

			if _, err := loadOverrides(); err == nil {
				t.Error("expected an error, got nil — the rule was silently discarded")
			}
		})
	}
}

func TestLoadOverrides_ValidSuppressRuleIsKept(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir,
		"overrides:\n  - files: \"*.json\"\n    suppress:\n      - constraint: dom-6\n        expires: 2099-12-31\n")
	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	_ = viper.ReadInConfig()

	ovs, err := loadOverrides()
	if err != nil {
		t.Fatalf("loadOverrides: %v", err)
	}
	if len(ovs) != 1 || len(ovs[0].Suppress) != 1 {
		t.Fatalf("expected one override with one rule, got %+v", ovs)
	}
	if ovs[0].Suppress[0].Expires.IsZero() {
		t.Error("expiry date was not parsed")
	}
}

func TestConfigFile_ValidationTimeoutFromConfig(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, "validation-timeout: 2m\n")

	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig: %v", err)
	}

	if got := viper.GetString("validation-timeout"); got != "2m" {
		t.Errorf("validation-timeout = %q, want %q", got, "2m")
	}
}

func TestConfigFile_MaxMessagesFromConfig(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, "max-messages: 500\n")

	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig: %v", err)
	}

	if got := viper.GetInt("max-messages"); got != 500 {
		t.Errorf("max-messages = %d, want %d", got, 500)
	}
}

// --- severity-override (#311) ---

func TestBuildSeverityOverrides_FromConfig(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, `severity-override:
  - messageId: Rule_bdl_1
    severity: warning
    reason: "upstream profile defect"
    expires: 2026-12-31
`)
	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig: %v", err)
	}

	rules, err := buildSeverityOverrides()
	if err != nil {
		t.Fatalf("buildSeverityOverrides: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
	if rules[0].Type != "messageId" || rules[0].Value != "Rule_bdl_1" {
		t.Errorf("selector = %s:%s", rules[0].Type, rules[0].Value)
	}
	if rules[0].To != "warning" {
		t.Errorf("To = %q, want warning", rules[0].To)
	}
	if rules[0].Expires.Format("2006-01-02") != "2026-12-31" {
		t.Errorf("Expires = %v", rules[0].Expires)
	}
}

func TestBuildSeverityOverrides_Unset(t *testing.T) {
	resetViper(t)
	rules, err := buildSeverityOverrides()
	if err != nil {
		t.Fatalf("buildSeverityOverrides: %v", err)
	}
	if rules != nil {
		t.Errorf("got %v, want nil when the key is absent", rules)
	}
}

func TestBuildSeverityOverrides_Rejects(t *testing.T) {
	cases := []struct {
		name string
		yaml string
	}{
		{"not a list", "severity-override:\n  messageId: x\n"},
		{"string shorthand", "severity-override:\n  - \"messageId:x\"\n"},
		{"no selector", "severity-override:\n  - severity: warning\n"},
		{"no severity", "severity-override:\n  - messageId: x\n"},
		{"unknown severity", "severity-override:\n  - messageId: x\n    severity: critical\n"},
		{"malformed expires", "severity-override:\n  - messageId: x\n    severity: warning\n    expires: 31.12.2026\n"},
		{"invalid pattern", "severity-override:\n  - pattern: \"[unclosed\"\n    severity: warning\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetViper(t)
			dir := t.TempDir()
			writeConfigFile(t, dir, tc.yaml)
			viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
			_ = viper.ReadInConfig()

			if _, err := buildSeverityOverrides(); err == nil {
				t.Error("expected an error, got nil")
			}
		})
	}
}

func TestBuildSeverityOverrides_RequireReason(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, "severity-override:\n  - messageId: x\n    severity: warning\n")
	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	_ = viper.ReadInConfig()

	flagRequireSuppressReason = true
	t.Cleanup(func() { flagRequireSuppressReason = false })

	if _, err := buildSeverityOverrides(); err == nil {
		t.Error("expected require-suppress-reason to reject a rule with no reason")
	}
}

// A downgraded error must actually stop failing the build — the exit code is
// where a re-levelling either works or is cosmetic.
func TestSeverityOverride_DrivesExitCode(t *testing.T) {
	flagFailOn = "error"
	results := []*validator.Result{{
		Filename: "a.json",
		Valid:    false,
		Issues:   []validator.Issue{{Severity: "error", MessageID: "Rule_bdl_1"}},
	}}

	if err := checkExitCode(results, nil); err == nil {
		t.Fatal("expected the error to fail the run before any override")
	}

	rule, err := suppress.ParseSeverityMap(map[string]interface{}{
		"messageId": "Rule_bdl_1", "severity": "warning",
	})
	if err != nil {
		t.Fatal(err)
	}
	suppress.ApplySeverity(results, []suppress.SeverityRule{rule})

	if err := checkExitCode(results, nil); err != nil {
		t.Errorf("downgraded finding should not fail the run, got: %v", err)
	}
	if err := checkMaxWarnings(results, nil); err != nil {
		t.Errorf("unexpected max-warnings failure: %v", err)
	}

	// …and the other direction: an upgrade must start failing it.
	flagMaxWarnings = 0
	t.Cleanup(func() { flagMaxWarnings = -1 })
	if err := checkMaxWarnings(results, nil); err == nil {
		t.Error("the downgraded finding is now a warning and should breach --max-warnings 0")
	}
}

func TestConfigFile_GroupFromConfig(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, "group: true\n")

	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig: %v", err)
	}

	if !viper.GetBool("group") {
		t.Error("group = false, want true from config")
	}
}

func TestConfigFile_RedactFromConfig(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, "redact: true\n")

	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig: %v", err)
	}

	// Setting it in the config rather than per invocation is the point: a new
	// CI job cannot forget a flag it never has to pass.
	if !viper.GetBool("redact") {
		t.Error("redact = false, want true")
	}
}

func TestConfigFile_CodeSystemSizeLimitFromConfig(t *testing.T) {
	resetViper(t)
	dir := t.TempDir()
	writeConfigFile(t, dir, "codesystem-size-limit: 5000\n")

	viper.SetConfigFile(filepath.Join(dir, "fhirlint.yml"))
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig: %v", err)
	}

	if got := viper.GetInt("codesystem-size-limit"); got != 5000 {
		t.Errorf("codesystem-size-limit = %d, want 5000", got)
	}
}
