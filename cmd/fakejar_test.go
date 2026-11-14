//go:build !integration

package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// The unit tests of the validate pipeline run against a stand-in for the HL7
// validator (#595): this test binary, copied into a directory as "java". When
// it starts with fakeJavaEnv set, TestMain hands it to fakeJava instead of the
// tests. Everything between flag parsing and the exit code then runs for real,
// without Java and on every platform, while the integration tests keep
// covering the real JAR.
const fakeJavaEnv = "FHIRLINT_TEST_FAKE_JAVA"

// fakeJavaRefuseEnv makes the fake answer "java -version" but fail every
// validation run, to prove a result came from somewhere else.
const fakeJavaRefuseEnv = "FHIRLINT_TEST_FAKE_JAVA_REFUSE"

// fakeJavaLogEnv names a file the fake appends each validation call's
// arguments to, one JSON array per line.
const fakeJavaLogEnv = "FHIRLINT_TEST_FAKE_JAVA_LOG"

// fakeJavaDir holds the "java" executable; empty when it could not be made.
var fakeJavaDir string

func TestMain(m *testing.M) {
	if os.Getenv(fakeJavaEnv) != "" {
		os.Exit(fakeJava(os.Args[1:], os.Stdout, os.Stderr))
	}
	dir, err := os.MkdirTemp("", "fhirlint-fakejava-")
	if err == nil {
		if err = installFakeJava(dir); err == nil {
			fakeJavaDir = dir
		}
	}
	code := m.Run()
	if dir != "" {
		_ = os.RemoveAll(dir)
	}
	os.Exit(code)
}

func installFakeJava(dir string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	name := "java"
	if runtime.GOOS == "windows" {
		name = "java.exe"
	}
	dst := filepath.Join(dir, name)
	if os.Link(self, dst) == nil {
		return nil
	}
	in, err := os.Open(self) //nolint:gosec // the running test binary
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o700) //nolint:gosec // test helper in a temp dir
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// fakeJava answers the calls fhirlint makes to java: "java -version", and a
// validation run that writes its result to -output. Each input gets an
// OperationOutcome whose issues are decided by markers in the input text:
//
//	FAKE_ERROR       an error, message id FAKE_ERROR_ID, at Patient.name line 1
//	FAKE_WARNING     a warning, message id FAKE_WARNING_ID, at Patient
//	FAKE_CRASH       the JAR exits 1 without writing output
//
// An input without a marker gets the validator's "All OK" information issue.
func fakeJava(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "-version" {
		_, _ = fmt.Fprintln(stderr, `openjdk version "21.0.5" 2024-10-15`)
		return 0
	}
	jar, ver, out := -1, -1, ""
	for i, a := range args {
		switch {
		case a == "-jar" && jar < 0:
			jar = i
		case a == "-version" && ver < 0 && jar >= 0:
			ver = i
		case a == "-output" && i+1 < len(args):
			out = args[i+1]
		}
	}
	if log := os.Getenv(fakeJavaLogEnv); log != "" {
		if f, err := os.OpenFile(log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil { //nolint:gosec // path set by the test
			b, _ := json.Marshal(args)
			_, _ = f.Write(append(b, '\n'))
			_ = f.Close()
		}
	}
	if os.Getenv(fakeJavaRefuseEnv) != "" {
		_, _ = fmt.Fprintln(stderr, "fake java: validation refused")
		return 3
	}
	if jar < 0 || ver < 0 || out == "" {
		_, _ = fmt.Fprintf(stderr, "fake java: unsupported call %q\n", args)
		return 2
	}
	_, _ = fmt.Fprintln(stdout, "FHIR Validation tool Version 7.0.5 (fake)")
	inputs := args[jar+2 : ver]
	var oos []map[string]any
	for _, in := range inputs {
		data, err := os.ReadFile(in) //nolint:gosec // inputs named by the test
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "fake java: %v\n", err)
			return 2
		}
		if bytes.Contains(data, []byte("FAKE_CRASH")) {
			_, _ = fmt.Fprintln(stdout, "Exception in thread \"main\" java.lang.IllegalStateException: fake crash")
			return 1
		}
		oos = append(oos, fakeOutcome(string(data)))
	}
	var doc any = oos[0]
	if len(oos) > 1 {
		entries := make([]map[string]any, len(oos))
		for i, oo := range oos {
			entries[i] = map[string]any{"fullUrl": "file://" + inputs[i], "resource": oo}
		}
		doc = map[string]any{"resourceType": "Bundle", "type": "collection", "entry": entries}
	}
	b, _ := json.Marshal(doc)
	if err := os.WriteFile(out, b, 0o600); err != nil {
		_, _ = fmt.Fprintf(stderr, "fake java: %v\n", err)
		return 2
	}
	return 0
}

func fakeOutcome(text string) map[string]any {
	issue := func(sev, code, msg, id string, expr string, line int) map[string]any {
		ext := []map[string]any{{"url": "http://hl7.org/fhir/StructureDefinition/operationoutcome-message-id", "valueCode": id}}
		if line > 0 {
			ext = append(ext,
				map[string]any{"url": "http://hl7.org/fhir/StructureDefinition/operationoutcome-issue-line", "valueInteger": line},
				map[string]any{"url": "http://hl7.org/fhir/StructureDefinition/operationoutcome-issue-col", "valueInteger": 2})
		}
		return map[string]any{"severity": sev, "code": code, "details": map[string]any{"text": msg}, "expression": []string{expr}, "extension": ext}
	}
	var issues []map[string]any
	if strings.Contains(text, "FAKE_ERROR") {
		issues = append(issues, issue("error", "invalid", "Fake error found", "FAKE_ERROR_ID", "Patient.name", 1))
	}
	if strings.Contains(text, "FAKE_WARNING") {
		issues = append(issues, issue("warning", "business-rule", "Fake warning found", "FAKE_WARNING_ID", "Patient", 1))
	}
	if len(issues) == 0 {
		issues = append(issues, map[string]any{"severity": "information", "code": "informational", "details": map[string]any{"text": "All OK"}})
	}
	return map[string]any{"resourceType": "OperationOutcome", "issue": issues}
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
	t.Setenv(fakeJavaEnv, "1")
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
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}
