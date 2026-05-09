package cmd

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/fhirlint/fhirlint/internal/input"
)

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
	f.WriteString(content)
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })
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

	data, _ := os.ReadFile(in.Path)
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

	data, _ := os.ReadFile(in.Path)
	var result map[string]interface{}
	json.Unmarshal(data, &result)
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

	data, _ := os.ReadFile(in.Path)
	var result map[string]interface{}
	json.Unmarshal(data, &result)
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

func TestPreprocessJSON_ExtractThenImplicitNoIgnore(t *testing.T) {
	in := tempJSONFile(t, `{"wrapper":{"resourceType":"Observation","status":"final"}}`)
	flagExtract = "$.wrapper"
	flagIgnore = nil
	t.Cleanup(func() { flagExtract = ""; flagIgnore = nil })

	if err := preprocessJSON(in); err != nil {
		t.Fatalf("preprocessJSON() error: %v", err)
	}

	data, _ := os.ReadFile(in.Path)
	var result map[string]interface{}
	json.Unmarshal(data, &result)
	if result["resourceType"] != "Observation" {
		t.Errorf("expected resourceType=Observation, got %v", result["resourceType"])
	}
}
