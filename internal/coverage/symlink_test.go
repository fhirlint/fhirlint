package coverage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A skipped file's reason quotes its parse error, so a link out of the
// directory must be skipped before it is parsed (#500).
func TestLoadResources_SkipsLinksThatLeaveTheDirectory(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "repo")
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(base, "secret.json")
	if err := os.WriteFile(secret, []byte("TOKEN=c2VjcmV0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "p.json"), []byte(`{"resourceType":"Patient"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(root, "leak.json")); err != nil {
		t.Skipf("symlinks not available: %v", err)
	}

	resources, skipped, err := LoadResources(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 {
		t.Errorf("want only p.json loaded, got %d resources", len(resources))
	}
	if len(skipped) != 1 || filepath.Base(skipped[0].Path) != "leak.json" {
		t.Fatalf("want leak.json skipped, got %+v", skipped)
	}
	if strings.Contains(skipped[0].Reason, "TOKEN") || !strings.Contains(skipped[0].Reason, "symbolic link") {
		t.Errorf("reason = %q, want the link named without the target's content", skipped[0].Reason)
	}
}
