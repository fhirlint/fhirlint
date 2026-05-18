// Package iglock implements the fhirlint.lock file for reproducible IG validation.
// The lock file records the SHA256 of each IG package's manifest (package.json) as
// installed in the FHIR package cache (~/.fhir/packages/).
package iglock

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const LockFileName = "fhirlint.lock"

// Entry is a single package entry in the lock file.
type Entry struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

// LockFile is the in-memory representation of fhirlint.lock.
type LockFile struct {
	Packages map[string]Entry `json:"packages"`
}

// ParseIGID splits a "name#version" IG string into its components.
// Returns empty strings if the format is not recognized or the version
// looks like a local path or directory.
func ParseIGID(ig string) (name, version string) {
	idx := strings.LastIndex(ig, "#")
	if idx < 0 {
		return "", ""
	}
	n, v := ig[:idx], ig[idx+1:]
	if n == "" || v == "" || v == "latest" || strings.ContainsAny(n, "/\\") {
		return "", ""
	}
	return n, v
}

// PackageURL returns the canonical packages.fhir.org URL for the given package.
func PackageURL(name, version string) string {
	return "https://packages.fhir.org/" + name + "/" + version
}

// fhirPackageCacheDir returns the default FHIR package cache directory.
func fhirPackageCacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".fhir", "packages"), nil
}

// PackageManifestPath returns the path to package.json for a cached package.
func PackageManifestPath(name, version string) (string, error) {
	dir, err := fhirPackageCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name+"#"+version, "package", "package.json"), nil
}

// HashPackage computes SHA256 of the package.json for the given cached IG.
// Returns os.ErrNotExist if the package is not in the local FHIR package cache.
func HashPackage(name, version string) (string, error) {
	path, err := PackageManifestPath(name, version)
	if err != nil {
		return "", err
	}
	return HashPackageFromPath(path)
}

// HashPackageFromPath computes SHA256 of the file at path.
func HashPackageFromPath(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // path constructed from trusted source
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// Read parses fhirlint.lock from the given path.
// Returns nil, nil if the file does not exist.
func Read(path string) (*LockFile, error) {
	data, err := os.ReadFile(path) //nolint:gosec // caller-supplied path
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var lf LockFile
	if err := json.Unmarshal(data, &lf); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if lf.Packages == nil {
		lf.Packages = make(map[string]Entry)
	}
	return &lf, nil
}

// Write serialises lf as pretty-printed JSON and writes it to path.
func Write(path string, lf *LockFile) error {
	data, err := json.MarshalIndent(lf, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0600) //nolint:gosec // lock file in project dir
}

// Verify checks that every lockable IG in igs matches the hash recorded in lf.
// Missing packages (not yet in FHIR cache) are skipped with a warning written to w.
// Returns an error for any hash mismatch.
func Verify(lf *LockFile, igs []string, w io.Writer) error {
	for _, ig := range igs {
		name, version := ParseIGID(ig)
		if name == "" {
			continue
		}
		entry, ok := lf.Packages[ig]
		if !ok {
			_, _ = fmt.Fprintf(w, "warn: %s not in lock file — run with --lock to update\n", ig)
			continue
		}
		got, err := HashPackage(name, version)
		if os.IsNotExist(err) {
			_, _ = fmt.Fprintf(w, "warn: %s not in FHIR package cache — cannot verify\n", ig)
			continue
		}
		if err != nil {
			return fmt.Errorf("hashing %s: %w", ig, err)
		}
		if got != entry.SHA256 {
			return fmt.Errorf(
				"lock file mismatch for %s: expected sha256 %s, got %s — run with --lock to update",
				ig, entry.SHA256, got,
			)
		}
	}
	return nil
}

// Update computes hashes for all lockable IGs and upserts them into lf.
// IGs not in "name#version" format are skipped.
// Returns the number of entries added or updated.
func Update(lf *LockFile, igs []string) (int, error) {
	count := 0
	for _, ig := range igs {
		name, version := ParseIGID(ig)
		if name == "" {
			continue
		}
		hash, err := HashPackage(name, version)
		if os.IsNotExist(err) {
			return count, fmt.Errorf("%s not found in FHIR package cache — ensure the JAR has downloaded it first", ig)
		}
		if err != nil {
			return count, fmt.Errorf("hashing %s: %w", ig, err)
		}
		lf.Packages[ig] = Entry{
			URL:    PackageURL(name, version),
			SHA256: hash,
		}
		count++
	}
	return count, nil
}
