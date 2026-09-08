package igbaseline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

// fakeCache builds a package cache holding one package, and returns its root.
// summary is written only when non-nil, so a package without a baseline — the
// majority of a real cache — can be built too.
func fakeCache(t *testing.T, pkgID string, summary map[string]Counts, resources map[string]string) string {
	t.Helper()
	root := t.TempDir()
	pkgDir := filepath.Join(root, pkgID, "package")
	if err := os.MkdirAll(filepath.Join(pkgDir, "example"), 0750); err != nil {
		t.Fatal(err)
	}
	if summary != nil {
		if err := os.MkdirAll(filepath.Join(pkgDir, "other"), 0750); err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(summary)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pkgDir, "other", "validation-summary.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range resources {
		if err := os.WriteFile(filepath.Join(pkgDir, "example", name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func resource(rt, id string) string {
	return `{"resourceType":"` + rt + `","id":"` + id + `"}`
}

func errorResult(path string) *validator.Result {
	return &validator.Result{
		Filename: path,
		Issues:   []validator.Issue{{Severity: "error", Message: "boom"}},
	}
}

func cleanResult(path string) *validator.Result {
	return &validator.Result{Filename: path, Valid: true}
}

// The four states the comparison can reach, in one run. Expected is what makes
// a package's own negative examples readable; New is the number worth acting on.
func TestCompareIn_Classification(t *testing.T) {
	root := fakeCache(t, "de.fhir.medication#2.0.0-ballot",
		map[string]Counts{
			"MedicationRequest/inv-1":   {Errors: 2},
			"MedicationRequest/clean-1": {Errors: 0, Warnings: 3},
			"MedicationRequest/inv-2":   {Errors: 1},
			"MedicationRequest/clean-2": {Errors: 0},
		},
		map[string]string{
			"a.json": resource("MedicationRequest", "inv-1"),
			"b.json": resource("MedicationRequest", "clean-1"),
			"c.json": resource("MedicationRequest", "inv-2"),
			"d.json": resource("MedicationRequest", "clean-2"),
		})
	ex := filepath.Join(root, "de.fhir.medication#2.0.0-ballot", "package", "example")

	rep := CompareIn(root, []*validator.Result{
		errorResult(filepath.Join(ex, "a.json")), // published fails, we fail
		errorResult(filepath.Join(ex, "b.json")), // published clean, we fail
		cleanResult(filepath.Join(ex, "c.json")), // published fails, we do not
		cleanResult(filepath.Join(ex, "d.json")), // both clean
	})

	if rep.Covered != 4 {
		t.Errorf("Covered = %d, want 4", rep.Covered)
	}
	for _, tc := range []struct {
		file string
		want Status
	}{
		{"a.json", StatusExpected},
		{"b.json", StatusNew},
		{"c.json", StatusMissing},
		{"d.json", StatusAgreesClean},
	} {
		if got := rep.Status[filepath.Join(ex, tc.file)]; got != tc.want {
			t.Errorf("%s: status = %v, want %v", tc.file, got, tc.want)
		}
	}
	if rep.Expected != 1 || rep.New != 1 || rep.Missing != 1 {
		t.Errorf("counts = expected %d, new %d, missing %d; want 1/1/1", rep.Expected, rep.New, rep.Missing)
	}
	if len(rep.Packages) != 1 || rep.Packages[0] != "de.fhir.medication#2.0.0-ballot" {
		t.Errorf("Packages = %v, want the one package", rep.Packages)
	}
}

// 85 of the 198 packages in a representative cache ship no baseline, including
// every version of de.basisprofil.r4. That has to report as "nothing to say",
// not as a finding and not as an error.
func TestCompareIn_PackageWithoutBaseline(t *testing.T) {
	root := fakeCache(t, "de.basisprofil.r4#1.5.4", nil,
		map[string]string{"a.json": resource("Patient", "x")})
	path := filepath.Join(root, "de.basisprofil.r4#1.5.4", "package", "example", "a.json")

	rep := CompareIn(root, []*validator.Result{errorResult(path)})

	if rep.Covered != 0 || len(rep.Status) != 0 || len(rep.Packages) != 0 {
		t.Errorf("got covered=%d status=%v packages=%v, want an empty report",
			rep.Covered, rep.Status, rep.Packages)
	}
}

// The ordinary run: a user's own files, nowhere near the package cache.
func TestCompareIn_FilesOutsideTheCache(t *testing.T) {
	root := fakeCache(t, "some.pkg#1.0.0", map[string]Counts{"Patient/x": {Errors: 1}}, nil)

	own := filepath.Join(t.TempDir(), "patient.json")
	if err := os.WriteFile(own, []byte(resource("Patient", "x")), 0600); err != nil {
		t.Fatal(err)
	}

	rep := CompareIn(root, []*validator.Result{errorResult(own)})

	if rep.Covered != 0 {
		t.Errorf("Covered = %d, want 0 — a file outside the cache has no baseline", rep.Covered)
	}
}

// A resource the baseline does not mention was not in the published build, so
// nothing follows from it. Distinct from one recorded with zero errors, which
// is a positive statement.
func TestCompareIn_ResourceNotInBaseline(t *testing.T) {
	root := fakeCache(t, "some.pkg#1.0.0",
		map[string]Counts{"Patient/known": {Errors: 0}},
		map[string]string{
			"known.json":   resource("Patient", "known"),
			"unknown.json": resource("Patient", "unknown"),
		})
	ex := filepath.Join(root, "some.pkg#1.0.0", "package", "example")

	rep := CompareIn(root, []*validator.Result{
		errorResult(filepath.Join(ex, "unknown.json")),
		cleanResult(filepath.Join(ex, "known.json")),
	})

	if rep.Covered != 1 {
		t.Errorf("Covered = %d, want 1 — only the recorded resource counts", rep.Covered)
	}
	if _, ok := rep.Status[filepath.Join(ex, "unknown.json")]; ok {
		t.Error("a resource absent from the baseline was given a status")
	}
}

// The key comes from the content because the filename cannot be split: ids
// contain hyphens, and "MedicationDispense-Example-MD-Cov-bedarf-doseRange" has
// no unambiguous boundary.
func TestResourceKey(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		return p
	}

	p := write("MedicationDispense-Example-MD-Cov-bedarf-doseRange.json",
		resource("MedicationDispense", "Example-MD-Cov-bedarf-doseRange"))
	key, ok := ResourceKey(p)
	if !ok || key != "MedicationDispense/Example-MD-Cov-bedarf-doseRange" {
		t.Errorf("ResourceKey = %q, %v", key, ok)
	}

	for name, body := range map[string]string{
		"noid.json":    `{"resourceType":"Patient"}`,
		"notype.json":  `{"id":"x"}`,
		"garbage.json": `not json`,
	} {
		if _, ok := ResourceKey(write(name, body)); ok {
			t.Errorf("%s: got a key, want none", name)
		}
	}

	if _, ok := ResourceKey(filepath.Join(dir, "absent.json")); ok {
		t.Error("missing file: got a key, want none")
	}
}

func TestPackageDirFor(t *testing.T) {
	root := t.TempDir()
	pkg := filepath.Join(root, "some.pkg#1.0.0")

	tests := map[string]string{
		filepath.Join(pkg, "package", "example", "a.json"): pkg,
		filepath.Join(pkg, "package", "a.json"):            pkg,
		// Under the cache root but not laid out like a package.
		filepath.Join(root, "some.pkg#1.0.0", "a.json"): "",
		filepath.Join(root, "loose.json"):               "",
		// A directory whose name carries no version.
		filepath.Join(root, "nohash", "package", "a.json"): "",
		// Outside the cache entirely.
		filepath.Join(t.TempDir(), "a.json"): "",
	}
	for path, want := range tests {
		if got := PackageDirFor(root, path); got != want {
			t.Errorf("PackageDirFor(%q) = %q, want %q", path, got, want)
		}
	}

	if got := PackageDirFor("", filepath.Join(pkg, "package", "a.json")); got != "" {
		t.Errorf("empty root: got %q, want empty", got)
	}
}

func TestLoad(t *testing.T) {
	root := fakeCache(t, "some.pkg#1.0.0",
		map[string]Counts{
			"Patient/a": {Errors: 2, Warnings: 1},
			"Patient/b": {Errors: 0, Warnings: 4},
		}, nil)
	pkgDir := filepath.Join(root, "some.pkg#1.0.0")

	b, err := Load(pkgDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if b == nil {
		t.Fatal("Load returned no baseline for a package that has one")
	}
	if b.PackageID != "some.pkg#1.0.0" {
		t.Errorf("PackageID = %q", b.PackageID)
	}
	if b.Len() != 2 {
		t.Errorf("Len = %d, want 2", b.Len())
	}
	if c, ok := b.Lookup("Patient/a"); !ok || c.Errors != 2 || c.Warnings != 1 {
		t.Errorf("Lookup(Patient/a) = %+v, %v", c, ok)
	}
	// Recorded with zero errors is a positive statement and must be
	// distinguishable from not being recorded at all.
	if c, ok := b.Lookup("Patient/b"); !ok || c.Errors != 0 {
		t.Errorf("Lookup(Patient/b) = %+v, %v; want a known, error-free entry", c, ok)
	}
	if _, ok := b.Lookup("Patient/absent"); ok {
		t.Error("Lookup of an unrecorded resource reported it as known")
	}
}

// A package without the file is the common case, so it is not an error — and a
// nil baseline still has to answer Len and Lookup without a special case at the
// call site.
func TestLoad_NoBaselineIsNotAnError(t *testing.T) {
	root := fakeCache(t, "de.basisprofil.r4#1.5.4", nil, nil)

	b, err := Load(filepath.Join(root, "de.basisprofil.r4#1.5.4"))
	if err != nil || b != nil {
		t.Fatalf("Load = %v, %v; want nil, nil", b, err)
	}
	if b.Len() != 0 {
		t.Errorf("nil baseline Len = %d, want 0", b.Len())
	}
	if _, ok := b.Lookup("Patient/x"); ok {
		t.Error("nil baseline reported a resource as known")
	}
}

func TestLoad_MalformedBaselineIsAnError(t *testing.T) {
	root := t.TempDir()
	other := filepath.Join(root, "bad.pkg#1.0.0", "package", "other")
	if err := os.MkdirAll(other, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "validation-summary.json"), []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(filepath.Join(root, "bad.pkg#1.0.0")); err == nil {
		t.Error("malformed baseline: got nil, want an error")
	}
}

// Compare must not fail a user's run over someone else's malformed artefact.
func TestCompareIn_MalformedBaselineIsIgnored(t *testing.T) {
	root := t.TempDir()
	pkgDir := filepath.Join(root, "bad.pkg#1.0.0", "package")
	if err := os.MkdirAll(filepath.Join(pkgDir, "other"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(pkgDir, "example"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "other", "validation-summary.json"), []byte("{{{"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(pkgDir, "example", "a.json")
	if err := os.WriteFile(path, []byte(resource("Patient", "x")), 0600); err != nil {
		t.Fatal(err)
	}

	rep := CompareIn(root, []*validator.Result{errorResult(path)})
	if rep.Covered != 0 {
		t.Errorf("Covered = %d, want 0", rep.Covered)
	}
}

// SourcePath is what the validator actually read; Filename can be a remapped
// display name after --extract or --bundle-entries. The baseline lookup has to
// follow the file on disk, while the status map stays keyed by what the rest of
// the pipeline calls the result.
func TestCompareIn_UsesSourcePathButKeysByFilename(t *testing.T) {
	root := fakeCache(t, "some.pkg#1.0.0",
		map[string]Counts{"Patient/x": {Errors: 1}},
		map[string]string{"a.json": resource("Patient", "x")})
	src := filepath.Join(root, "some.pkg#1.0.0", "package", "example", "a.json")

	r := errorResult("displayed-as-this.json")
	r.SourcePath = src

	rep := CompareIn(root, []*validator.Result{r})

	if got := rep.Status["displayed-as-this.json"]; got != StatusExpected {
		t.Errorf("status = %v, want StatusExpected keyed by Filename", got)
	}
}
