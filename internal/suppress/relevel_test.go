package suppress

import (
	"testing"
	"time"

	"github.com/fhirlint/fhirlint/internal/validator"
)

func severityRule(t *testing.T, m map[string]interface{}) SeverityRule {
	t.Helper()
	r, err := ParseSeverityMap(m)
	if err != nil {
		t.Fatalf("ParseSeverityMap(%v): %v", m, err)
	}
	return r
}

func TestParseSeverityMap(t *testing.T) {
	r := severityRule(t, map[string]interface{}{
		"messageId": "Rule_bdl_1",
		"severity":  "warning",
		"reason":    "upstream profile defect",
		"expires":   "2026-12-31",
	})
	if r.Type != "messageId" || r.Value != "Rule_bdl_1" {
		t.Errorf("selector = %s:%s, want messageId:Rule_bdl_1", r.Type, r.Value)
	}
	if r.To != "warning" {
		t.Errorf("To = %q, want warning", r.To)
	}
	if r.Reason != "upstream profile defect" {
		t.Errorf("Reason = %q", r.Reason)
	}
	if r.Expires.Format("2006-01-02") != "2026-12-31" {
		t.Errorf("Expires = %v", r.Expires)
	}
	// The embedded match filter stays empty: `severity` was the target level.
	if r.Severity != "" {
		t.Errorf("Severity match filter = %q, want empty", r.Severity)
	}
}

func TestParseSeverityMapRejects(t *testing.T) {
	cases := map[string]map[string]interface{}{
		"no selector":      {"severity": "warning"},
		"no severity":      {"messageId": "x"},
		"unknown severity": {"messageId": "x", "severity": "critical"},
		"empty severity":   {"messageId": "x", "severity": ""},
		"bad expiry":       {"messageId": "x", "severity": "warning", "expires": "31.12.2026"},
		"bad pattern":      {"pattern": "([", "severity": "warning"},
	}
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseSeverityMap(m); err == nil {
				t.Fatalf("ParseSeverityMap(%v) = nil error, want one", m)
			}
		})
	}
}

func TestParseSeverityMapNormalisesCase(t *testing.T) {
	r := severityRule(t, map[string]interface{}{"messageId": "x", "severity": " Warning "})
	if r.To != "warning" {
		t.Errorf("To = %q, want warning", r.To)
	}
}

func TestApplySeverityDowngradeMakesResultValid(t *testing.T) {
	res := []*validator.Result{{
		Filename: "a.json",
		Valid:    false,
		Issues: []validator.Issue{
			{Severity: "error", MessageID: "Rule_bdl_1", Message: "fullUrl missing"},
			{Severity: "warning", MessageID: "other", Message: "something else"},
		},
	}}
	rules := []SeverityRule{severityRule(t, map[string]interface{}{
		"messageId": "Rule_bdl_1", "severity": "warning",
	})}

	outcomes := ApplySeverity(res, rules)

	if got := res[0].Issues[0].Severity; got != "warning" {
		t.Errorf("severity = %q, want warning", got)
	}
	if got := res[0].Issues[0].OriginalSeverity; got != "error" {
		t.Errorf("originalSeverity = %q, want error", got)
	}
	if !res[0].Valid {
		t.Error("result should be valid once the only error is downgraded")
	}
	if res[0].Issues[1].OriginalSeverity != "" {
		t.Error("unmatched issue should not be marked as re-levelled")
	}
	if outcomes[0].Matches != 1 {
		t.Errorf("Matches = %d, want 1", outcomes[0].Matches)
	}
}

func TestApplySeverityUpgradeInvalidatesResult(t *testing.T) {
	res := []*validator.Result{{
		Filename: "a.json",
		Valid:    true,
		Issues:   []validator.Issue{{Severity: "warning", MessageID: "ext", Message: "Unknown extension foo"}},
	}}
	rules := []SeverityRule{severityRule(t, map[string]interface{}{
		"pattern": "Unknown extension.*", "severity": "error",
	})}

	ApplySeverity(res, rules)

	if res[0].Issues[0].Severity != "error" {
		t.Errorf("severity = %q, want error", res[0].Issues[0].Severity)
	}
	if res[0].Valid {
		t.Error("result should be invalid once a finding is upgraded to error")
	}
}

func TestApplySeverityFirstRuleWins(t *testing.T) {
	res := []*validator.Result{{
		Issues: []validator.Issue{{Severity: "error", MessageID: "dom-6"}},
	}}
	rules := []SeverityRule{
		severityRule(t, map[string]interface{}{"constraint": "dom-6", "severity": "warning"}),
		severityRule(t, map[string]interface{}{"constraint": "dom-6", "severity": "information"}),
	}

	outcomes := ApplySeverity(res, rules)

	if got := res[0].Issues[0].Severity; got != "warning" {
		t.Errorf("severity = %q, want warning (first rule wins)", got)
	}
	if outcomes[1].Matches != 0 {
		t.Errorf("second rule Matches = %d, want 0", outcomes[1].Matches)
	}
}

func TestApplySeverityNoOpRuleCountsAsMatch(t *testing.T) {
	res := []*validator.Result{{
		Issues: []validator.Issue{{Severity: "warning", MessageID: "x"}},
	}}
	rules := []SeverityRule{severityRule(t, map[string]interface{}{"messageId": "x", "severity": "warning"})}

	outcomes := ApplySeverity(res, rules)

	// It matched, so "matched 0 issues" would be a lie — but nothing changed, so
	// the issue must not claim it was re-levelled.
	if outcomes[0].Matches != 1 {
		t.Errorf("Matches = %d, want 1", outcomes[0].Matches)
	}
	if res[0].Issues[0].OriginalSeverity != "" {
		t.Errorf("originalSeverity = %q, want empty", res[0].Issues[0].OriginalSeverity)
	}
}

func TestApplySeverityExpiredRuleDoesNothing(t *testing.T) {
	res := []*validator.Result{{
		Valid:  false,
		Issues: []validator.Issue{{Severity: "error", MessageID: "Rule_bdl_1"}},
	}}
	rules := []SeverityRule{severityRule(t, map[string]interface{}{
		"messageId": "Rule_bdl_1", "severity": "warning", "expires": "2026-01-01",
	})}

	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	outcomes := ApplySeverityAt(res, rules, now)

	if res[0].Issues[0].Severity != "error" {
		t.Errorf("severity = %q, want error — an expired rule re-levels nothing", res[0].Issues[0].Severity)
	}
	if res[0].Valid {
		t.Error("result should stay invalid when the rule has lapsed")
	}
	if !outcomes[0].Expired {
		t.Error("outcome should report the rule as expired")
	}
}

func TestApplySeverityExpiryIsInclusive(t *testing.T) {
	rules := []SeverityRule{severityRule(t, map[string]interface{}{
		"messageId": "x", "severity": "warning", "expires": "2026-12-31",
	})}

	onLastDay := []*validator.Result{{Issues: []validator.Issue{{Severity: "error", MessageID: "x"}}}}
	ApplySeverityAt(onLastDay, rules, time.Date(2026, 12, 31, 23, 0, 0, 0, time.UTC))
	if onLastDay[0].Issues[0].Severity != "warning" {
		t.Error("rule should still apply throughout its expiry date")
	}

	dayAfter := []*validator.Result{{Issues: []validator.Issue{{Severity: "error", MessageID: "x"}}}}
	ApplySeverityAt(dayAfter, rules, time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	if dayAfter[0].Issues[0].Severity != "error" {
		t.Error("rule should have lapsed the day after its expiry date")
	}
}

func TestApplySeverityExpiresSoon(t *testing.T) {
	rules := []SeverityRule{severityRule(t, map[string]interface{}{
		"messageId": "x", "severity": "warning", "expires": "2026-12-31",
	})}
	res := []*validator.Result{{Issues: []validator.Issue{{Severity: "error", MessageID: "x"}}}}

	outcomes := ApplySeverityAt(res, rules, time.Date(2026, 12, 24, 0, 0, 0, 0, time.UTC))

	if !outcomes[0].ExpiresSoon {
		t.Error("a rule lapsing in a week should be flagged as expiring soon")
	}
	if outcomes[0].Expired {
		t.Error("a rule lapsing in a week has not expired yet")
	}
}

func TestSelectors(t *testing.T) {
	rules := []SeverityRule{
		severityRule(t, map[string]interface{}{"messageId": "x", "severity": "warning", "reason": "why"}),
		severityRule(t, map[string]interface{}{"constraint": "dom-6", "severity": "error"}),
	}

	sel := Selectors(rules)

	if len(sel) != 2 {
		t.Fatalf("len = %d, want 2", len(sel))
	}
	if missing := WithoutReason(sel); len(missing) != 1 || missing[0].Value != "dom-6" {
		t.Errorf("WithoutReason = %v, want just the dom-6 rule", missing)
	}
}

func TestSeverityLevels(t *testing.T) {
	got := SeverityLevels()
	want := []string{"error", "fatal", "information", "warning"}
	if len(got) != len(want) {
		t.Fatalf("SeverityLevels() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("SeverityLevels()[%d] = %q, want %q (sorted)", i, got[i], want[i])
		}
	}
}
