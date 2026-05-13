package suppress

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/fhirlint/fhirlint/internal/validator"
)

// Rule is a single suppression selector.
type Rule struct {
	Type     string // "messageId", "constraint", "expression", or "pattern"
	Value    string
	Regexp   *regexp.Regexp // compiled pattern; non-nil only when Type == "pattern"
	Severity string         // optional; empty matches any severity
	Reason   string         // optional; shown in --show-suppressed output for audit trail
	Raw      string         // original string for diagnostics
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
		return Rule{}, fmt.Errorf("suppress rule must have one of: messageId, constraint, expression, pattern")
	}
	if r.Type == "pattern" {
		re, err := regexp.Compile(r.Value)
		if err != nil {
			return Rule{}, fmt.Errorf("invalid suppress pattern %q: %w", r.Value, err)
		}
		r.Regexp = re
	}
	if sev, ok := m["severity"]; ok {
		r.Severity = fmt.Sprintf("%v", sev)
	}
	if reason, ok := m["reason"]; ok {
		r.Reason = strings.TrimSpace(fmt.Sprintf("%v", reason))
	}
	return r, nil
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

// Apply partitions each result's Issues into active (Issues) and Suppressed slices.
// It recomputes Result.Valid based on active issues only.
// The returned slice contains the match count per rule — zero means the rule was unused.
func Apply(results []*validator.Result, rules []Rule) []int {
	matchCount := make([]int, len(rules))
	for _, r := range results {
		var active, suppressed []validator.Issue
		for _, issue := range r.Issues {
			matched := false
			for i, rule := range rules {
				if rule.Matches(issue) {
					issue.SuppressReason = rule.Reason
					suppressed = append(suppressed, issue)
					matchCount[i]++
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
	return matchCount
}
