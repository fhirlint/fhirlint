package suppress

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/fhirlint/fhirlint/internal/validator"
)

// SeverityRule re-levels the findings it matches instead of hiding them.
//
// It exists for the finding you do not want failing the build and do not want
// to lose sight of either — typically an error caused by a defect in someone
// else's published profile, which will not be fixed on your schedule (#311).
// Suppressing it works but drops it silently, so nobody notices when the
// profile is eventually corrected.
//
// The selector machinery is the suppression one, embedded rather than copied.
// Note that the embedded Rule.Severity is a *match filter* and stays empty for
// rules coming from config: under `severity-override:` the `severity` key names
// the level to apply, which is To.
type SeverityRule struct {
	Rule
	To string // target severity: fatal, error, warning or information
}

// severityLevels are the severities a rule may target. Anything else is
// rejected at parse time rather than silently producing a level no reporter,
// filter or --fail-on threshold knows about.
var severityLevels = map[string]struct{}{
	"fatal": {}, "error": {}, "warning": {}, "information": {},
}

// ParseSeverityMap parses one entry of the `severity-override:` list:
// {messageId|constraint|expression|pattern: value, severity: level, reason?, expires?}.
//
// Unlike a suppression there is no string shorthand — a bare "messageId:x"
// carries no target level, so the map form is the only one that can express
// the rule at all.
func ParseSeverityMap(m map[string]interface{}) (SeverityRule, error) {
	r, err := parseSelectorMap(m, "severity-override")
	if err != nil {
		return SeverityRule{}, err
	}
	sev, ok := m["severity"]
	if !ok {
		return SeverityRule{}, fmt.Errorf(
			"severity-override rule %s must have a severity: the level to apply", r.Raw)
	}
	to := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", sev)))
	if _, valid := severityLevels[to]; !valid {
		return SeverityRule{}, fmt.Errorf(
			"invalid severity %q in severity-override rule %s (allowed: %s)",
			sev, r.Raw, strings.Join(SeverityLevels(), ", "))
	}
	return SeverityRule{Rule: r, To: to}, nil
}

// SeverityLevels returns the accepted target severities, sorted, so error
// messages and `config check` list exactly the same set.
func SeverityLevels() []string {
	out := make([]string, 0, len(severityLevels))
	for s := range severityLevels {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// ApplySeverity re-levels matching issues in every result, using the current
// time to decide which rules have expired.
func ApplySeverity(results []*validator.Result, rules []SeverityRule) []Outcome {
	return ApplySeverityAt(results, rules, time.Now())
}

// ApplySeverityAt is ApplySeverity with an explicit clock, so expiry behaviour
// is testable.
//
// The new level is written into Issue.Severity and the reported one is kept in
// Issue.OriginalSeverity. Everything downstream — the severity filter,
// --fail-on, --max-warnings, the exit code and every reporter — reads Severity,
// so a re-levelled finding is treated as its new level throughout rather than
// only where it is displayed.
//
// The first matching rule wins, as with suppression. An expired rule re-levels
// nothing: the finding goes back to what the validator said, which may well
// fail the build, and that is the point of writing a date.
func ApplySeverityAt(results []*validator.Result, rules []SeverityRule, now time.Time) []Outcome {
	outcomes := make([]Outcome, len(rules))
	for i, rule := range rules {
		outcomes[i].Expired = rule.ExpiredAt(now)
		outcomes[i].ExpiresSoon = rule.ExpiresSoonAt(now)
	}
	for _, r := range results {
		for j := range r.Issues {
			for i, rule := range rules {
				if outcomes[i].Expired || !rule.Matches(r.Issues[j]) {
					continue
				}
				outcomes[i].Matches++
				// A rule that names the level a finding already has is not an
				// error, but it has changed nothing, so nothing is recorded as
				// having been changed.
				if r.Issues[j].Severity != rule.To {
					r.Issues[j].OriginalSeverity = r.Issues[j].Severity
					r.Issues[j].Severity = rule.To
				}
				break
			}
		}
		r.Valid = true
		for _, issue := range r.Issues {
			if issue.Severity == "error" || issue.Severity == "fatal" {
				r.Valid = false
				break
			}
		}
	}
	return outcomes
}

// Selectors returns the embedded selector rules, so the checks written against
// suppression rules — require-suppress-reason in particular — apply to
// severity overrides without a second implementation.
func Selectors(rules []SeverityRule) []Rule {
	out := make([]Rule, len(rules))
	for i, r := range rules {
		out[i] = r.Rule
	}
	return out
}
