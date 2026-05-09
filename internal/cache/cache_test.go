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
