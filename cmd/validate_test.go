package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/input"
	"github.com/fhirlint/fhirlint/internal/validator"
)

// --- checkExitCode ---

func makeResults(severities ...string) []*validator.Result {
	issues := make([]validator.Issue, len(severities))
	for i, s := range severities {
		issues[i] = validator.Issue{Severity: s}
	}
	return []*validator.Result{{Issues: issues}}
}

func TestCheckExitCode_Never_AlwaysPasses(t *testing.T) {
	flagFailOn = "never"
	if err := checkExitCode(makeResults("error", "fatal")); err != nil {
		t.Errorf("expected nil for never, got: %v", err)
	}
}

func TestCheckExitCode_FailOnError_PassesWarning(t *testing.T) {
	flagFailOn = "error"
	if err := checkExitCode(makeResults("warning")); err != nil {
		t.Errorf("expected nil for warning with fail-on=error, got: %v", err)
	}
}

func TestCheckExitCode_FailOnError_FailsOnError(t *testing.T) {
	flagFailOn = "error"
	if err := checkExitCode(makeResults("error")); err == nil {
		t.Error("expected error for error with fail-on=error")
	}
}

func TestCheckExitCode_FailOnError_FailsOnFatal(t *testing.T) {
	flagFailOn = "error"
	if err := checkExitCode(makeResults("fatal")); err == nil {
		t.Error("expected error for fatal with fail-on=error")
	}
}

func TestCheckExitCode_FailOnWarning_FailsOnWarning(t *testing.T) {
	flagFailOn = "warning"
	if err := checkExitCode(makeResults("warning")); err == nil {
		t.Error("expected error for warning with fail-on=warning")
	}
}

func TestCheckExitCode_FailOnWarning_PassesInformation(t *testing.T) {
	flagFailOn = "warning"
	if err := checkExitCode(makeResults("information")); err != nil {
		t.Errorf("expected nil for information with fail-on=warning, got: %v", err)
	}
}

func TestCheckExitCode_FailOnInformation_FailsOnInformation(t *testing.T) {
	flagFailOn = "information"
	if err := checkExitCode(makeResults("information")); err == nil {
		t.Error("expected error for information with fail-on=information")
	}
}

func TestCheckExitCode_UnknownValue_ReturnsError(t *testing.T) {
	flagFailOn = "typo"
	if err := checkExitCode(makeResults()); err == nil {
		t.Error("expected error for unknown fail-on value")
	}
}

// --- profile-map helpers ---

func TestIsFilenameGlob(t *testing.T) {
	cases := []struct {
		pattern string
		want    bool
	}{
		{"Patient", false},
		{"MedicationRequest", false},
		{"tests/fixtures/*.json", true},
		{"vendor/**", true},
		{"*.xml", true},
	}
	for _, tc := range cases {
		if got := isFilenameGlob(tc.pattern); got != tc.want {
			t.Errorf("isFilenameGlob(%q) = %v, want %v", tc.pattern, got, tc.want)
		}
	}
}

func TestPeekResourceType_ValidJSON(t *testing.T) {
	f, err := os.CreateTemp("", "peek-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	_, _ = f.WriteString(`{"resourceType":"Patient","id":"123"}`)
	_ = f.Close()

	if got := peekResourceType(f.Name()); got != "Patient" {
		t.Errorf("expected Patient, got %q", got)
	}
}

func TestPeekResourceType_Missing_ReturnsEmpty(t *testing.T) {
	if got := peekResourceType("/nonexistent/file.json"); got != "" {
		t.Errorf("expected empty for missing file, got %q", got)
	}
}

func TestResolveProfilesForPath_ResourceType(t *testing.T) {
	f, err := os.CreateTemp("", "res-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	_, _ = f.WriteString(`{"resourceType":"Patient"}`)
	_ = f.Close()

	pm := map[string][]string{
		"Patient": {"http://example.com/patient-profile"},
	}
	got := resolveProfilesForPath(f.Name(), pm)
	if len(got) != 1 || got[0] != "http://example.com/patient-profile" {
		t.Errorf("unexpected profiles: %v", got)
	}
}

func TestResolveProfilesForPath_NoMatch_ReturnsNil(t *testing.T) {
	f, err := os.CreateTemp("", "res-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	_, _ = f.WriteString(`{"resourceType":"Observation"}`)
	_ = f.Close()

	pm := map[string][]string{
		"Patient": {"http://example.com/patient-profile"},
	}
	if got := resolveProfilesForPath(f.Name(), pm); len(got) != 0 {
		t.Errorf("expected no profiles, got: %v", got)
	}
}

// --- matchesExclude ---

func TestMatchesExclude_TrailingSlash(t *testing.T) {
	cases := []struct {
		relPath string
		pattern string
		want    bool
	}{
		{"vendor/foo.json", "vendor/", true},
		{"vendor", "vendor/", true},
		{"src/vendor/foo.json", "vendor/", false},
		{"tests/fixtures/legacy/a.json", "tests/fixtures/legacy/**", true},
		{"tests/fixtures/other/a.json", "tests/fixtures/legacy/**", false},
		{"src/generated/foo.json", "src/generated/*.json", true},
		{"src/generated/sub/foo.json", "src/generated/*.json", false},
		{"foo.json", "*.json", true},
		{"src/foo.json", "*.json", true},
		{"src/foo.xml", "*.json", false},
		{"src/exact.json", "src/exact.json", true},
		{"src/other.json", "src/exact.json", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got := matchesExclude(tc.relPath, tc.pattern)
		if got != tc.want {
			t.Errorf("matchesExclude(%q, %q) = %v, want %v", tc.relPath, tc.pattern, got, tc.want)
		}
	}
}

func TestCollectFHIRPaths_ExcludeDir(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "vendor"), 0750)
	_ = os.WriteFile(filepath.Join(dir, "vendor", "v.json"), []byte("{}"), 0600)
	_ = os.WriteFile(filepath.Join(dir, "keep.json"), []byte("{}"), 0600)

	in := &input.Input{Source: input.SourceDir, Path: dir}
	paths, err := collectFHIRPaths(in, []string{"vendor/"})
	if err != nil {
		t.Fatalf("collectFHIRPaths error: %v", err)
	}
	for _, p := range paths {
		if strings.Contains(filepath.ToSlash(p), "/vendor/") {
			t.Errorf("excluded path should not appear: %s", p)
		}
	}
	if len(paths) != 1 {
		t.Errorf("expected 1 path, got %d: %v", len(paths), paths)
	}
}

func TestCollectFHIRPaths_ExcludeGlob(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "keep.xml"), []byte("<r/>"), 0600)
	_ = os.WriteFile(filepath.Join(dir, "skip.json"), []byte("{}"), 0600)

	in := &input.Input{Source: input.SourceDir, Path: dir}
	paths, err := collectFHIRPaths(in, []string{"*.json"})
	if err != nil {
		t.Fatalf("collectFHIRPaths error: %v", err)
	}
	for _, p := range paths {
		if strings.HasSuffix(p, ".json") {
			t.Errorf("excluded .json file should not appear: %s", p)
		}
	}
}

func TestLoadIgnoreFile_ParsesPatterns(t *testing.T) {
	f, err := os.CreateTemp("", ".fhirlintignore-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	content := "# comment\nvendor/\n\nsrc/generated/*.json\n"
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	patterns, err := loadIgnoreFile(f.Name())
	if err != nil {
		t.Fatalf("loadIgnoreFile error: %v", err)
	}
	if len(patterns) != 2 {
		t.Fatalf("expected 2 patterns, got %d: %v", len(patterns), patterns)
	}
	if patterns[0] != "vendor/" || patterns[1] != "src/generated/*.json" {
		t.Errorf("unexpected patterns: %v", patterns)
	}
}

func TestLoadIgnoreFile_Missing_ReturnsNil(t *testing.T) {
	patterns, err := loadIgnoreFile("/nonexistent/.fhirlintignore")
	if err != nil {
		t.Errorf("expected nil error for missing file, got: %v", err)
	}
	if patterns != nil {
		t.Errorf("expected nil patterns for missing file, got: %v", patterns)
	}
}

// --- checkMaxWarnings ---

func TestCheckMaxWarnings_Disabled_AlwaysPasses(t *testing.T) {
	flagMaxWarnings = -1
	if err := checkMaxWarnings(makeResults("warning", "warning", "warning")); err != nil {
		t.Errorf("expected nil when disabled, got: %v", err)
	}
}

func TestCheckMaxWarnings_WithinThreshold_Passes(t *testing.T) {
	flagMaxWarnings = 3
	if err := checkMaxWarnings(makeResults("warning", "warning")); err != nil {
		t.Errorf("expected nil when count <= max, got: %v", err)
	}
}

func TestCheckMaxWarnings_ExactThreshold_Passes(t *testing.T) {
	flagMaxWarnings = 2
	if err := checkMaxWarnings(makeResults("warning", "warning")); err != nil {
		t.Errorf("expected nil when count == max, got: %v", err)
	}
}

func TestCheckMaxWarnings_ExceedsThreshold_Fails(t *testing.T) {
	flagMaxWarnings = 1
	if err := checkMaxWarnings(makeResults("warning", "warning")); err == nil {
		t.Error("expected error when warning count exceeds max")
	}
}

func TestCheckMaxWarnings_OnlyCountsWarnings(t *testing.T) {
	flagMaxWarnings = 0
	// errors and information should not count toward the warning limit
	if err := checkMaxWarnings(makeResults("error", "information", "fatal")); err != nil {
		t.Errorf("expected nil when no warnings, got: %v", err)
	}
}

func TestCheckMaxWarnings_ZeroThreshold_FailsOnAnyWarning(t *testing.T) {
	flagMaxWarnings = 0
	if err := checkMaxWarnings(makeResults("warning")); err == nil {
		t.Error("expected error when max-warnings=0 and one warning present")
	}
}

// --- gjsonPath ---

func TestGjsonPath_DollarDotPrefix(t *testing.T) {
	got := gjsonPath("$.foo.bar")
	if got != "foo.bar" {
		t.Errorf("gjsonPath(\"$.foo.bar\") = %q, want %q", got, "foo.bar")
	}
}

func TestGjsonPath_DollarOnlyPrefix(t *testing.T) {
	got := gjsonPath("$")
	if got != "" {
		t.Errorf("gjsonPath(\"$\") = %q, want empty string", got)
	}
}

func TestGjsonPath_ArrayBrackets(t *testing.T) {
	got := gjsonPath("$.entry[0].resource")
	if got != "entry.0.resource" {
		t.Errorf("gjsonPath(\"$.entry[0].resource\") = %q, want \"entry.0.resource\"", got)
	}
}

func TestGjsonPath_NoPrefix(t *testing.T) {
	got := gjsonPath("foo.bar")
	if got != "foo.bar" {
		t.Errorf("gjsonPath(\"foo.bar\") = %q, want \"foo.bar\"", got)
	}
}

func TestGjsonPath_DeepNested(t *testing.T) {
	got := gjsonPath("$.data.fhir")
	if got != "data.fhir" {
		t.Errorf("gjsonPath(\"$.data.fhir\") = %q, want \"data.fhir\"", got)
	}
}

// --- deleteNestedKey ---

func jsonObj(t *testing.T, s string) map[string]interface{} {
	t.Helper()
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(s), &obj); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	return obj
}

func TestDeleteNestedKey_TopLevel(t *testing.T) {
	obj := jsonObj(t, `{"name":"Hans","gender":"male"}`)
	deleteNestedKey(obj, []string{"gender"})
	if _, ok := obj["gender"]; ok {
		t.Error("expected 'gender' to be deleted")
	}
	if _, ok := obj["name"]; !ok {
		t.Error("expected 'name' to be preserved")
	}
}

func TestDeleteNestedKey_Nested(t *testing.T) {
	obj := jsonObj(t, `{"meta":{"tag":["foo"],"profile":["bar"]}}`)
	deleteNestedKey(obj, []string{"meta", "tag"})
	meta := obj["meta"].(map[string]interface{})
	if _, ok := meta["tag"]; ok {
		t.Error("expected 'meta.tag' to be deleted")
	}
	if _, ok := meta["profile"]; !ok {
		t.Error("expected 'meta.profile' to be preserved")
	}
}

func TestDeleteNestedKey_NonExistentKey_NoError(t *testing.T) {
	obj := jsonObj(t, `{"name":"Hans"}`)
	deleteNestedKey(obj, []string{"nonexistent"})
	deleteNestedKey(obj, []string{"name", "deep", "path"})
}

func TestDeleteNestedKey_EmptyParts_NoError(t *testing.T) {
	obj := jsonObj(t, `{"name":"Hans"}`)
	deleteNestedKey(obj, []string{})
}

func TestDeleteNestedKey_InArray(t *testing.T) {
	obj := jsonObj(t, `{"issue":[{"severity":"error","code":"invalid"},{"severity":"warning","code":"invariant"}]}`)
	deleteNestedKey(obj, []string{"issue", "code"})
	issues := obj["issue"].([]interface{})
	for i, item := range issues {
		m := item.(map[string]interface{})
		if _, ok := m["code"]; ok {
			t.Errorf("expected 'code' deleted in issue[%d]", i)
		}
		if _, ok := m["severity"]; !ok {
			t.Errorf("expected 'severity' preserved in issue[%d]", i)
		}
	}
}

// --- preprocessJSON ---

func tempJSONFile(t *testing.T, content string) *input.Input {
	t.Helper()
	f, err := os.CreateTemp("", "fhirlint-preprocess-*.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(f.Name()) })
	return &input.Input{Path: f.Name(), Label: f.Name()}
}

func TestPreprocessJSON_Extract(t *testing.T) {
	in := tempJSONFile(t, `{"data":{"fhir":{"resourceType":"Patient","id":"1"}}}`)
	flagExtract = "$.data.fhir"
	flagIgnore = nil
	t.Cleanup(func() { flagExtract = ""; flagIgnore = nil })

	if err := preprocessJSON(in); err != nil {
		t.Fatalf("preprocessJSON() error: %v", err)
	}

	data, err := os.ReadFile(in.Path)
	if err != nil {
		t.Fatalf("reading result: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("result is not valid JSON after extract: %v", err)
	}
	if result["resourceType"] != "Patient" {
		t.Errorf("expected resourceType=Patient after extract, got %v", result["resourceType"])
	}
	if _, ok := result["data"]; ok {
		t.Error("expected wrapper 'data' key to be gone after extract")
	}
}

func TestPreprocessJSON_Ignore(t *testing.T) {
	in := tempJSONFile(t, `{"resourceType":"Patient","meta":{"tag":["foo"]},"id":"1"}`)
	flagExtract = ""
	flagIgnore = []string{"$.meta"}
	t.Cleanup(func() { flagExtract = ""; flagIgnore = nil })

	if err := preprocessJSON(in); err != nil {
		t.Fatalf("preprocessJSON() error: %v", err)
	}

	data, err := os.ReadFile(in.Path)
	if err != nil {
		t.Fatalf("reading result: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if _, ok := result["meta"]; ok {
		t.Error("expected 'meta' to be removed by --ignore")
	}
	if result["resourceType"] != "Patient" {
		t.Error("expected 'resourceType' preserved after --ignore")
	}
}

func TestPreprocessJSON_ExtractMissingPath_ReturnsError(t *testing.T) {
	in := tempJSONFile(t, `{"foo":"bar"}`)
	flagExtract = "$.nonexistent"
	flagIgnore = nil
	t.Cleanup(func() { flagExtract = ""; flagIgnore = nil })

	if err := preprocessJSON(in); err == nil {
		t.Error("expected error for missing extract path, got nil")
	}
}

func TestPreprocessJSON_IgnoreMultipleFields(t *testing.T) {
	in := tempJSONFile(t, `{"resourceType":"Patient","meta":{"tag":["a"]},"text":{"status":"generated"},"id":"1"}`)
	flagExtract = ""
	flagIgnore = []string{"$.meta", "$.text"}
	t.Cleanup(func() { flagExtract = ""; flagIgnore = nil })

	if err := preprocessJSON(in); err != nil {
		t.Fatalf("preprocessJSON() error: %v", err)
	}

	data, err := os.ReadFile(in.Path)
	if err != nil {
		t.Fatalf("reading result: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if _, ok := result["meta"]; ok {
		t.Error("expected 'meta' removed")
	}
	if _, ok := result["text"]; ok {
		t.Error("expected 'text' removed")
	}
	if result["id"] != "1" {
		t.Error("expected 'id' preserved")
	}
}

// --- extractEachElements ---

func TestExtractEachElements_BasicArray(t *testing.T) {
	in := tempJSONFile(t, `{"medications":[
		{"resourceType":"Medication","id":"med-1"},
		{"resourceType":"Medication","id":"med-2"}
	]}`)
	in.Label = "api.json"
	flagExtractEach = "$.medications"
	t.Cleanup(func() { flagExtractEach = "" })

	ins, err := extractEachElements(in, flagExtractEach)
	if err != nil {
		t.Fatalf("extractEachElements error: %v", err)
	}
	defer func() {
		for _, t2 := range ins {
			t2.Cleanup()
		}
	}()

	if len(ins) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(ins))
	}
	if ins[0].Label != "api.json[0] (Medication/med-1)" {
		t.Errorf("label[0] = %q", ins[0].Label)
	}
	if ins[1].Label != "api.json[1] (Medication/med-2)" {
		t.Errorf("label[1] = %q", ins[1].Label)
	}

	// Verify temp files contain valid JSON
	for i, elem := range ins {
		data, err := os.ReadFile(elem.Path)
		if err != nil {
			t.Fatalf("reading temp file %d: %v", i, err)
		}
		var obj map[string]interface{}
		if err := json.Unmarshal(data, &obj); err != nil {
			t.Fatalf("element %d is not valid JSON: %v", i, err)
		}
	}
}

func TestExtractEachElements_LabelWithoutID(t *testing.T) {
	in := tempJSONFile(t, `{"items":[{"resourceType":"Patient"},{"foo":"bar"}]}`)
	in.Label = "response.json"

	ins, err := extractEachElements(in, "$.items")
	if err != nil {
		t.Fatalf("extractEachElements error: %v", err)
	}
	defer func() {
		for _, t2 := range ins {
			t2.Cleanup()
		}
	}()

	if ins[0].Label != "response.json[0]" {
		t.Errorf("label[0] without id = %q, want response.json[0]", ins[0].Label)
	}
	if ins[1].Label != "response.json[1]" {
		t.Errorf("label[1] = %q, want response.json[1]", ins[1].Label)
	}
}

func TestExtractEachElements_PathNotFound(t *testing.T) {
	in := tempJSONFile(t, `{"foo":"bar"}`)
	_, err := extractEachElements(in, "$.nonexistent")
	if err == nil {
		t.Error("expected error for missing path")
	}
}

func TestExtractEachElements_NotAnArray(t *testing.T) {
	in := tempJSONFile(t, `{"data":{"resourceType":"Patient"}}`)
	_, err := extractEachElements(in, "$.data")
	if err == nil {
		t.Error("expected error when path points to an object, not array")
	}
}

func TestExtractEachElements_EmptyArray(t *testing.T) {
	in := tempJSONFile(t, `{"items":[]}`)
	_, err := extractEachElements(in, "$.items")
	if err == nil {
		t.Error("expected error for empty array")
	}
}

func TestPreprocessJSON_ExtractThenImplicitNoIgnore(t *testing.T) {
	in := tempJSONFile(t, `{"wrapper":{"resourceType":"Observation","status":"final"}}`)
	flagExtract = "$.wrapper"
	flagIgnore = nil
	t.Cleanup(func() { flagExtract = ""; flagIgnore = nil })

	if err := preprocessJSON(in); err != nil {
		t.Fatalf("preprocessJSON() error: %v", err)
	}

	data, err := os.ReadFile(in.Path)
	if err != nil {
		t.Fatalf("reading result: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if result["resourceType"] != "Observation" {
		t.Errorf("expected resourceType=Observation, got %v", result["resourceType"])
	}
}
