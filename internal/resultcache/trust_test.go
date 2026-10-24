package resultcache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const trustKey = "2222222222222222222222222222222222222222222222222222222222222222"

// A planted symbolic link under an entry's name is neither read through nor
// written through (#498).
func TestEntryThatIsALink(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte(`{"result":{"valid":true}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, entryPath(dir, trustKey)); err != nil {
		t.Skipf("symlinks not available: %v", err)
	}

	if _, err := Get(dir, trustKey); !os.IsNotExist(err) {
		t.Errorf("Get through a link: err = %v, want a miss", err)
	}

	if err := Put(dir, trustKey, Entry{FhirlintVersion: "test"}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if data, _ := os.ReadFile(victim); string(data) != `{"result":{"valid":true}}` { //nolint:gosec // test temp file
		t.Errorf("Put wrote through the link: victim now holds %q", data)
	}
	info, err := os.Lstat(entryPath(dir, trustKey))
	if err != nil || !info.Mode().IsRegular() {
		t.Errorf("the entry should now be a regular file: %v, %v", info, err)
	}
	if e, err := Get(dir, trustKey); err != nil || e.FhirlintVersion != "test" {
		t.Errorf("Get after Put = %+v, %v", e, err)
	}
}

func TestPut_LeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	if err := Put(dir, trustKey, Entry{}); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != trustKey+".json" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("cache dir holds %v, want only the entry", names)
	}
}

func TestClear_RemovesLeftoverTempFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, trustKey+".123.tmp"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if n, err := Clear(dir); err != nil || n != 1 {
		t.Errorf("Clear = %d, %v, want 1 removed", n, err)
	}
}

func TestCheckDir(t *testing.T) {
	dir := t.TempDir()
	if err := CheckDir(filepath.Join(dir, "not-yet")); err != nil {
		t.Errorf("a missing directory is fine, Put creates it: %v", err)
	}
	if err := CheckDir(dir); err != nil {
		t.Errorf("own directory refused: %v", err)
	}
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckDir(file); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("CheckDir(file) = %v, want not a directory", err)
	}
}
