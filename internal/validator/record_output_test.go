// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build record

package validator

import (
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

// recordVersion is a release tag, nothing a shell could make more of.
var recordVersion = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// TestRecordValidatorOutput runs the JAR named by FHIRLINT_VALIDATOR_VERSION
// over every recorded case and writes what it left behind into
// testdata/validator-output/<version>/ (#715). It needs Java and the network,
// and it is not part of any CI job: scripts/record-validator-output.sh runs it
// by hand when the validator version moves.
func TestRecordValidatorOutput(t *testing.T) {
	version := os.Getenv("FHIRLINT_VALIDATOR_VERSION")
	if !recordVersion.MatchString(version) {
		t.Fatalf("FHIRLINT_VALIDATOR_VERSION=%q: set it to the release to record, e.g. 7.0.6", version)
	}
	jar, err := EnsureJAR("", version)
	if err != nil {
		t.Fatal(err)
	}
	if got := jarVersion(jar); got != version {
		t.Fatalf("the JAR at %s is %s, not %s", jar, got, version)
	}

	root, err := filepath.Abs(recordedOutputDir)
	if err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(root, version)
	// outDir is testdata/validator-output/<version>, the version checked
	// against recordVersion above.
	if err := os.RemoveAll(outDir); err != nil { //nolint:gosec // see above
		t.Fatal(err)
	}
	if err := os.MkdirAll(outDir, 0o750); err != nil { //nolint:gosec // see above
		t.Fatal(err)
	}
	if err := copyMessageBundle(jar, filepath.Join(outDir, "Messages.properties.gz")); err != nil {
		t.Fatal(err)
	}

	home, _ := os.UserHomeDir()
	for _, c := range recordedCases {
		t.Run(c.name, func(t *testing.T) {
			work := t.TempDir()
			if err := os.CopyFS(work, os.DirFS(filepath.Join(root, recordedInputsDir, c.name))); err != nil {
				t.Fatal(err)
			}
			resolved, _ := filepath.EvalSymlinks(work)
			t.Chdir(work)

			opts := c.opts
			opts.FHIRVersion = "4.0.1"
			// Its own terminology cache, so no earlier run answers for the
			// server.
			opts.TxCache = filepath.Join(work, "tx-cache")
			out, err := runJAR(context.Background(), jar, c.paths, opts)
			if err != nil {
				t.Fatal(err)
			}

			n := recordingNormaliser{work: []string{resolved, work}, home: home}
			dir := filepath.Join(outDir, c.name)
			write := func(name, content string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.MkdirAll(dir, 0o750); err != nil {
				t.Fatal(err)
			}
			write("stdout.txt", n.normalise(out.stdout))
			write("stderr.txt", n.normalise(out.stderr))
			if len(out.json) > 0 {
				write("output.json", n.normalise(string(out.json)))
			}
			meta, err := json.MarshalIndent(recordedRun{Paths: c.paths, Failed: out.failed}, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			write("run.json", string(meta)+"\n")
			t.Logf("%s: exit failed=%v, %d bytes of output", c.name, out.failed, len(out.json))
		})
	}
}

// copyMessageBundle keeps the JAR's English message bundle next to the
// recordings, compressed. The id inference of #449 reads it from the JAR that
// ran; the tests read it from here.
func copyMessageBundle(jar, dst string) error {
	zr, err := zip.OpenReader(jar)
	if err != nil {
		return err
	}
	defer func() { _ = zr.Close() }()
	src, err := zr.Open(messagesEntry)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	f, err := os.Create(dst) //nolint:gosec // a path below the repository's testdata
	if err != nil {
		return err
	}
	zw, err := gzip.NewWriterLevel(f, gzip.BestCompression)
	if err != nil {
		_ = f.Close()
		return err
	}
	zw.ModTime = time.Time{} // byte-identical across re-recordings of the same JAR
	if _, err := io.Copy(zw, src); err != nil {
		_ = f.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
