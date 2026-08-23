package fhirpkg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// fakeCache builds a package cache in a temp dir and points CacheRoot at it.
func fakeCache(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	orig := homeDir
	homeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { homeDir = orig })

	root := filepath.Join(home, ".fhir", "packages")
	if err := os.MkdirAll(root, 0750); err != nil {
		t.Fatal(err)
	}
	return root
}

// install writes one package into the fake cache.
func install(t *testing.T, root, id string, m Manifest, filler int) {
	t.Helper()
	dir := filepath.Join(root, id, "package")
	if err := os.MkdirAll(dir, 0750); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if filler > 0 {
		if err := os.WriteFile(filepath.Join(dir, "filler.bin"), make([]byte, filler), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestList_Empty(t *testing.T) {
	fakeCache(t)
	pkgs, err := List(false)
	if err != nil {
		t.Fatalf("an empty cache is a state, not an error: %v", err)
	}
	if len(pkgs) != 0 {
		t.Errorf("got %d packages, want 0", len(pkgs))
	}
}

func TestList_NoCacheDirectory(t *testing.T) {
	home := t.TempDir()
	orig := homeDir
	homeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { homeDir = orig })

	_, err := List(false)
	if err == nil {
		t.Fatal("want an error naming the missing cache")
	}
	if !isNoCache(err) {
		t.Errorf("want ErrNoCache, got %v", err)
	}
}

func isNoCache(err error) bool {
	for err != nil {
		if err == ErrNoCache { //nolint:errorlint // sentinel identity is the point
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

func TestList_SortsAndReadsManifest(t *testing.T) {
	root := fakeCache(t)
	install(t, root, "b.pkg#1.0.0", Manifest{Title: "B", FHIRVersions: []string{"4.0.1"}}, 0)
	install(t, root, "a.pkg#2.0.0", Manifest{Title: "A2"}, 0)
	install(t, root, "a.pkg#1.0.0", Manifest{Title: "A1"}, 0)

	pkgs, err := List(false)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range pkgs {
		got = append(got, p.ID)
	}
	want := []string{"a.pkg#1.0.0", "a.pkg#2.0.0", "b.pkg#1.0.0"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if pkgs[2].Title != "B" || len(pkgs[2].FHIRVersions) != 1 {
		t.Errorf("manifest not read back: %+v", pkgs[2])
	}
}

// The directory name is authoritative: a few published packages carry a
// manifest whose version disagrees with where they install.
func TestList_DirectoryNameWinsOverManifest(t *testing.T) {
	root := fakeCache(t)
	install(t, root, "a.pkg#1.0.0", Manifest{Name: "wrong.name", Version: "9.9.9"}, 0)

	pkgs, err := List(false)
	if err != nil {
		t.Fatal(err)
	}
	if pkgs[0].Name != "a.pkg" || pkgs[0].Version != "1.0.0" {
		t.Errorf("got %s#%s, want a.pkg#1.0.0", pkgs[0].Name, pkgs[0].Version)
	}
}

// One unreadable entry must not hide the rest — the cache is written by other tools.
func TestList_SkipsUnreadableEntries(t *testing.T) {
	root := fakeCache(t)
	install(t, root, "good.pkg#1.0.0", Manifest{}, 0)
	// A directory with no manifest at all.
	if err := os.MkdirAll(filepath.Join(root, "broken.pkg#1.0.0", "package"), 0750); err != nil {
		t.Fatal(err)
	}
	// A manifest that is not JSON.
	bad := filepath.Join(root, "bad.pkg#1.0.0", "package")
	if err := os.MkdirAll(bad, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "package.json"), []byte("{{{"), 0600); err != nil {
		t.Fatal(err)
	}
	// A stray file and a directory without the # convention.
	if err := os.WriteFile(filepath.Join(root, "loose.txt"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "no-hash-dir"), 0750); err != nil {
		t.Fatal(err)
	}

	pkgs, err := List(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 1 || pkgs[0].ID != "good.pkg#1.0.0" {
		t.Errorf("got %d packages %v, want just good.pkg#1.0.0", len(pkgs), pkgs)
	}
}

func TestList_Sizes(t *testing.T) {
	root := fakeCache(t)
	install(t, root, "a.pkg#1.0.0", Manifest{}, 4096)

	withSizes, err := List(true)
	if err != nil {
		t.Fatal(err)
	}
	if withSizes[0].Bytes < 4096 {
		t.Errorf("size = %d, want at least the 4096-byte filler", withSizes[0].Bytes)
	}

	without, err := List(false)
	if err != nil {
		t.Fatal(err)
	}
	if without[0].Bytes != 0 {
		t.Errorf("size = %d, want 0 when sizes were not requested", without[0].Bytes)
	}
}

func TestGrouped_FoldsVersions(t *testing.T) {
	root := fakeCache(t)
	install(t, root, "a.pkg#1.0.0", Manifest{}, 100)
	install(t, root, "a.pkg#2.0.0", Manifest{}, 100)
	install(t, root, "b.pkg#1.0.0", Manifest{}, 100)

	pkgs, err := List(true)
	if err != nil {
		t.Fatal(err)
	}
	groups := Grouped(pkgs)
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}
	if len(groups[0].Versions) != 2 {
		t.Errorf("a.pkg should hold 2 versions, got %d", len(groups[0].Versions))
	}
	if groups[0].MixedCase {
		t.Error("one spelling must not be reported as mixed case")
	}
	if groups[0].Bytes <= 100 {
		t.Errorf("group size %d should sum its versions", groups[0].Bytes)
	}
}

// KBV.Basis and kbv.basis are the same package upstream and both end up on disk.
func TestGrouped_MixedCaseIsOneGroupWithTheNewerSpelling(t *testing.T) {
	root := fakeCache(t)
	install(t, root, "KBV.Basis#1.1.0", Manifest{}, 0)
	install(t, root, "kbv.basis#1.9.0", Manifest{}, 0)

	pkgs, err := List(false)
	if err != nil {
		t.Fatal(err)
	}
	groups := Grouped(pkgs)
	if len(groups) != 1 {
		t.Fatalf("got %d groups, want them folded into 1", len(groups))
	}
	if !groups[0].MixedCase {
		t.Error("two spellings on disk must be reported")
	}
	if groups[0].Name != "kbv.basis" {
		t.Errorf("group name = %q, want the newer spelling kbv.basis", groups[0].Name)
	}
	// The directory names themselves must be preserved, not normalised.
	if groups[0].Versions[0].Name != "KBV.Basis" {
		t.Errorf("the on-disk spelling was rewritten: %q", groups[0].Versions[0].Name)
	}
}

func TestLoad(t *testing.T) {
	root := fakeCache(t)
	install(t, root, "a.pkg#1.0.0", Manifest{Dependencies: map[string]string{"b.pkg": "2.0.0"}}, 0)

	p, ok := Load("a.pkg", "1.0.0")
	if !ok {
		t.Fatal("want the installed package")
	}
	if p.Dependencies["b.pkg"] != "2.0.0" {
		t.Errorf("dependencies not read: %+v", p.Dependencies)
	}
	if _, ok := Load("a.pkg", "9.9.9"); ok {
		t.Error("an uninstalled version must not load")
	}
}

func TestSplitID(t *testing.T) {
	for _, tc := range []struct{ in, name, version string }{
		{"kbv.basis#1.9.0", "kbv.basis", "1.9.0"},
		{"kbv.basis", "kbv.basis", ""},
		{"a#b#c", "a#b", "c"},
		{"", "", ""},
	} {
		n, v := SplitID(tc.in)
		if n != tc.name || v != tc.version {
			t.Errorf("SplitID(%q) = %q,%q want %q,%q", tc.in, n, v, tc.name, tc.version)
		}
	}
}

func TestManifestPath(t *testing.T) {
	root := fakeCache(t)
	got, err := ManifestPath("a.pkg", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "a.pkg#1.0.0", "package", "package.json")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
