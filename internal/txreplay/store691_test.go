// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package txreplay

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The corners of a recording directory (#691).

// A recording that can't be read fails loudly: replaying a partial one would
// turn the missing answers into terminology errors that aren't real.
func TestOpen_UnreadableRecordings(t *testing.T) {
	file := filepath.Join(t.TempDir(), "rec")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	// Not on Windows: there reading a file as a directory reports it as not
	// existing, so Open gives an empty store, and validate then stops with
	// "no terminology recording … record one first" anyway.
	if _, err := Open(file); runtime.GOOS != "windows" && (err == nil || !strings.Contains(err.Error(), "reading terminology recording")) {
		t.Errorf("a recording path that is a file: %v", err)
	}
	if runtime.GOOS != "windows" {
		dir := t.TempDir()
		if err := os.Symlink(filepath.Join(dir, "gone"), filepath.Join(dir, "abc.json")); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "abc.json") {
			t.Errorf("an entry that can't be read must be named: %v", err)
		}
	}
}

// Directories, the manifest and non-JSON files aren't interactions.
func TestOpen_SkipsWhatIsNoInteraction(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub.json"), 0o750); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"manifest.json": `{"entries":3}`, "README.md": "# notes"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("a directory with no interactions must open: %v", err)
	}
	if s.Len() != 0 {
		t.Errorf("got %d entries, want none", s.Len())
	}
}

// A recording whose file name isn't today's key (renamed, or from an older
// fhirlint) still replays by the request it stores.
func TestOpen_RekeysByTheStoredRequest(t *testing.T) {
	dir := t.TempDir()
	body := `{"method":"GET","path":"/metadata","query":"mode=terminology","status":200,"response":{"resourceType":"TerminologyCapabilities"}}`
	if err := os.WriteFile(filepath.Join(dir, "0123456789abcdef.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if in, ok := s.Lookup("GET", "/metadata", "mode=terminology", nil); !ok || in.Status != 200 {
		t.Errorf("a renamed recording must still be found: %+v, %v", in, ok)
	}
	if _, ok := s.Lookup("GET", "/metadata", "mode=other", nil); ok {
		t.Error("another request must not match it")
	}
}

// Writing into a recording below a file fails with what it tried.
func TestStore_WritesBelowAFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Store{dir: filepath.Join(file, "rec"), entries: map[string]*Interaction{}, rekeyed: map[string]string{}}
	if err := s.Put(&Interaction{Method: "GET", Path: "/metadata", Status: 200}, nil); err == nil || !strings.Contains(err.Error(), "creating terminology recording directory") {
		t.Errorf("Put: %v", err)
	}
	if err := s.WriteManifest(Manifest{}); err == nil || !strings.Contains(err.Error(), "creating terminology recording directory") {
		t.Errorf("WriteManifest: %v", err)
	}
}

// A manifest that isn't JSON reads as no provenance, not as an error.
func TestReadManifest_Corrupt(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, manifestName), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m := s.ReadManifest(); m != nil {
		t.Errorf("got %+v", m)
	}
}
