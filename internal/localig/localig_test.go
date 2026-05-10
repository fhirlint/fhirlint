package localig

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	data, err := os.ReadFile(pkgPath)
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

	for _, name := range []string{"CodeSystem-drugs.json", "ValueSet-drugs.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected %s in package dir: %v", name, err)
		}
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

	data, _ := os.ReadFile(filepath.Join(dir, "package.json"))
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
