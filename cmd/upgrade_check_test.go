// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/reporter"
	"github.com/fhirlint/fhirlint/internal/validator"
)

func TestParseUpgradeCheckArgs(t *testing.T) {
	a, err := parseUpgradeCheckArgs([]string{"--to", "7.0.3", "./fhir/", "--ig", "kbv.basis#1.10.0", "--from=6.10.3", "--json", "--tx-offline"})
	if err != nil {
		t.Fatal(err)
	}
	if a.to != "7.0.3" || a.from != "6.10.3" || !a.json {
		t.Errorf("parsed %+v", a)
	}
	if strings.Join(a.passthrough, " ") != "./fhir/ --ig kbv.basis#1.10.0 --tx-offline" {
		t.Errorf("passthrough = %q", a.passthrough)
	}
	if !usesRecordedTerminology(a.passthrough) {
		t.Error("--tx-offline not seen")
	}

	for _, bad := range [][]string{
		{"./fhir/"},
		{"--to"},
		{"--to", "7.0.3", "--format", "sarif"},
		{"--to", "7.0.3", "-o", "x.json"},
		{"--to", "7.0.3", "--jar=x.jar"},
	} {
		if _, err := parseUpgradeCheckArgs(bad); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
}

func report(version string, issues ...validator.Issue) *reporter.JSONReport {
	return &reporter.JSONReport{
		Meta:  &reporter.RunInfo{Validator: version},
		Files: []*validator.Result{{Filename: "in/Patient.json", Issues: issues}},
	}
}

// The case from the 2026-10-29 review in #525: 6.10.3 → 7.0.3 renamed
// MSG_DRAFT, and added warnings.
func TestCompareUpgrade(t *testing.T) {
	from := report("6.10.3",
		validator.Issue{Severity: "information", MessageID: "MSG_DRAFT", Location: "Patient.meta.profile[0] (line 3, col 5)",
			Message: "Reference to draft profile http://x/SD/p|1.0"},
		validator.Issue{Severity: "warning", MessageID: "dom-6", Location: "Patient", Message: "Constraint failed: dom-6"},
		validator.Issue{Severity: "error", MessageID: "OLD_ERR", Location: "Patient.gender", Message: "gone in 7"},
	)
	to := report("7.0.3",
		validator.Issue{Severity: "information", MessageID: "MSG_DRAFT_SRC_STATUS", Location: "Patient.meta.profile[0] (line 3, col 5)",
			Message: "Reference to draft profile http://x/SD/p|1.0 (status draft)"},
		validator.Issue{Severity: "warning", MessageID: "dom-6", Location: "Patient", Message: "Constraint failed: dom-6"},
		validator.Issue{Severity: "error", MessageID: "UNKNOWN_CODESYSTEM_VERSION", Location: "Patient.identifier[0].type",
			Message: "A definition for CodeSystem x version 2026 could not be found"},
	)
	r := compareUpgrade(from, to)

	if len(r.Renamed) != 1 || r.Renamed[0].From.MessageID != "MSG_DRAFT" || r.Renamed[0].To.MessageID != "MSG_DRAFT_SRC_STATUS" {
		t.Errorf("renamed = %+v", r.Renamed)
	}
	if len(r.New) != 1 || r.New[0].MessageID != "UNKNOWN_CODESYSTEM_VERSION" || r.NewErrors != 1 {
		t.Errorf("new = %+v, newErrors = %d", r.New, r.NewErrors)
	}
	if len(r.Resolved) != 1 || r.Resolved[0].MessageID != "OLD_ERR" {
		t.Errorf("resolved = %+v", r.Resolved)
	}
	if len(r.Files) != 1 || r.Files[0] != (upgradeFile{File: "in/Patient.json", New: 1, Resolved: 1, Renamed: 1}) {
		t.Errorf("files = %+v", r.Files)
	}
	// dom-6 is unchanged, so it is not listed.
	for _, c := range r.ByID {
		if c.MessageID == "dom-6" {
			t.Error("unchanged id listed")
		}
	}
	if len(r.ByID) != 4 {
		t.Errorf("byId = %+v", r.ByID)
	}

	var out strings.Builder
	printUpgradeReport(&out, r)
	for _, want := range []string{"from: 6.10.3", "to:   7.0.3", "MSG_DRAFT → MSG_DRAFT_SRC_STATUS", "1 new error(s)"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report misses %q:\n%s", want, out.String())
		}
	}
}

// Different text at the same place is a new finding, not a rename.
func TestPairRenames_NeedsSimilarText(t *testing.T) {
	r := compareUpgrade(
		report("a", validator.Issue{Severity: "error", MessageID: "A", Location: "Patient.x", Message: "the value is not a valid date"}),
		report("b", validator.Issue{Severity: "error", MessageID: "B", Location: "Patient.x", Message: "unknown code system referenced here"}),
	)
	if len(r.Renamed) != 0 || len(r.New) != 1 || len(r.Resolved) != 1 {
		t.Errorf("renamed %v new %v resolved %v", r.Renamed, r.New, r.Resolved)
	}
}

func TestCompareUpgrade_NoChange(t *testing.T) {
	iss := validator.Issue{Severity: "warning", MessageID: "dom-6", Location: "Patient"}
	r := compareUpgrade(report("a", iss), report("b", iss))
	var out strings.Builder
	printUpgradeReport(&out, r)
	if r.NewErrors != 0 || !strings.Contains(out.String(), "No change") {
		t.Errorf("%+v\n%s", r, out.String())
	}
}
