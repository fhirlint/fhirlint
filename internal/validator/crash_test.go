package validator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The validator's stdout around a crash, as validator 6.10.3 writes it for
// BodyStructure-0c5948ba-… from kbv.basis#1.9.0 (hapifhir/org.hl7.fhir.core#2736),
// colour codes included. The finished file is followed by its timing line, the
// crashed one by the exception.
const crashLog = "\x1b[0;39m\x1b[39mValidating\n" +
	"\x1b[0;39m\x1b[39m  Validate /work/in/a-patient.json\n" +
	"\x1b[0;39m\x1b[39m 00:00.294\n" +
	"\x1b[0;39m\x1b[39m  Validate /work/in/b-crash.json\n" +
	"\x1b[0;39m\x1b[1;31mException generating snapshot for http://hl7.org/fhir/5.0/StructureDefinition/extension-BodyStructure.includedStructure|4.0.1: invalid constrained type\n" +
	"\x1b[0;39m\x1b[1;31mEncountered an exception during validation\n" +
	"\tat org.hl7.fhir.validation.instance.InstanceValidator.checkChild(InstanceValidator.java:7623)\n" +
	"\x1b[39mDone. Times: Loading: 00:25.972, validation: 00:02.098 (#2). Max Memory = 1024Mb\n"

func TestCrashedFile_NamesTheFileAndTheException(t *testing.T) {
	paths := []string{"/work/in/a-patient.json", "/work/in/b-crash.json", "/work/in/c-patient.json"}
	idx, msg, ok := crashedFile(crashLog, paths)
	if !ok {
		t.Fatal("crash not recognised")
	}
	if idx != 1 {
		t.Errorf("crashed file = %d, want 1 (b-crash.json)", idx)
	}
	want := "Exception generating snapshot for http://hl7.org/fhir/5.0/StructureDefinition/extension-BodyStructure.includedStructure|4.0.1: invalid constrained type"
	if msg != want {
		t.Errorf("message = %q, want %q", msg, want)
	}
}

// fhirlint may pass relative paths; the validator logs absolute ones.
func TestCrashedFile_MatchesARelativeInput(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	abs := filepath.Join(dir, "in", "x.json")
	log := "  Validate " + abs + "\nboom\n"
	idx, _, ok := crashedFile(log, []string{filepath.Join("in", "other.json"), filepath.Join("in", "x.json")})
	if !ok || idx != 1 {
		t.Errorf("crashedFile = (%d, %v), want (1, true)", idx, ok)
	}
}

func TestCrashedFile_DoesNotGuess(t *testing.T) {
	paths := []string{"/work/in/a.json", "/work/in/b.json"}
	for name, log := range map[string]string{
		"crash while loading, before any file": "Loading\nException in thread \"main\" java.lang.RuntimeException: package not found\n",
		"last file was finished":               "  Validate /work/in/a.json\n 00:00.100\n  Validate /work/in/b.json\n 00:00.200\nException writing output\n",
		"path is not one of ours":              "  Validate /elsewhere/z.json\nboom\n",
	} {
		t.Run(name, func(t *testing.T) {
			if idx, msg, ok := crashedFile(log, paths); ok {
				t.Errorf("blamed file %d (%q), want no answer", idx, msg)
			}
		})
	}
}

func TestCrashedFile_TruncatesLongExceptions(t *testing.T) {
	long := strings.Repeat("ä", maxCrashMessageLen+50)
	_, msg, ok := crashedFile("  Validate /a.json\n"+long+"\n", []string{"/a.json"})
	if !ok {
		t.Fatal("crash not recognised")
	}
	if got := len([]rune(msg)); got != maxCrashMessageLen+1 || !strings.HasSuffix(msg, "…") {
		t.Errorf("message has %d runes, want %d ending in …", got, maxCrashMessageLen+1)
	}
}

func TestResultIncomplete(t *testing.T) {
	if !crashResult("a.json", "boom").Incomplete() {
		t.Error("crash result not incomplete")
	}
	if !notValidatedResult("a.json", "why").Incomplete() {
		t.Error("not-validated result not incomplete")
	}
	if (&Result{Issues: []Issue{{Severity: "fatal", MessageID: "Validation_VAL_Unknown"}}}).Incomplete() {
		t.Error("a validator finding counted as incomplete")
	}
}

// fakeCrashingJava is a stand-in for the validator. Like the real one, it logs
// "  Validate <absolute path>" per file and a timing line after each one it
// finishes. On a file whose name contains "crash" it logs an exception and
// exits 1 without writing output; otherwise it writes a Bundle with one
// OperationOutcome per file, carrying the file's name, so the test can tell
// which result belongs where. Every start is appended to starts.log.
const fakeCrashingJava = `#!/bin/sh
dir=$(dirname "$0")
echo start >> "$dir/starts.log"
inputs=""
out=""
while [ $# -gt 0 ]; do
  case "$1" in
    -jar) shift ;;
    -version) shift ;;
    -output) shift; out="$1" ;;
    -*) ;;
    *.json) inputs="$inputs $1" ;;
  esac
  shift
done
entries=""
for f in $inputs; do
  echo "  Validate $f"
  case "$f" in
    *crash*)
      echo "Exception generating snapshot for $(basename "$f")"
      echo "	at org.hl7.fhir.Some.frame(Some.java:1)"
      exit 1 ;;
  esac
  echo " 00:00.010"
  e="{\"resource\":{\"resourceType\":\"OperationOutcome\",\"issue\":[{\"severity\":\"information\",\"details\":{\"text\":\"checked $(basename "$f")\"}}]}}"
  if [ -z "$entries" ]; then entries="$e"; else entries="$entries,$e"; fi
done
printf '{"resourceType":"Bundle","entry":[%s]}' "$entries" > "$out"
exit 0
`

// setupFakeJava puts the fake validator first on PATH and returns a dummy JAR
// and the inputs, created in dir under the given names.
func setupFakeJava(t *testing.T, names ...string) (jar string, inputs []string, dir string) {
	t.Helper()
	dir = t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "java"), []byte(fakeCrashingJava), 0o755); err != nil { //nolint:gosec // test helper must be executable
		t.Fatal(err)
	}
	// ZIP magic bytes, so EnsureJAR accepts it.
	jar = filepath.Join(dir, "validator_cli.jar")
	if err := os.WriteFile(jar, []byte{0x50, 0x4B, 0x03, 0x04}, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte(`{"resourceType":"Patient"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		inputs = append(inputs, p)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return jar, inputs, dir
}

func jvmStarts(t *testing.T, dir string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "starts.log")) //nolint:gosec // test temp dir
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(data), "start")
}

func onlyIssue(t *testing.T, r *Result) Issue {
	t.Helper()
	if r == nil || len(r.Issues) != 1 {
		t.Fatalf("want one issue, got %+v", r)
	}
	return r.Issues[0]
}

// One file that crashes the validator must not cost the others their results
// (#471): it gets a fatal issue, the files before and after it are validated.
func TestRunMultiple_ContainsACrash(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test")
	}
	jar, inputs, dir := setupFakeJava(t, "a.json", "b-crash.json", "c.json")

	results, err := RunMultiple(inputs, Options{FHIRVersion: "4.0.1", JARPath: jar})
	if err != nil {
		t.Fatalf("RunMultiple: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}

	for _, i := range []int{0, 2} {
		r := results[i]
		if r.Filename != inputs[i] {
			t.Errorf("result %d is for %s, want %s", i, r.Filename, inputs[i])
		}
		if got := onlyIssue(t, r).Message; got != "checked "+filepath.Base(inputs[i]) {
			t.Errorf("result %d message = %q, want the validator's finding for its own file", i, got)
		}
		if !r.Valid {
			t.Errorf("result %d not valid", i)
		}
	}

	crash := results[1]
	iss := onlyIssue(t, crash)
	if crash.Filename != inputs[1] || crash.Valid {
		t.Errorf("crash result = %+v, want invalid result for %s", crash, inputs[1])
	}
	if iss.Severity != "fatal" || iss.MessageID != MessageIDValidatorCrash {
		t.Errorf("crash issue = %+v, want fatal %s", iss, MessageIDValidatorCrash)
	}
	if !strings.Contains(iss.Message, "Exception generating snapshot for b-crash.json") {
		t.Errorf("crash message %q lacks the exception", iss.Message)
	}

	if n := jvmStarts(t, dir); n != 2 {
		t.Errorf("JVM started %d times, want 2", n)
	}
}

// Run goes through RunMultiple, so a crash on its one file is the same fatal
// issue rather than an error.
func TestRun_ReportsACrashAsAFinding(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test")
	}
	jar, inputs, _ := setupFakeJava(t, "crash.json")

	r, err := Run(inputs[0], Options{FHIRVersion: "4.0.1", JARPath: jar})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if iss := onlyIssue(t, r); iss.MessageID != MessageIDValidatorCrash || r.Valid {
		t.Errorf("result = %+v, want invalid with %s", r, MessageIDValidatorCrash)
	}
}

// A directory full of crashers must not cost one JVM start per file.
func TestRunMultiple_StopsAfterMaxCrashes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test")
	}
	jar, inputs, dir := setupFakeJava(t, "crash1.json", "crash2.json", "crash3.json", "crash4.json", "ok.json")

	results, err := RunMultiple(inputs, Options{FHIRVersion: "4.0.1", JARPath: jar})
	if err != nil {
		t.Fatalf("RunMultiple: %v", err)
	}
	if n := jvmStarts(t, dir); n != maxValidatorCrashes {
		t.Errorf("JVM started %d times, want %d", n, maxValidatorCrashes)
	}
	for i := 0; i < maxValidatorCrashes; i++ {
		if id := onlyIssue(t, results[i]).MessageID; id != MessageIDValidatorCrash {
			t.Errorf("result %d id = %s, want %s", i, id, MessageIDValidatorCrash)
		}
	}
	for _, i := range []int{3, 4} {
		iss := onlyIssue(t, results[i])
		if iss.MessageID != MessageIDNotValidated || iss.Severity != "fatal" || results[i].Valid {
			t.Errorf("result %d = %+v, want fatal %s", i, results[i], MessageIDNotValidated)
		}
		if results[i].Filename != inputs[i] {
			t.Errorf("result %d is for %s, want %s", i, results[i].Filename, inputs[i])
		}
	}
}

// When --timeout runs out during a re-run, the crash already found is kept
// and the rest is reported as not validated rather than discarded.
func TestRunMultiple_TimeoutDuringReRunKeepsTheCrash(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test")
	}
	jar, inputs, dir := setupFakeJava(t, "a-crash.json", "b.json")
	// The second start hangs. exec, so the kill reaches sleep and the pipes close.
	slow := strings.Replace(fakeCrashingJava, `echo start >> "$dir/starts.log"`,
		`echo start >> "$dir/starts.log"
[ "$(grep -c start "$dir/starts.log")" -gt 1 ] && exec sleep 60`, 1)
	if err := os.WriteFile(filepath.Join(dir, "java"), []byte(slow), 0o755); err != nil { //nolint:gosec // test helper must be executable
		t.Fatal(err)
	}

	results, err := RunMultiple(inputs, Options{FHIRVersion: "4.0.1", JARPath: jar, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("RunMultiple: %v", err)
	}
	if id := onlyIssue(t, results[0]).MessageID; id != MessageIDValidatorCrash {
		t.Errorf("result 0 id = %s, want %s", id, MessageIDValidatorCrash)
	}
	iss := onlyIssue(t, results[1])
	if iss.MessageID != MessageIDNotValidated || !strings.Contains(iss.Message, "--timeout") {
		t.Errorf("result 1 issue = %+v, want %s naming --timeout", iss, MessageIDNotValidated)
	}
}

// No Validate line to go by: the old error, not a guess.
func TestRunMultiple_UnrecognisedFailureStaysAnError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test")
	}
	jar, inputs, dir := setupFakeJava(t, "a.json")
	if err := os.WriteFile(filepath.Join(dir, "java"), []byte("#!/bin/sh\necho 'Exception in thread main'\nexit 1\n"), 0o755); err != nil { //nolint:gosec // test helper must be executable
		t.Fatal(err)
	}
	_, err := RunMultiple(inputs, Options{FHIRVersion: "4.0.1", JARPath: jar})
	if err == nil || !strings.Contains(err.Error(), "validator produced no result") {
		t.Errorf("err = %v, want 'validator produced no result'", err)
	}
}
