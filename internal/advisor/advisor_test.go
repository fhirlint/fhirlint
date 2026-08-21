package advisor_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fhirlint/fhirlint/internal/advisor"
	"github.com/fhirlint/fhirlint/internal/suppress"
)

func mustParse(t *testing.T, s string) suppress.Rule {
	t.Helper()
	r, err := suppress.ParseCLI(s)
	if err != nil {
		t.Fatalf("parsing %q: %v", s, err)
	}
	return r
}

func TestConvert_MessageID(t *testing.T) {
	f, dropped := advisor.Convert([]suppress.Rule{mustParse(t, "messageId:Type_Specific_Checks_DT_URL_Resolve")})

	want := []string{"Type_Specific_Checks_DT_URL_Resolve"}
	if !slices.Equal(f.Suppress, want) {
		t.Errorf("Suppress = %q, want %q", f.Suppress, want)
	}
	if len(dropped) != 0 {
		t.Errorf("dropped = %+v, want none", dropped)
	}
}

// An expression rule matches the location and everything below it, which takes
// two advisor entries: a path matches only its own depth unless its last
// segment is `*`.
func TestConvert_ExpressionCoversElementAndDescendants(t *testing.T) {
	f, dropped := advisor.Convert([]suppress.Rule{mustParse(t, "expression:Patient.name")})

	want := []string{"*@Patient.name", "*@Patient.name.*"}
	if !slices.Equal(f.Suppress, want) {
		t.Errorf("Suppress = %q, want %q", f.Suppress, want)
	}
	if len(dropped) != 0 {
		t.Errorf("dropped = %+v, want none", dropped)
	}
}

func TestConvert_UntranslatableRules(t *testing.T) {
	cases := []struct {
		name   string
		rule   string
		reason string
	}{
		{"constraint", "constraint:dom-6", advisor.ReasonConstraint},
		{"pattern", `pattern:.*example\.org.*`, advisor.ReasonPattern},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, dropped := advisor.Convert([]suppress.Rule{mustParse(t, tc.rule)})

			if len(f.Suppress) != 0 {
				t.Errorf("Suppress = %q, want nothing exported", f.Suppress)
			}
			if len(dropped) != 1 || dropped[0].Reason != tc.reason {
				t.Fatalf("dropped = %+v, want one entry with reason %q", dropped, tc.reason)
			}
		})
	}
}

// A severity filter narrows any rule type. Exporting the selector without it
// would suppress more than the project asked for, so the rule is dropped
// whatever its type.
func TestConvert_SeverityFilterIsNotSilentlyLost(t *testing.T) {
	rule := mustParse(t, "messageId:SOME_ID")
	rule.Severity = "warning"

	f, dropped := advisor.Convert([]suppress.Rule{rule})

	if len(f.Suppress) != 0 {
		t.Errorf("Suppress = %q, want nothing exported", f.Suppress)
	}
	if len(dropped) != 1 || dropped[0].Reason != advisor.ReasonSeverityFilter {
		t.Errorf("dropped = %+v, want the severity-filter reason", dropped)
	}
}

// The advisor format has no expiry, so exporting a lapsed rule would bring it
// back to life in the file it is exported into.
func TestConvertAt_ExpiredRuleIsNotExported(t *testing.T) {
	now := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)

	expired := mustParse(t, "messageId:OLD_ID")
	expired.Expires = now.AddDate(0, 0, -2)
	live := mustParse(t, "messageId:LIVE_ID")
	live.Expires = now.AddDate(0, 0, 2)

	f, dropped := advisor.ConvertAt([]suppress.Rule{expired, live}, now)

	if !slices.Equal(f.Suppress, []string{"LIVE_ID"}) {
		t.Errorf("Suppress = %q, want only the unexpired rule", f.Suppress)
	}
	if len(dropped) != 1 || dropped[0].Reason != advisor.ReasonExpired {
		t.Errorf("dropped = %+v, want the expired rule reported", dropped)
	}
}

// Two rules covering the same ground must not produce the same entry twice —
// the file is checked in and read in diffs.
func TestConvert_DeduplicatesEntries(t *testing.T) {
	f, _ := advisor.Convert([]suppress.Rule{
		mustParse(t, "messageId:SAME"),
		mustParse(t, "messageId:SAME"),
		mustParse(t, "expression:Patient.name"),
		mustParse(t, "expression:Patient.name"),
	})

	want := []string{"SAME", "*@Patient.name", "*@Patient.name.*"}
	if !slices.Equal(f.Suppress, want) {
		t.Errorf("Suppress = %q, want %q", f.Suppress, want)
	}
}

// Rule order is config order: the file should read like the config it came
// from, not like an arbitrary map walk.
func TestConvert_PreservesRuleOrder(t *testing.T) {
	f, _ := advisor.Convert([]suppress.Rule{
		mustParse(t, "messageId:B"),
		mustParse(t, "messageId:A"),
		mustParse(t, "messageId:C"),
	})
	if !slices.Equal(f.Suppress, []string{"B", "A", "C"}) {
		t.Errorf("Suppress = %q, want config order preserved", f.Suppress)
	}
}

// An empty rule set is a valid advisor file that suppresses nothing. It must
// not marshal to `"suppress": null`, which is a different document.
func TestMarshal_EmptyIsAnEmptyArray(t *testing.T) {
	f, _ := advisor.Convert(nil)

	data, err := advisor.Marshal(f)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), "null") {
		t.Errorf("marshalled to %s, want an empty array", data)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Error("advisor file does not end in a newline")
	}

	// Round-trips as the shape the validator reads: one "suppress" key.
	var back struct {
		Suppress []string `json:"suppress"`
	}
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshalling own output: %v", err)
	}
	if back.Suppress == nil || len(back.Suppress) != 0 {
		t.Errorf("round-tripped suppress = %#v, want an empty non-nil list", back.Suppress)
	}
}
