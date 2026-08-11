package resultcache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fhirlint/fhirlint/internal/validator"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func defaultOpts() KeyOpts {
	return KeyOpts{
		FhirlintVersion: "0.2.0",
		FHIRVersion:     "4.0.1",
		Profiles:        []string{"kbv-basis"},
		IGs:             []string{"kbv.basis#1.5.0"},
	}
}

func TestKey_SameContentSameOpts(t *testing.T) {
	dir := t.TempDir()
	p1 := writeFile(t, dir, "a.json", `{"resourceType":"Patient"}`)
	p2 := writeFile(t, dir, "b.json", `{"resourceType":"Patient"}`)

	k1, err := Key(p1, defaultOpts())
	if err != nil {
		t.Fatalf("Key error: %v", err)
	}
	k2, err := Key(p2, defaultOpts())
	if err != nil {
		t.Fatalf("Key error: %v", err)
	}
	if k1 != k2 {
		t.Error("identical content + options should produce same key")
	}
}

func TestKey_DifferentContent(t *testing.T) {
	dir := t.TempDir()
	p1 := writeFile(t, dir, "a.json", `{"resourceType":"Patient"}`)
	p2 := writeFile(t, dir, "b.json", `{"resourceType":"Observation"}`)

	k1, _ := Key(p1, defaultOpts())
	k2, _ := Key(p2, defaultOpts())
	if k1 == k2 {
		t.Error("different content should produce different keys")
	}
}

func TestKey_DifferentFHIRVersion(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "a.json", `{"resourceType":"Patient"}`)

	opts1 := defaultOpts()
	opts2 := defaultOpts()
	opts2.FHIRVersion = "5.0.0"

	k1, _ := Key(p, opts1)
	k2, _ := Key(p, opts2)
	if k1 == k2 {
		t.Error("different FHIR version should produce different keys")
	}
}

func TestKey_ProfileOrderIndependent(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "a.json", `{"resourceType":"Patient"}`)

	opts1 := KeyOpts{FhirlintVersion: "0.2.0", FHIRVersion: "4.0.1", Profiles: []string{"a", "b"}}
	opts2 := KeyOpts{FhirlintVersion: "0.2.0", FHIRVersion: "4.0.1", Profiles: []string{"b", "a"}}

	k1, _ := Key(p, opts1)
	k2, _ := Key(p, opts2)
	if k1 != k2 {
		t.Error("profile order should not affect key")
	}
}

func TestKey_MissingFile(t *testing.T) {
	_, err := Key("/nonexistent/file.json", defaultOpts())
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestGetPut_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	entry := Entry{
		CachedAt:        time.Now().UTC().Truncate(time.Second),
		FhirlintVersion: "0.2.0",
		Result: validator.Result{
			Label: "patient.json",
			Valid: true,
			Issues: []validator.Issue{
				{Severity: "warning", Message: "test", MessageID: "dom-6"},
			},
		},
	}

	if err := Put(dir, "abc123", entry); err != nil {
		t.Fatalf("Put error: %v", err)
	}

	got, err := Get(dir, "abc123")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if got.Result.Label != "patient.json" {
		t.Errorf("label = %q, want patient.json", got.Result.Label)
	}
	if !got.Result.Valid {
		t.Error("result should be valid")
	}
	if len(got.Result.Issues) != 1 {
		t.Errorf("issues = %d, want 1", len(got.Result.Issues))
	}
}

func TestGet_MissingEntry(t *testing.T) {
	dir := t.TempDir()
	_, err := Get(dir, "doesnotexist")
	if err == nil {
		t.Error("expected error for missing cache entry")
	}
}

func TestPut_CreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "cache")
	entry := Entry{FhirlintVersion: "0.2.0", Result: validator.Result{}}
	if err := Put(dir, "key1", entry); err != nil {
		t.Fatalf("Put should create missing dir: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Error("cache dir should have been created")
	}
}

func TestClear_RemovesEntries(t *testing.T) {
	dir := t.TempDir()
	entry := Entry{FhirlintVersion: "0.2.0", Result: validator.Result{}}
	_ = Put(dir, "key1", entry)
	_ = Put(dir, "key2", entry)

	n, err := Clear(dir)
	if err != nil {
		t.Fatalf("Clear error: %v", err)
	}
	if n != 2 {
		t.Errorf("Clear removed %d entries, want 2", n)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".json" {
			t.Errorf("unexpected file after Clear: %s", e.Name())
		}
	}
}

func TestClear_NonExistentDir(t *testing.T) {
	n, err := Clear("/nonexistent/dir")
	if err != nil {
		t.Errorf("Clear on non-existent dir should not error: %v", err)
	}
	if n != 0 {
		t.Errorf("Clear on non-existent dir should return 0")
	}
}

// A cache entry that cannot be removed must be reported. Silently counting only
// the successes produced "Removed 0 cached result(s)" over a directory that was
// still full, which reads as success (#316).
func TestClear_UnremovableEntryIsReported(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: directory modes do not deny access")
	}
	dir := t.TempDir()
	entry := Entry{FhirlintVersion: "0.2.0", Result: validator.Result{}}
	if err := Put(dir, "key1", entry); err != nil {
		t.Fatal(err)
	}
	// Removal needs write permission on the directory, not on the file.
	if err := os.Chmod(dir, 0500); err != nil { //nolint:gosec // a directory needs the execute bit; this is deliberately read-only, not writable
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) }) //nolint:gosec // restoring a temp directory, which needs the execute bit

	n, err := Clear(dir)
	if err == nil {
		t.Fatal("expected an error when an entry cannot be removed")
	}
	if n != 0 {
		t.Errorf("removed count = %d, want 0 — nothing was actually removed", n)
	}
	if !strings.Contains(err.Error(), "1 cache entry could not be removed") {
		t.Errorf("error should say how many failed, got: %v", err)
	}
}

// A directory whose name ends in .json is not a cache entry and must be left
// alone, without stopping the real entries from being cleared.
func TestClear_SkipsDirectoriesNamedLikeEntries(t *testing.T) {
	dir := t.TempDir()
	entry := Entry{FhirlintVersion: "0.2.0", Result: validator.Result{}}
	for _, k := range []string{"key1", "key2"} {
		if err := Put(dir, k, entry); err != nil {
			t.Fatal(err)
		}
	}
	sub := filepath.Join(dir, "key3.json")
	if err := os.Mkdir(sub, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "blocker"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	n, err := Clear(dir)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 2 {
		t.Errorf("removed %d, want both real entries", n)
	}
	if _, statErr := os.Stat(sub); statErr != nil {
		t.Error("a directory named like an entry must survive")
	}
}

func TestClear_CountsOnlyWhatItRemoved(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	n, err := Clear(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("removed = %d, want 0 — non-entry files are left alone", n)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "notes.txt")); statErr != nil {
		t.Error("Clear must not remove files that are not cache entries")
	}
}
