// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/baseline"
	"github.com/fhirlint/fhirlint/internal/reporter"
	"github.com/fhirlint/fhirlint/internal/suppress"
	"github.com/fhirlint/fhirlint/internal/validator"
)

var update = flag.Bool("update", false, "rewrite testdata/identity.golden.json")

const goldenFile = "testdata/identity.golden.json"

// rules are matched against every issue; the snapshot records which match.
var rules = []string{
	"messageId:Terminology_TX_NoValid_1_CC",
	"constraint:dom-6",
	"expression:Observation.code",
	"pattern:minimum required",
	"messageId:fhirlint-validator-crash",
}

// cases are the inputs whose identity other tools and user files store. They
// must give the same snapshot on every OS, so paths are built with
// filepath.Join and an absolute path is made from the working directory.
func cases(t *testing.T) []*validator.Result {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	long := "Long message " + strings.Repeat("y", 2100)
	return []*validator.Result{
		{
			Filename: filepath.Join("fhir", "Observation-a.json"),
			Issues: []validator.Issue{
				{Severity: "error", Message: "Observation.status: minimum required = 1, but only found 0", Location: "Observation (line 3, col 4)", MessageID: "Validation_VAL_Profile_Minimum"},
				{Severity: "error", Message: "Observation.status: minimum required = 1, but only found 0", Location: "Observation (line 9, col 4)", MessageID: "Validation_VAL_Profile_Minimum"},
				{Severity: "warning", Message: "Constraint failed: dom-6", Location: "Observation", MessageID: "http://hl7.org/fhir/StructureDefinition/DomainResource#dom-6"},
				{Severity: "information", Message: "Code not in value set", Location: "Observation.code.coding[0] (line 12, col 8)", MessageID: "Terminology_TX_NoValid_1_CC", MessageIDInferred: true},
				{Severity: "warning", Message: "No message id here", Location: "Observation.code"},
				{Severity: "error", Message: long, Location: "Observation.note[0].text (line 30, col 10)", MessageID: "Type_Specific_Checks_DT_String_Length"},
			},
		},
		{
			// Absolute: the identity is the path relative to the working directory.
			Filename: filepath.Join(cwd, "fhir", "sub dir", "Patient-ä.json"),
			Issues: []validator.Issue{
				{Severity: "fatal", Message: "the validator crashed on this file: java.lang.NullPointerException", MessageID: validator.MessageIDValidatorCrash},
			},
		},
	}
}

type issueIdentity struct {
	File            string   `json:"file"`
	MessageID       string   `json:"messageId"`
	Location        string   `json:"location"`
	CodeClimate     string   `json:"codeClimateFingerprint"`
	SARIFRuleID     string   `json:"sarifRuleId"`
	SARIFFinger     string   `json:"sarifPartialFingerprint"`
	SuppressMatches []string `json:"suppressMatches"`
}

type snapshotFile struct {
	// Baseline is what fhirlint --baseline-generate writes, sorted.
	Baseline []baseline.Entry `json:"baseline"`
	// Findings are the per-issue identities, in report order.
	Findings []issueIdentity `json:"findings"`
}

func snapshot(t *testing.T) snapshotFile {
	t.Helper()
	results := cases(t)
	dir := t.TempDir()

	// Baseline: the entries Generate writes to fhirlint-baseline.json.
	entries := baseline.Generate(results).Entries
	sort.Slice(entries, func(a, b int) bool {
		x, y := entries[a], entries[b]
		if x.File != y.File {
			return x.File < y.File
		}
		if x.MessageID != y.MessageID {
			return x.MessageID < y.MessageID
		}
		return x.Location < y.Location
	})

	ccPath := filepath.Join(dir, "cc.json")
	if err := reporter.CodeClimate(results, "information", ccPath); err != nil {
		t.Fatal(err)
	}
	var cc []struct {
		Fingerprint string `json:"fingerprint"`
	}
	readJSON(t, ccPath, &cc)

	sarifPath := filepath.Join(dir, "r.sarif")
	if err := reporter.SARIF(results, "information", reporter.RunInfo{}, sarifPath); err != nil {
		t.Fatal(err)
	}
	var sarif struct {
		Runs []struct {
			Results []struct {
				RuleID              string            `json:"ruleId"`
				PartialFingerprints map[string]string `json:"partialFingerprints"`
			} `json:"results"`
		} `json:"runs"`
	}
	readJSON(t, sarifPath, &sarif)

	parsed := make([]suppress.Rule, 0, len(rules))
	for _, r := range rules {
		pr, err := suppress.ParseCLI(r)
		if err != nil {
			t.Fatal(err)
		}
		parsed = append(parsed, pr)
	}

	var out []issueIdentity
	i := 0
	for _, r := range results {
		for _, iss := range r.Issues {
			if i >= len(cc) || i >= len(sarif.Runs[0].Results) {
				t.Fatalf("report has fewer findings than the input (%d)", i)
			}
			sr := sarif.Runs[0].Results[i]
			matches := []string{}
			for k, rule := range parsed {
				if rule.Matches(iss) {
					matches = append(matches, rules[k])
				}
			}
			fp := ""
			for _, v := range sr.PartialFingerprints {
				fp = v
			}
			out = append(out, issueIdentity{
				File:            filepath.Base(r.Filename),
				MessageID:       iss.MessageID,
				Location:        iss.Location,
				CodeClimate:     cc[i].Fingerprint,
				SARIFRuleID:     sr.RuleID,
				SARIFFinger:     fp,
				SuppressMatches: matches,
			})
			i++
		}
	}
	return snapshotFile{Baseline: entries, Findings: out}
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // G304: a file the test just wrote
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

// TestIdentitySnapshot fails when any finding identity changes. Such a change
// makes existing findings look new in GitLab and code scanning, or stops a
// baseline or suppression from matching. If it is intended, run
// go test ./internal/identity -update, commit the snapshot, and explain the
// change in the pull request and the release notes.
func TestIdentitySnapshot(t *testing.T) {
	got, err := json.MarshalIndent(snapshot(t), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	if *update {
		if err := os.WriteFile(goldenFile, got, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(goldenFile)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/identity -update)", err)
	}
	want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
	if !bytes.Equal(got, want) {
		t.Errorf("a finding identity changed; if intended, run go test ./internal/identity -update and explain it in the PR and release notes.\ngot:\n%s", got)
	}
}

// A file named with backslashes on Windows has the same identity as with
// slashes elsewhere.
func TestIdentityWindowsSeparators(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("backslash is a separator only on Windows")
	}
	iss := validator.Issue{Severity: "error", Message: "m", Location: "Patient (line 1, col 1)", MessageID: "X"}
	fp := func(name string) string {
		p := filepath.Join(t.TempDir(), "cc.json")
		if err := reporter.CodeClimate([]*validator.Result{{Filename: name, Issues: []validator.Issue{iss}}}, "information", p); err != nil {
			t.Fatal(err)
		}
		var cc []struct {
			Fingerprint string `json:"fingerprint"`
		}
		readJSON(t, p, &cc)
		return cc[0].Fingerprint
	}
	if a, b := fp(`fhir\a.json`), fp("fhir/a.json"); a != b {
		t.Errorf("fingerprint differs by separator: %s vs %s", a, b)
	}
}

// A reformatted file moves line and column, and a validator release may
// reword a message (6.10.3 → 7.0.5 did, #543): neither may change the
// Code Climate fingerprint or the SARIF partial fingerprint.
func TestIdentityStableAcrossLinesAndWording(t *testing.T) {
	ids := func(iss validator.Issue) (string, string) {
		res := []*validator.Result{{Filename: "fhir/a.json", Issues: []validator.Issue{iss}}}
		dir := t.TempDir()
		ccPath, sarifPath := filepath.Join(dir, "cc.json"), filepath.Join(dir, "r.sarif")
		if err := reporter.CodeClimate(res, "information", ccPath); err != nil {
			t.Fatal(err)
		}
		if err := reporter.SARIF(res, "information", reporter.RunInfo{}, sarifPath); err != nil {
			t.Fatal(err)
		}
		var cc []struct {
			Fingerprint string `json:"fingerprint"`
		}
		readJSON(t, ccPath, &cc)
		var sarif struct {
			Runs []struct {
				Results []struct {
					PartialFingerprints map[string]string `json:"partialFingerprints"`
				} `json:"results"`
			} `json:"runs"`
		}
		readJSON(t, sarifPath, &sarif)
		return cc[0].Fingerprint, sarif.Runs[0].Results[0].PartialFingerprints["fhirlintFinding/v1"]
	}
	before := validator.Issue{Severity: "warning", Message: "The version is not stated", Location: "Patient.meta (line 3, col 5)", MessageID: "X_VERSION_NONE"}
	after := before
	after.Message = "No version was stated for this reference"
	after.Location = "Patient.meta (line 17, col 9)"
	cb, sb := ids(before)
	ca, sa := ids(after)
	if cb != ca || sb != sa {
		t.Errorf("identity changed with line and wording: cc %s → %s, sarif %s → %s", cb, ca, sb, sa)
	}
	if sb == "" {
		t.Error("no partialFingerprints in SARIF")
	}
}
