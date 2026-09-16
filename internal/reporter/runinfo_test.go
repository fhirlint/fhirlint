package reporter

import (
	"encoding/json"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/diff"
	"github.com/fhirlint/fhirlint/internal/validator"
)

// Every format that outlives the run says what produced it, once (#428).

const build = "FHIR Validation tool Version 6.10.5 (Git# e9cb40e7b4ea). Built 2026-09-10T00:02:31.157+10:00 (6 days old)"

var info = RunInfo{Fhirlint: "1.13.0", Validator: "6.10.4", FHIRVersion: "4.0.1"}

func TestJSON_MetaSaysWhatProducedTheReport(t *testing.T) {
	r := makeResult(true)
	dest := filepath.Join(t.TempDir(), "r.json")
	if err := JSON([]*validator.Result{r}, "information", info, dest); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest) //nolint:gosec // test temp dir
	if err != nil {
		t.Fatal(err)
	}

	var got struct {
		Meta  *RunInfo `json:"meta"`
		Files []struct {
			ValidatorBuild *string `json:"validatorBuild"`
		} `json:"files"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Meta == nil || *got.Meta != info {
		t.Errorf("meta = %+v, want %+v", got.Meta, info)
	}
	// Once, at the top — not repeated per file.
	if strings.Count(string(data), `"validator"`) != 1 {
		t.Errorf("the validator version should appear exactly once:\n%s", data)
	}
	if strings.Contains(string(data), "validatorBuild") {
		t.Errorf("per-file build line leaked into the JSON:\n%s", data)
	}
}

func TestJSON_NoMetaWhenNothingIsKnown(t *testing.T) {
	report := buildJSONReport([]*validator.Result{makeResult(true)}, "information")
	report.Meta = metaFor(nil, RunInfo{})
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"meta"`) {
		t.Errorf("an empty meta object is noise, got:\n%s", data)
	}
}

func TestRunInfo_ValidatorsOwnStatementIsLiftedFromTheResults(t *testing.T) {
	r := makeResult(true)
	r.ValidatorBuild = build
	got := info.WithResults([]*validator.Result{makeResult(false), r})
	if got.ValidatorBuild != build {
		t.Errorf("ValidatorBuild = %q, want the JAR's line from the results", got.ValidatorBuild)
	}
	if got.Validator != "6.10.4" {
		t.Errorf("the manifest version must stay alongside it, got %q", got.Validator)
	}
	// The JAR's own line wins over the manifest for one-line renderings.
	if got.validatorLine() != build {
		t.Errorf("validatorLine() = %q", got.validatorLine())
	}
	if info.validatorLine() != "6.10.4" {
		t.Errorf("without a build line, validatorLine() = %q, want the version", info.validatorLine())
	}
}

func TestSARIF_ValidatorIsAToolExtension(t *testing.T) {
	r := makeResult(true)
	r.ValidatorBuild = build
	report := buildSARIFReport([]*validator.Result{r}, "information", info)

	tool := report.Runs[0].Tool
	if tool.Driver.Name != "fhirlint" || tool.Driver.Version != "1.13.0" {
		t.Errorf("driver stays fhirlint: %+v", tool.Driver)
	}
	if len(tool.Extensions) != 1 {
		t.Fatalf("extensions = %+v, want exactly the validator", tool.Extensions)
	}
	ext := tool.Extensions[0]
	if ext.Name != "HL7 FHIR Validator" || ext.Version != "6.10.4" || ext.InformationURI != sarifValidatorURI {
		t.Errorf("extension = %+v", ext)
	}
	if ext.Properties["build"] != build || ext.Properties["fhirVersion"] != "4.0.1" {
		t.Errorf("extension properties = %v", ext.Properties)
	}

	// Unknown validator: no half-filled extension.
	report = buildSARIFReport(nil, "information", RunInfo{Fhirlint: "1.13.0"})
	if len(report.Runs[0].Tool.Extensions) != 0 {
		t.Errorf("extensions = %+v, want none when the validator is unknown", report.Runs[0].Tool.Extensions)
	}
}

func TestJUnit_PropertiesOnTheTestsuite(t *testing.T) {
	r := makeResult(true)
	r.ValidatorBuild = build
	suites := buildJUnitReport([]*validator.Result{r}, "information", info)
	out, err := xml.Marshal(suites)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)

	for _, want := range []string{
		`<property name="fhirlint.version" value="1.13.0">`,
		`<property name="validator.version" value="6.10.4">`,
		`<property name="validator.build" value="` + build + `">`,
		`<property name="fhir.version" value="4.0.1">`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s in:\n%s", want, s)
		}
	}
	// On testsuite, where the common schema allows it — not on testsuites.
	if !strings.Contains(s, `<testsuite name="FHIR Validation" tests="1" failures="0"><properties>`) {
		t.Errorf("properties must be the first child of testsuite:\n%s", s)
	}

	if p := buildJUnitReport(nil, "information", RunInfo{}).Suites[0].Properties; p != nil {
		t.Errorf("no provenance, no properties element; got %+v", p)
	}
}

func TestHTML_MetaLine(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "r.html")
	if err := HTML([]*validator.Result{makeResult(true)}, "information", info, dest); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest) //nolint:gosec // test temp dir
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), " · FHIR 4.0.1 · validator 6.10.4 · fhirlint 1.13.0</p>") {
		t.Errorf("meta line missing or misordered:\n%s", data)
	}
}

func TestMarkdown_ProvenanceFooter(t *testing.T) {
	out := buildMarkdownReport([]*validator.Result{makeResult(true)}, "information", info)
	if !strings.HasSuffix(out, "<sub>fhirlint 1.13.0 · validator 6.10.4 · FHIR 4.0.1</sub>\n") {
		t.Errorf("footer missing:\n%s", out)
	}
	out = buildMarkdownReport([]*validator.Result{makeResult(true)}, "information", RunInfo{})
	if strings.Contains(out, "<sub>") {
		t.Errorf("no provenance, no footer:\n%s", out)
	}
}

func TestDiffSides_ValidatorChanged(t *testing.T) {
	same := DiffSides{Baseline: info, Current: info}
	if same.validatorChanged() {
		t.Error("identical sides reported as changed")
	}
	other := info
	other.Validator = "6.10.5"
	if !(DiffSides{Baseline: info, Current: other}).validatorChanged() {
		t.Error("a different validator version not reported")
	}
	// A side that did not say (an older report) is not a difference.
	if (DiffSides{Baseline: RunInfo{}, Current: info}).validatorChanged() {
		t.Error("an unknown side must not count as a change")
	}
}

func TestDiffJSON_CarriesBothSides(t *testing.T) {
	other := info
	other.Validator = "6.10.5"
	dest := filepath.Join(t.TempDir(), "d.json")
	if err := DiffJSON(&diff.Result{}, DiffSides{Baseline: info, Current: other}, dest); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest) //nolint:gosec // test temp dir
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Baseline, Current *RunInfo
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Baseline == nil || got.Current == nil || got.Baseline.Validator != "6.10.4" || got.Current.Validator != "6.10.5" {
		t.Errorf("sides = %+v / %+v", got.Baseline, got.Current)
	}
}
