package cache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDir_CreatesDirectory(t *testing.T) {
	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir() error: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Dir() returned non-existent path %q: %v", dir, err)
	}
	if !info.IsDir() {
		t.Errorf("Dir() path %q is not a directory", dir)
	}
}

func TestDir_IsUnderHomeDir(t *testing.T) {
	t.Setenv(DirEnvVar, "") // this test is about the home-based default
	home, _ := os.UserHomeDir()
	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir() error: %v", err)
	}
	if !strings.HasPrefix(dir, home) {
		t.Errorf("Dir() %q should be under home dir %q", dir, home)
	}
}

func TestDir_ContainsFhirlint(t *testing.T) {
	t.Setenv(DirEnvVar, "") // this test is about the home-based default
	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir() error: %v", err)
	}
	if !strings.Contains(dir, "fhirlint") {
		t.Errorf("Dir() %q should contain 'fhirlint'", dir)
	}
}

func TestJARPath_CorrectFilename(t *testing.T) {
	path, err := JARPath()
	if err != nil {
		t.Fatalf("JARPath() error: %v", err)
	}
	if filepath.Base(path) != "validator_cli.jar" {
		t.Errorf("JARPath() base = %q, want validator_cli.jar", filepath.Base(path))
	}
}

func TestJARPath_InsideCacheDir(t *testing.T) {
	dir, _ := Dir()
	path, err := JARPath()
	if err != nil {
		t.Fatalf("JARPath() error: %v", err)
	}
	if filepath.Dir(path) != dir {
		t.Errorf("JARPath() dir = %q, want %q", filepath.Dir(path), dir)
	}
}

func TestValidatorVersionPath_CorrectFilename(t *testing.T) {
	path, err := ValidatorVersionPath()
	if err != nil {
		t.Fatalf("ValidatorVersionPath() error: %v", err)
	}
	if filepath.Base(path) != "validator_version.txt" {
		t.Errorf("ValidatorVersionPath() base = %q, want validator_version.txt", filepath.Base(path))
	}
}

func TestValidatorVersionPath_InsideCacheDir(t *testing.T) {
	dir, _ := Dir()
	path, err := ValidatorVersionPath()
	if err != nil {
		t.Fatalf("ValidatorVersionPath() error: %v", err)
	}
	if filepath.Dir(path) != dir {
		t.Errorf("ValidatorVersionPath() dir = %q, want %q", filepath.Dir(path), dir)
	}
}

func TestDir_IdempotentCalls(t *testing.T) {
	dir1, err1 := Dir()
	dir2, err2 := Dir()
	if err1 != nil || err2 != nil {
		t.Fatalf("Dir() errors: %v, %v", err1, err2)
	}
	if dir1 != dir2 {
		t.Errorf("Dir() not idempotent: %q != %q", dir1, dir2)
	}
}

func TestDir_HonoursEnvOverride(t *testing.T) {
	want := filepath.Join(t.TempDir(), "custom-cache")
	t.Setenv(DirEnvVar, want)

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir() error: %v", err)
	}
	if dir != want {
		t.Errorf("expected %q, got %q", want, dir)
	}
	// The override must be created, not merely returned — callers write into it
	// straight away.
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("override directory was not created: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("%q is not a directory", dir)
	}
}

func TestDir_EmptyEnvFallsBackToHome(t *testing.T) {
	t.Setenv(DirEnvVar, "   ")
	home, _ := os.UserHomeDir()

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir() error: %v", err)
	}
	if !strings.HasPrefix(dir, home) {
		t.Errorf("a blank override must fall back to the home dir, got %q", dir)
	}
}

// Every cache path must sit inside the override, or an override that only moves
// some of the files is worse than none.
func TestCachePaths_AllInsideEnvOverride(t *testing.T) {
	root := t.TempDir()
	t.Setenv(DirEnvVar, root)

	paths := map[string]func() (string, error){
		"JARPath":              JARPath,
		"ValidatorVersionPath": ValidatorVersionPath,
		"UpdateCheckPath":      UpdateCheckPath,
		"ChecksumStatusPath":   ChecksumStatusPath,
		"ResultCacheDir":       ResultCacheDir,
	}
	for name, fn := range paths {
		got, err := fn()
		if err != nil {
			t.Errorf("%s() error: %v", name, err)
			continue
		}
		if !strings.HasPrefix(got, root) {
			t.Errorf("%s() = %q, expected it inside the override %q", name, got, root)
		}
	}
}
