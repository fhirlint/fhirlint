// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !integration

package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// The unit tests of the validate pipeline run against a stand-in for the HL7
// validator (#595): internal/tools/fakejava, built once per test run into a
// directory as "java". Everything between flag parsing and the exit code then
// runs for real, without Java and on every platform, while the integration
// tests keep covering the real JAR. The fake's markers and switches are
// described in that package.
const (
	fakeJavaLogEnv    = "FAKEJAVA_LOG"
	fakeJavaRefuseEnv = "FAKEJAVA_REFUSE"
)

// fakeJavaDir holds the "java" executable; empty when it could not be built.
var fakeJavaDir string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fhirlint-fakejava-")
	if err == nil {
		if err = buildFakeJava(dir); err == nil {
			fakeJavaDir = dir
		} else {
			fmt.Fprintf(os.Stderr, "fake java not built, its tests are skipped: %v\n", err)
		}
	}
	code := m.Run()
	if dir != "" {
		_ = os.RemoveAll(dir)
	}
	os.Exit(code)
}

// buildFakeJava builds internal/tools/fakejava into dir as java (java.exe on
// Windows), with the go command that runs these tests.
func buildFakeJava(dir string) error {
	gobin, err := exec.LookPath("go")
	if err != nil {
		return err
	}
	name := "java"
	if runtime.GOOS == "windows" {
		name = "java.exe"
	}
	cmd := exec.Command(gobin, "build", "-o", filepath.Join(dir, name), "github.com/fhirlint/fhirlint/internal/tools/fakejava") //nolint:gosec // the go toolchain, fixed arguments
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%v: %s", err, out)
	}
	return nil
}

// fakeRun is the outcome of one in-process fhirlint run.
type fakeRun struct {
	err            error
	stdout, stderr string
}

// runFake runs `fhirlint <args>` in-process against the fake validator, from
// dir, with HOME and the update checks pointed away from the real machine.
// Flags and config are reset afterwards, since cobra and viper keep them in
// package state.
func runFake(t *testing.T, dir string, args ...string) fakeRun {
	t.Helper()
	if fakeJavaDir == "" {
		t.Skip("fake java could not be installed")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("PATH", fakeJavaDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(noUpdateNotifierEnvVar, "1")
	t.Setenv("CI", "true")
	t.Setenv("FHIRLINT_JAR", fakeJAR(t, "7.0.5"))
	return runFakeKeepEnv(t, dir, args...)
}

// runFakeKeepEnv is runFake without touching the environment, for a second
// run in the same test that changed it on purpose.
func runFakeKeepEnv(t *testing.T, dir string, args ...string) fakeRun {
	t.Helper()
	t.Chdir(dir)
	t.Cleanup(resetCommandState)

	resetCommandState()
	outR, outW, _ := os.Pipe()
	errR, errW, _ := os.Pipe()
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	var outB, errB bytes.Buffer
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(&outB, outR); done <- struct{}{} }()
	go func() { _, _ = io.Copy(&errB, errR); done <- struct{}{} }()

	rootCmd.SetArgs(args)
	err := rootCmd.Execute()

	os.Stdout, os.Stderr = oldOut, oldErr
	_ = outW.Close()
	_ = errW.Close()
	<-done
	<-done
	return fakeRun{err: err, stdout: outB.String(), stderr: errB.String()}
}

// resetCommandState puts every flag of every command back to its default and
// drops a config file read by an earlier run.
func resetCommandState() {
	var reset func(c *cobra.Command)
	reset = func(c *cobra.Command) {
		for _, fs := range []*pflag.FlagSet{c.Flags(), c.PersistentFlags()} {
			fs.VisitAll(func(f *pflag.Flag) {
				if sv, ok := f.Value.(pflag.SliceValue); ok {
					var def []string
					if d := strings.Trim(f.DefValue, "[]"); d != "" {
						def = strings.Split(d, ",")
					}
					_ = sv.Replace(def)
				} else {
					_ = f.Value.Set(f.DefValue)
				}
				f.Changed = false
			})
		}
		for _, sub := range c.Commands() {
			reset(sub)
		}
	}
	reset(rootCmd)
	// Per-run state outside the flags: --since keeps the files it resolved.
	sinceChanged, sinceExcluded = nil, nil
	// Other tests in this package call viper.Reset, which also drops the
	// bindings init made, FHIRLINT_JAR among them; start from a clean viper
	// with those bindings, whatever ran before.
	viper.Reset()
	bindRootConfig()
	bindValidateConfig()
}

// fakeJavaCalls returns the argument lists of the validation runs logged
// through fakeJavaLogEnv.
func fakeJavaCalls(t *testing.T, log string) [][]string {
	t.Helper()
	data, err := os.ReadFile(log) //nolint:gosec // test log
	if err != nil {
		t.Fatalf("no validator call was logged: %v", err)
	}
	var calls [][]string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var args []string
		if err := json.Unmarshal([]byte(line), &args); err != nil {
			t.Fatal(err)
		}
		calls = append(calls, args)
	}
	return calls
}

// argAfter reports the value following flag in args, or "".
func argAfter(args []string, flag string) []string {
	var vals []string
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			vals = append(vals, args[i+1])
		}
	}
	return vals
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil { //nolint:gosec // a test file under the test's temp dir
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil { //nolint:gosec // a test file under the test's temp dir
		t.Fatal(err)
	}
	return p
}
