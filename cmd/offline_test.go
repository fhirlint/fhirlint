package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

// With nothing else set, an offline run would otherwise send the validator to
// tx.fhir.org and fail there.
func TestApplyOffline_SkipsTerminologyByDefault(t *testing.T) {
	opts := validator.Options{}
	var out bytes.Buffer

	if err := applyOffline(&opts, &out); err != nil {
		t.Fatalf("applyOffline: %v", err)
	}
	if !opts.NoTerminologyServer {
		t.Error("terminology server left on for an offline run")
	}
	if !strings.Contains(out.String(), "terminology checks are skipped") {
		t.Errorf("output = %q, want it to say terminology was dropped", out.String())
	}
	if !strings.Contains(out.String(), "tx warm") {
		t.Errorf("output = %q, want it to point at the way to keep terminology", out.String())
	}
}

// Replaying from the local server means the JAR cannot be given -no-http-access
// (its PROHIBITED policy blocks loopback too), so the run must say which
// guarantee it actually has rather than implying the stronger one.
func TestApplyOffline_ReplayStatesTheWeakerGuarantee(t *testing.T) {
	opts := validator.Options{TerminologyServer: "http://127.0.0.1:8081"}
	var out bytes.Buffer

	if err := applyOffline(&opts, &out); err != nil {
		t.Fatalf("applyOffline: %v", err)
	}
	if opts.NoTerminologyServer {
		t.Error("replay server switched off by the offline policy")
	}
	msg := out.String()
	if !strings.Contains(msg, "network block cannot be used") {
		t.Errorf("output = %q, want the residual risk stated", msg)
	}
	if !strings.Contains(msg, "downloads nothing") {
		t.Errorf("output = %q, want it to say what is still guaranteed", msg)
	}
}

func TestRequireCachedIGs(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("HOME", cacheRoot)
	pkgDir := filepath.Join(cacheRoot, ".fhir", "packages", "kbv.basis#1.9.0", "package")
	if err := os.MkdirAll(pkgDir, 0o750); err != nil {
		t.Fatal(err)
	}

	if err := requireCachedIGs([]string{"kbv.basis#1.9.0"}, nil); err != nil {
		t.Errorf("cached package rejected: %v", err)
	}

	err := requireCachedIGs([]string{"kbv.basis#1.9.0", "hl7.fhir.us.core#9.0.0"}, nil)
	if err == nil {
		t.Fatal("err = nil for a package that is not cached")
	}
	if !strings.Contains(err.Error(), "hl7.fhir.us.core#9.0.0") {
		t.Errorf("err = %q, want it to name the missing package", err)
	}

	// A canonical URL or a bare profile name is not something to fetch, so it
	// must not be mistaken for an uncached package.
	if err := requireCachedIGs(nil, []string{"http://hl7.org/fhir/StructureDefinition/Patient"}); err != nil {
		t.Errorf("canonical profile URL treated as a package: %v", err)
	}
}

// serve and lsp reach the same policy through the server config: the terminology
// decision is made once for the server's whole lifetime.
func TestApplyOfflineServer(t *testing.T) {
	var out bytes.Buffer
	cfg := validator.ServerConfig{}

	if err := applyOfflineServer(&cfg, &out); err != nil {
		t.Fatalf("applyOfflineServer: %v", err)
	}
	if !cfg.Offline {
		t.Error("Offline not set on the server config")
	}
	if !cfg.NoTerminologyServer {
		t.Error("terminology server left on for an offline server")
	}
	if !strings.Contains(out.String(), "terminology checks are skipped") {
		t.Errorf("output = %q, want it to say terminology was dropped", out.String())
	}

	// Replaying: same weaker guarantee as a one-shot run, same wording.
	out.Reset()
	replay := validator.ServerConfig{TerminologyServer: "http://127.0.0.1:8081"}
	if err := applyOfflineServer(&replay, &out); err != nil {
		t.Fatalf("applyOfflineServer: %v", err)
	}
	if replay.NoTerminologyServer {
		t.Error("replay server switched off by the offline policy")
	}
	if !strings.Contains(out.String(), "network block cannot be used") {
		t.Errorf("output = %q, want the residual risk stated", out.String())
	}
}
