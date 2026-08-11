package suppress

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/fhirlint/fhirlint/internal/validator"
	"time"
)

// Rule is a single suppression selector.
type Rule struct {
	Type     string // "messageId", "constraint", "expression", or "pattern"
	Value    string
	Regexp   *regexp.Regexp // compiled pattern; non-nil only when Type == "pattern"
	Severity string         // optional; empty matches any severity
	Reason   string         // optional; shown in --show-suppressed output for audit trail
	Expires  time.Time      // optional; zero means the rule never expires (config only)
	Raw      string         // original string for diagnostics
}

// expiryWarnWindow is how far ahead a pending expiry is announced, so a rule
// lapsing does not arrive as a surprise on the day it happens.
const expiryWarnWindow = 14 * 24 * time.Hour

// expiryLayout is the only accepted form for `expires`. Deliberately strict: a
// half-understood date would silently change when a suppression stops working.
const expiryLayout = "2006-01-02"

// ExpiredAt reports whether the rule's expiry date has passed at now.
// The date is inclusive — a rule with expires: 2026-12-31 still suppresses
// throughout that day and lapses at the start of the next.
func (r Rule) ExpiredAt(now time.Time) bool {
	if r.Expires.IsZero() {
		return false
	}
	return !now.Before(r.Expires.AddDate(0, 0, 1))
}

// ExpiresSoonAt reports whether the rule expires within expiryWarnWindow and
// has not lapsed yet.
func (r Rule) ExpiresSoonAt(now time.Time) bool {
	if r.Expires.IsZero() || r.ExpiredAt(now) {
		return false
	}
	return r.Expires.AddDate(0, 0, 1).Sub(now) <= expiryWarnWindow
}

// ParseCLI parses the CLI flag format "type:value" or "type:value|reason".
// The optional "|reason" suffix is split on the first pipe character.
// For pattern rules whose regex contains "|", use the YAML config format instead.
func ParseCLI(s string) (Rule, error) {
	idx := strings.Index(s, ":")
	if idx < 0 {
		return Rule{}, fmt.Errorf("invalid suppress rule %q: expected type:value (e.g. messageId:dom-6)", s)
	}
	typ, rest := s[:idx], s[idx+1:]
	val, reason, _ := strings.Cut(rest, "|")
	if val == "" {
		return Rule{}, fmt.Errorf("invalid suppress rule %q: value is empty", s)
	}
	if err := validateType(typ); err != nil {
		return Rule{}, err
	}
	r := Rule{Type: typ, Value: val, Reason: strings.TrimSpace(reason), Raw: s}
	if typ == "pattern" {
		re, err := regexp.Compile(val)
		if err != nil {
			return Rule{}, fmt.Errorf("invalid suppress rule %q: invalid regex: %w", s, err)
		}
		r.Regexp = re
	}
	return r, nil
}

// ParseMap parses the YAML map format: {messageId|constraint|expression|pattern: value, severity?: sev}.
// Viper lowercases all config keys, so we accept both camelCase ("messageId") and lowercase ("messageid").
func ParseMap(m map[string]interface{}) (Rule, error) {
	r, err := parseSelectorMap(m, "suppress")
	if err != nil {
		return Rule{}, err
	}
	// Here `severity` narrows what the rule matches. Severity overrides spell the
	// same key differently — there it is the level to apply — which is why they
	// parse through parseSelectorMap rather than through this function.
	if sev, ok := m["severity"]; ok {
		r.Severity = fmt.Sprintf("%v", sev)
	}
	return r, nil
}

// parseSelectorMap reads the parts every selector-based rule shares: the
// selector itself, an optional reason and an optional expiry. kind names the
// rule type in error messages ("suppress", "severity-override").
func parseSelectorMap(m map[string]interface{}, kind string) (Rule, error) {
	r := Rule{}
	for _, typ := range []string{"messageId", "constraint", "expression", "pattern"} {
		v, ok := m[typ]
		if !ok {
			v, ok = m[strings.ToLower(typ)] // viper normalises keys to lowercase
		}
		if ok {
			r.Type = typ
			r.Value = fmt.Sprintf("%v", v)
			r.Raw = fmt.Sprintf("%s:%s", typ, r.Value)
			break
		}
	}
	if r.Type == "" {
		return Rule{}, fmt.Errorf("%s rule must have one of: messageId, constraint, expression, pattern", kind)
	}
	if r.Type == "pattern" {
		re, err := regexp.Compile(r.Value)
		if err != nil {
			return Rule{}, fmt.Errorf("invalid %s pattern %q: %w", kind, r.Value, err)
		}
		r.Regexp = re
	}
	if reason, ok := m["reason"]; ok {
		r.Reason = strings.TrimSpace(fmt.Sprintf("%v", reason))
	}
	if exp, ok := m["expires"]; ok {
		parsed, err := ParseExpiry(exp)
		if err != nil {
			return Rule{}, err
		}
		r.Expires = parsed
	}
	return r, nil
}

// ParseExpiry accepts YYYY-MM-DD, either as a string or as the time.Time a YAML
// parser produces for an unquoted date. Anything else is an error rather than a
// silent "no expiry", which would turn a typo into a permanent suppression.
//
// Exported so `fhirlint config check` validates expiry dates by exactly the
// rule that parsing applies, instead of keeping a second copy that can drift.
func ParseExpiry(v interface{}) (time.Time, error) {
	switch t := v.(type) {
	case time.Time:
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), nil
	case string:
		parsed, err := time.Parse(expiryLayout, strings.TrimSpace(t))
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid suppress expires %q: use YYYY-MM-DD", t)
		}
		return parsed, nil
	default:
		return time.Time{}, fmt.Errorf("invalid suppress expires %v: use YYYY-MM-DD", v)
	}
}

func validateType(typ string) error {
	switch typ {
	case "messageId", "constraint", "expression", "pattern":
		return nil
	}
	return fmt.Errorf("unknown suppress type %q: use messageId, constraint, expression, or pattern", typ)
}

// Matches returns true when the rule matches the given issue.
func (r Rule) Matches(issue validator.Issue) bool {
	if r.Severity != "" && r.Severity != issue.Severity {
		return false
	}
	switch r.Type {
	case "messageId":
		return issue.MessageID == r.Value
	case "constraint":
		// Match the short constraint key (e.g. "dom-6") against a full URI messageId
		// like "http://hl7.org/fhir/StructureDefinition/DomainResource#dom-6".
		return issue.MessageID == r.Value || strings.HasSuffix(issue.MessageID, "#"+r.Value)
	case "expression":
		loc := issue.Location
		// strip " (line X, col Y)" suffix before comparing
		if i := strings.Index(loc, " (line "); i >= 0 {
			loc = loc[:i]
		}
		return loc == r.Value || strings.HasPrefix(loc, r.Value+".")
	case "pattern":
		return r.Regexp != nil && r.Regexp.MatchString(issue.Message)
	}
	return false
}

// Outcome reports what happened to one rule during Apply, so the caller can
// tell an unused rule apart from one that has lapsed.
type Outcome struct {
	Matches     int  // issues this rule suppressed; zero means unused
	Expired     bool // expiry date has passed — the rule suppressed nothing
	ExpiresSoon bool // lapses within the warning window
}

// Apply partitions each result's Issues into active (Issues) and Suppressed
// slices, using the current time to decide which rules have expired.
func Apply(results []*validator.Result, rules []Rule) []Outcome {
	return ApplyAt(results, rules, time.Now())
}

// ApplyAt is Apply with an explicit clock, so expiry behaviour is testable.
//
// An expired rule stops suppressing: its findings come back and can fail the
// build. That is the point — a suppression that keeps working past its own
// deadline is the problem expiry dates exist to prevent. The caller is expected
// to report Expired rules so the returning findings are explained rather than
// mysterious.
func ApplyAt(results []*validator.Result, rules []Rule, now time.Time) []Outcome {
	outcomes := make([]Outcome, len(rules))
	for i, rule := range rules {
		outcomes[i].Expired = rule.ExpiredAt(now)
		outcomes[i].ExpiresSoon = rule.ExpiresSoonAt(now)
	}
	for _, r := range results {
		var active, suppressed []validator.Issue
		for _, issue := range r.Issues {
			matched := false
			for i, rule := range rules {
				if outcomes[i].Expired {
					continue
				}
				if rule.Matches(issue) {
					issue.SuppressReason = rule.Reason
					suppressed = append(suppressed, issue)
					outcomes[i].Matches++
					matched = true
					break
				}
			}
			if !matched {
				active = append(active, issue)
			}
		}
		r.Issues = active
		r.Suppressed = suppressed
		r.Valid = true
		for _, issue := range active {
			if issue.Severity == "error" || issue.Severity == "fatal" {
				r.Valid = false
				break
			}
		}
	}
	return outcomes
}

// WithoutReason returns the rules that carry no reason. It backs the
// require-suppress-reason policy, which exists so a suppression cannot silence
// a finding without recording why — the thing anyone auditing the config later
// actually needs.
func WithoutReason(rules []Rule) []Rule {
	var out []Rule
	for _, r := range rules {
		if strings.TrimSpace(r.Reason) == "" {
			out = append(out, r)
		}
	}
	return out
}
