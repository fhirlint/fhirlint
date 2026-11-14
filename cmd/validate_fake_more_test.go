//go:build !integration

package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// More end-to-end runs against the fake validator (#595): the paths through
// validatePaths that only several inputs, XML, URLs, overrides and git reach.

func logCalls(t *testing.T) string {
	t.Helper()
	log := filepath.Join(t.TempDir(), "calls.jsonl")
	t.Setenv(fakeJavaLogEnv, log)
	return log
}

func TestFakeValidate_ExtractOverSeveralFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.json", `{"data":`+errPatient+`}`)
	writeFile(t, dir, "b.json", `{"other":{}}`)
	r := runFake(t, dir, "validate", "--no-color", "--extract", "$.data", "a.json", "b.json")
	mustFail(t, r)
	if !strings.Contains(r.stdout, "Fake error found") {
		t.Errorf("the extracted resource of a.json must be validated:\n%s", r.stdout)
	}
	// b.json has nothing at $.data: it is reported, not silently dropped.
	if !strings.Contains(r.stdout, "b.json") {
		t.Errorf("a file the extract path misses must be reported:\n%s", r.stdout)
	}
}

func TestFakeValidate_IgnoreOverSeveralFilesIncludingXML(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.json", errPatient)
	writeFile(t, dir, "b.xml", `<Patient xmlns="http://hl7.org/fhir"><id value="x"/><note value="FAKE_ERROR"/></Patient>`)
	mustFail(t, runFake(t, dir, "validate", "a.json", "b.xml"))
	r := runFake(t, dir, "validate", "--no-color", "--ignore", "$.note", "a.json", "b.xml")
	mustPass(t, r)
	if strings.Contains(r.stdout, "Fake error found") {
		t.Errorf("--ignore must remove the field from JSON and XML alike:\n%s", r.stdout)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "b.xml")) //nolint:gosec // test file
	if !strings.Contains(string(data), "FAKE_ERROR") {
		t.Error("--ignore must not modify the XML input file")
	}
}

func TestFakeValidate_BundleEntriesOverSeveralFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "b1.json", `{"resourceType":"Bundle","type":"collection","entry":[{"resource":`+errPatient+`}]}`)
	writeFile(t, dir, "b2.json", `{"resourceType":"Bundle","type":"collection","entry":[{"resource":`+cleanPatient+`}]}`)
	r := runFake(t, dir, "validate", "--no-color", "--bundle-entries", "b1.json", "b2.json")
	mustFail(t, r)
	if !strings.Contains(r.stdout, "Files: 4") {
		t.Errorf("two Bundles and their two entries:\n%s", r.stdout)
	}
}

func TestFakeValidate_URLs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Patient/ok":
			_, _ = w.Write([]byte(cleanPatient))
		case "/Patient/bad":
			_, _ = w.Write([]byte(errPatient))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	r := runFake(t, dir, "validate", "--no-color", "--url", srv.URL+"/Patient/ok", "--url", srv.URL+"/Patient/bad")
	mustFail(t, r)
	if !strings.Contains(r.stdout, srv.URL+"/Patient/bad") {
		t.Errorf("a URL input is reported under its URL:\n%s", r.stdout)
	}
	if r := runFake(t, dir, "validate", "--url", srv.URL+"/missing"); r.err == nil {
		t.Error("a 404 must fail the run")
	}
	mustPass(t, runFake(t, dir, "validate", "--url", srv.URL+"/Patient/ok"))
}

func TestFakeValidate_OverridesPassTheirOwnOptions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "kbv/med.json", `{"resourceType":"Medication","id":"m1"}`)
	writeFile(t, dir, "other/p.json", cleanPatient)
	writeFile(t, dir, "fhirlint.yml", `
overrides:
  - files: ["kbv/**"]
    ig:
      - kbv.basis#1.5.0
`)
	log := logCalls(t)
	mustPass(t, runFake(t, dir, "validate", "kbv", "other"))
	calls := fakeJavaCalls(t, log)
	if len(calls) != 2 {
		t.Fatalf("files with different options run in separate groups, got %d calls: %v", len(calls), calls)
	}
	var withIG int
	for _, c := range calls {
		if slices.Contains(argAfter(c, "-ig"), "kbv.basis#1.5.0") {
			withIG++
			if !strings.Contains(strings.Join(c, " "), "med.json") {
				t.Errorf("the override's IG must go with the kbv file only: %v", c)
			}
		}
	}
	if withIG != 1 {
		t.Errorf("exactly one call carries the override's IG: %v", calls)
	}
}

func TestFakeValidate_ProfileMapPicksTheProfile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "p.json", cleanPatient)
	writeFile(t, dir, "o.json", `{"resourceType":"Observation","id":"o1"}`)
	writeFile(t, dir, "fhirlint.yml", `
profile-map:
  Patient: http://example.org/StructureDefinition/my-patient
`)
	log := logCalls(t)
	mustPass(t, runFake(t, dir, "validate", "p.json", "o.json"))
	var got []string
	for _, c := range fakeJavaCalls(t, log) {
		if slices.Contains(argAfter(c, "-profile"), "http://example.org/StructureDefinition/my-patient") {
			got = append(got, strings.Join(c, " "))
		}
	}
	if len(got) != 1 || !strings.Contains(got[0], "p.json") || strings.Contains(got[0], "o.json") {
		t.Errorf("the mapped profile must be passed for the Patient only: %v", got)
	}
}

func TestFakeValidate_LocalCodeSystemBecomesAnIG(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "p.json", cleanPatient)
	writeFile(t, dir, "cs.json", `{"resourceType":"CodeSystem","id":"local","url":"http://example.org/cs","status":"active","content":"complete","concept":[{"code":"a"}]}`)
	log := logCalls(t)
	mustPass(t, runFake(t, dir, "validate", "--codesystem", "cs.json", "p.json"))
	igs := argAfter(fakeJavaCalls(t, log)[0], "-ig")
	if len(igs) == 0 {
		t.Fatal("a --codesystem file must reach the validator as a local IG")
	}
}

func TestFakeValidate_NoTerminologyServerAndBestPractice(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "p.json", cleanPatient)
	log := logCalls(t)
	mustPass(t, runFake(t, dir, "validate", "--no-terminology-server", "--best-practice", "error", "--fhir-version", "5.0.0", "p.json"))
	c := fakeJavaCalls(t, log)[0]
	for flag, want := range map[string]string{"-tx": "n/a", "-best-practice": "error", "-version": "5.0.0"} {
		if !slices.Contains(argAfter(c, flag), want) {
			t.Errorf("%s %s missing from the validator call: %v", flag, want, c)
		}
	}
}

func TestFakeValidate_ValidatorArgGuard(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "p.json", cleanPatient)
	if r := runFake(t, dir, "validate", "--validator-arg", "-output", "--validator-arg", "x.json", "p.json"); r.err == nil {
		t.Error("--validator-arg must not be able to redirect the output fhirlint parses")
	}
	log := logCalls(t)
	mustPass(t, runFake(t, dir, "validate", "--validator-arg", "-allow-example-urls", "--validator-arg", "true", "p.json"))
	if !slices.Contains(fakeJavaCalls(t, log)[0], "-allow-example-urls") {
		t.Error("an allowed --validator-arg must reach the validator")
	}
}

func TestFakeValidate_Since(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.org", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.org")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	writeFile(t, dir, "old.json", errPatient)
	writeFile(t, dir, "new.json", cleanPatient)
	git("add", ".")
	git("commit", "-q", "-m", "init")
	git("checkout", "-q", "-b", "feature")
	writeFile(t, dir, "new.json", strings.Replace(cleanPatient, "p1", "p9", 1))
	git("commit", "-q", "-am", "touch new")

	// old.json has an error but did not change, so --since main leaves it out.
	r := runFake(t, dir, "validate", "--no-color", "--since", "main", ".")
	mustPass(t, r)
	if strings.Contains(r.stdout, "old.json") {
		t.Errorf("an unchanged file must not be validated:\n%s", r.stdout)
	}
	if r := runFake(t, dir, "validate", "--since", "no-such-ref", "."); r.err == nil {
		t.Error("an unknown ref must fail rather than validate everything")
	}
}
