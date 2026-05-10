package validator

import (
	"encoding/json"
	"testing"
)

func TestBuildArgs_RequiredFlags(t *testing.T) {
	args := buildArgs("/fake/validator.jar", []string{"/tmp/patient.json"}, "/tmp/out.json", Options{
		FHIRVersion: "4.0.1",
	})

	mustContainPair(t, args, "-jar", "/fake/validator.jar")
	mustContainPair(t, args, "-version", "4.0.1")
	mustContainPair(t, args, "-output-style", "json")
	mustContainPair(t, args, "-output", "/tmp/out.json")
	mustContain(t, args, "/tmp/patient.json")
}

func TestBuildArgs_NoTerminologyServer(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion:         "4.0.1",
		NoTerminologyServer: true,
	})

	mustContainPair(t, args, "-tx", "n/a")
}

func TestBuildArgs_CustomTerminologyServer(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion:       "4.0.1",
		TerminologyServer: "https://my-tx.example.com",
	})

	mustContainPair(t, args, "-tx", "https://my-tx.example.com")
}

func TestBuildArgs_NoTerminologyServerTakesPrecedence(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion:         "4.0.1",
		NoTerminologyServer: true,
		TerminologyServer:   "https://my-tx.example.com",
	})

	mustContainPair(t, args, "-tx", "n/a")
	mustNotContain(t, args, "https://my-tx.example.com")
}

func TestBuildArgs_DefaultHasNoTxFlag(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{FHIRVersion: "4.0.1"})

	mustNotContain(t, args, "-tx")
}

func TestBuildArgs_Profiles(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion: "4.0.1",
		Profiles:    []string{"http://example.com/profile1", "http://example.com/profile2"},
	})

	mustContainPair(t, args, "-profile", "http://example.com/profile1")
	mustContainPair(t, args, "-profile", "http://example.com/profile2")
}

func TestBuildArgs_IGs(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion: "4.0.1",
		IGs:         []string{"kbv.basis#1.5.0", "de.medizininformatikinitiative.kerndatensatz#2024.0.0"},
	})

	mustContainPair(t, args, "-ig", "kbv.basis#1.5.0")
	mustContainPair(t, args, "-ig", "de.medizininformatikinitiative.kerndatensatz#2024.0.0")
}

func TestBuildArgs_FHIRVersions(t *testing.T) {
	for _, version := range []string{"4.0.1", "4.3.0", "5.0.0"} {
		args := buildArgs("jar", []string{"input"}, "out", Options{FHIRVersion: version})
		mustContainPair(t, args, "-version", version)
	}
}

func TestBuildArgs_EmptyProfilesAndIGs(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion: "4.0.1",
		Profiles:    []string{},
		IGs:         []string{},
	})

	mustNotContain(t, args, "-profile")
	mustNotContain(t, args, "-ig")
}

func TestBuildArgs_BestPracticeIgnore(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion:  "4.0.1",
		BestPractice: "ignore",
	})
	mustContainPair(t, args, "-best-practice", "ignore")
}

func TestBuildArgs_BestPracticeError(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion:  "4.0.1",
		BestPractice: "error",
	})
	mustContainPair(t, args, "-best-practice", "error")
}

func TestBuildArgs_BestPracticeEmptyOmitted(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{FHIRVersion: "4.0.1"})
	mustNotContain(t, args, "-best-practice")
}

func TestBuildArgs_TxCache(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion: "4.0.1",
		TxCache:     "/tmp/tx-cache",
	})
	mustContainPair(t, args, "-txCache", "/tmp/tx-cache")
}

func TestBuildArgs_TxCacheDisabled(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion: "4.0.1",
		TxCache:     "n/a",
	})
	mustContainPair(t, args, "-txCache", "n/a")
}

func TestBuildArgs_TxCacheEmptyOmitted(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{FHIRVersion: "4.0.1"})
	mustNotContain(t, args, "-txCache")
}

func TestBuildArgs_Locale(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion: "4.0.1",
		Locale:      "de",
	})
	mustContainPair(t, args, "-locale", "de")
}

func TestBuildArgs_LocaleEmptyOmitted(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{FHIRVersion: "4.0.1"})
	mustNotContain(t, args, "-locale")
}

func TestBuildArgs_AllowExampleURLs(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{
		FHIRVersion:      "4.0.1",
		AllowExampleURLs: true,
	})
	mustContain(t, args, "-allow-example-urls")
}

func TestBuildArgs_AllowExampleURLsFalseOmitted(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "out", Options{FHIRVersion: "4.0.1"})
	mustNotContain(t, args, "-allow-example-urls")
}

func TestBuildArgs_EmptyOutputPath_OmitsOutputFlags(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "", Options{FHIRVersion: "4.0.1"})
	mustNotContain(t, args, "-output-style")
	mustNotContain(t, args, "-output")
}

func TestBuildArgs_WithOutputPath_IncludesOutputFlags(t *testing.T) {
	args := buildArgs("jar", []string{"input"}, "/tmp/out.json", Options{FHIRVersion: "4.0.1"})
	mustContainPair(t, args, "-output-style", "json")
	mustContainPair(t, args, "-output", "/tmp/out.json")
}

func TestBuildArgs_MultipleInputPaths(t *testing.T) {
	paths := []string{"/tmp/a.json", "/tmp/b.json", "/tmp/c.json"}
	args := buildArgs("jar", paths, "out", Options{FHIRVersion: "4.0.1"})

	for _, p := range paths {
		mustContain(t, args, p)
	}
}

func TestParseOutput_OperationOutcome(t *testing.T) {
	oo := fixtureOO(t, "oo-error.json")
	data, err := encodeJSON(oo)
	if err != nil {
		t.Fatalf("encoding fixture: %v", err)
	}
	results, err := parseOutput(data, []string{"patient.json"}, "")
	if err != nil {
		t.Fatalf("parseOutput error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Filename != "patient.json" {
		t.Errorf("expected filename=patient.json, got %q", results[0].Filename)
	}
}

func TestParseOutput_Bundle(t *testing.T) {
	oo1 := fixtureOO(t, "oo-no-issues.json")
	oo2 := fixtureOO(t, "oo-error.json")
	bundle := map[string]interface{}{
		"resourceType": "Bundle",
		"type":         "collection",
		"entry": []map[string]interface{}{
			{"fullUrl": "file:///tmp/a.json", "resource": oo1},
			{"fullUrl": "file:///tmp/b.json", "resource": oo2},
		},
	}
	data, err := encodeJSON(bundle)
	if err != nil {
		t.Fatalf("encoding bundle: %v", err)
	}
	results, err := parseOutput(data, []string{"/tmp/a.json", "/tmp/b.json"}, "")
	if err != nil {
		t.Fatalf("parseOutput error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Filename != "/tmp/a.json" {
		t.Errorf("expected filename=/tmp/a.json, got %q", results[0].Filename)
	}
	if results[1].Filename != "/tmp/b.json" {
		t.Errorf("expected filename=/tmp/b.json, got %q", results[1].Filename)
	}
	if !results[0].Valid {
		t.Error("first result (no issues) should be valid")
	}
	if results[1].Valid {
		t.Error("second result (error) should be invalid")
	}
}

func TestParseOutput_UnknownResourceType(t *testing.T) {
	data := []byte(`{"resourceType":"Patient","id":"123"}`)
	_, err := parseOutput(data, nil, "")
	if err == nil {
		t.Error("expected error for unknown resourceType")
	}
}

func TestParseOutput_MalformedJSON(t *testing.T) {
	_, err := parseOutput([]byte(`not json`), nil, "")
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

// mustContainPair asserts that args contains flag immediately followed by value.
func mustContainPair(t *testing.T, args []string, flag, value string) {
	t.Helper()
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag && args[i+1] == value {
			return
		}
	}
	t.Errorf("expected args to contain %q %q, got: %v", flag, value, args)
}

// mustContain asserts that args contains the given value.
func mustContain(t *testing.T, args []string, value string) {
	t.Helper()
	for _, a := range args {
		if a == value {
			return
		}
	}
	t.Errorf("expected args to contain %q, got: %v", value, args)
}

// mustNotContain asserts that args does not contain the given value.
func mustNotContain(t *testing.T, args []string, value string) {
	t.Helper()
	for _, a := range args {
		if a == value {
			t.Errorf("expected args NOT to contain %q, got: %v", value, args)
			return
		}
	}
}

func encodeJSON(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}
