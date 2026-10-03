package localig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}
	return path
}

func TestPackageDir_CreatesPackageJSON(t *testing.T) {
	src := writeTempFile(t, "CodeSystem-drugs.json", `{"resourceType":"CodeSystem","id":"drugs"}`)

	dir, cleanup, err := PackageDir([]string{src}, "4.0.1")
	if err != nil {
		t.Fatalf("PackageDir error: %v", err)
	}
	defer cleanup()

	pkgPath := filepath.Join(dir, "package.json")
	data, err := os.ReadFile(pkgPath) //nolint:gosec // reading from test-controlled temp dir
	if err != nil {
		t.Fatalf("reading package.json: %v", err)
	}

	var pkg map[string]interface{}
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatalf("invalid package.json: %v", err)
	}
	if pkg["name"] != "fhirlint.local" {
		t.Errorf("package name = %v, want fhirlint.local", pkg["name"])
	}
}

func TestPackageDir_CopiesFiles(t *testing.T) {
	cs := writeTempFile(t, "CodeSystem-drugs.json", `{"resourceType":"CodeSystem"}`)
	vs := writeTempFile(t, "ValueSet-drugs.json", `{"resourceType":"ValueSet"}`)

	dir, cleanup, err := PackageDir([]string{cs, vs}, "4.0.1")
	if err != nil {
		t.Fatalf("PackageDir error: %v", err)
	}
	defer cleanup()

	for _, name := range []string{"CodeSystem-drugs.0.json", "ValueSet-drugs.1.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected %s in package dir: %v", name, err)
		}
	}
}

// The validator skips a file whose name ends in template.json when it loads
// the temp IG, so a --valueset/--codesystem file named that way must not keep
// its name (#444).
func TestPackageDir_RenamesFilesTheValidatorWouldSkip(t *testing.T) {
	jsonSrc := writeTempFile(t, "StructureDefinition-patient-template.json", `{"resourceType":"StructureDefinition"}`)
	xmlSrc := writeTempFile(t, "ValueSet-x-template.xml", `<ValueSet xmlns="http://hl7.org/fhir"/>`)

	dir, cleanup, err := PackageDir([]string{jsonSrc, xmlSrc}, "4.0.1")
	if err != nil {
		t.Fatalf("PackageDir error: %v", err)
	}
	defer cleanup()

	if skipped := SkippedByValidator([]string{dir}); len(skipped) != 0 {
		t.Errorf("temp IG holds files the validator skips: %v", skipped)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 3 {
		t.Errorf("want package.json plus 2 resources, got %d entries", len(entries))
	}
}

// Two sources with the same base name from different directories used to
// overwrite each other in the temp IG.
func TestPackageDir_KeepsSameBaseNameFromDifferentDirs(t *testing.T) {
	a := writeTempFile(t, "ValueSet.json", `{"resourceType":"ValueSet","id":"a"}`)
	b := writeTempFile(t, "ValueSet.json", `{"resourceType":"ValueSet","id":"b"}`)

	dir, cleanup, err := PackageDir([]string{a, b}, "4.0.1")
	if err != nil {
		t.Fatalf("PackageDir error: %v", err)
	}
	defer cleanup()

	for name, want := range map[string]string{"ValueSet.0.json": `"id":"a"`, "ValueSet.1.json": `"id":"b"`} {
		data, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // reading from test-controlled temp dir
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		if !strings.Contains(string(data), want) {
			t.Errorf("%s = %s, want content with %s", name, data, want)
		}
	}
}

func TestRejectedName(t *testing.T) {
	cases := map[string]bool{
		"StructureDefinition-sdc-questionnaire-extr-template.json": true,
		"report-template.xml":  true,
		"report-template.JSON": true, // the JAR swaps in the detected format's extension first
		"template.json":        true,
		"ReportTemplate.json":  false, // the check is case-sensitive on the stem
		"template-report.json": false,
		"report-template.txt":  false,
		"report-template":      false,
		"Patient.json":         false,
	}
	for name, want := range cases {
		if got := rejectedName(name); got != want {
			t.Errorf("rejectedName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestSkippedByValidator(t *testing.T) {
	folder := t.TempDir()
	for _, name := range []string{"StructureDefinition-a-template.json", "StructureDefinition-b.json", ".hidden-template.json"} {
		if err := os.WriteFile(filepath.Join(folder, name), []byte(`{}`), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Only the top level is scanned: fhirlint never passes -recurse.
	if err := os.MkdirAll(filepath.Join(folder, "sub"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "sub", "x-template.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}

	// A folder with package.tgz is read as that package, not scanned.
	pkgFolder := t.TempDir()
	for _, name := range []string{"package.tgz", "y-template.json"} {
		if err := os.WriteFile(filepath.Join(pkgFolder, name), []byte(`{}`), 0600); err != nil {
			t.Fatal(err)
		}
	}

	single := writeTempFile(t, "z-template.json", `{}`)

	got := SkippedByValidator([]string{
		folder,
		pkgFolder,
		single,
		"de.basisprofil.r4#1.6.0",
		"/nonexistent/q-template.json",
	})
	want := []string{filepath.Join(folder, "StructureDefinition-a-template.json"), single}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("SkippedByValidator = %v, want %v", got, want)
	}
}

func TestPackageDir_CleanupRemovesDir(t *testing.T) {
	src := writeTempFile(t, "CodeSystem-test.json", `{}`)

	dir, cleanup, err := PackageDir([]string{src}, "4.0.1")
	if err != nil {
		t.Fatalf("PackageDir error: %v", err)
	}
	cleanup()

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("expected temp dir to be removed after cleanup")
	}
}

func TestPackageDir_FHIRVersionR4B(t *testing.T) {
	src := writeTempFile(t, "cs.json", `{"resourceType":"CodeSystem"}`)
	dir, cleanup, err := PackageDir([]string{src}, "4.3.0")
	if err != nil {
		t.Fatalf("PackageDir error: %v", err)
	}
	defer cleanup()

	data, _ := os.ReadFile(filepath.Join(dir, "package.json")) //nolint:gosec // reading from test-controlled temp dir
	var pkg map[string]interface{}
	_ = json.Unmarshal(data, &pkg)
	deps := pkg["dependencies"].(map[string]interface{})
	if _, ok := deps["hl7.fhir.r4b.core"]; !ok {
		t.Errorf("expected hl7.fhir.r4b.core dependency for FHIR 4.3.0, got %v", deps)
	}
}

func TestPackageDir_MissingSourceFile(t *testing.T) {
	_, _, err := PackageDir([]string{"/nonexistent/path/cs.json"}, "4.0.1")
	if err == nil {
		t.Error("expected error for missing source file")
	}
}

func TestCorePackageName(t *testing.T) {
	cases := []struct{ version, want string }{
		{"4.0.1", "hl7.fhir.r4.core"},
		{"4.3.0", "hl7.fhir.r4b.core"},
		{"5.0.0", "hl7.fhir.r5.core"},
		{"", "hl7.fhir.r4.core"},
	}
	for _, tc := range cases {
		got := corePackageName(tc.version)
		if got != tc.want {
			t.Errorf("corePackageName(%q) = %q, want %q", tc.version, got, tc.want)
		}
	}
}
