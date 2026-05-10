package suppress

import (
	"fmt"
	"strings"

	"github.com/fhirlint/fhirlint/internal/validator"
)

// Rule is a single suppression selector.
type Rule struct {
	Type     string // "messageId", "constraint", or "expression"
	Value    string
	Severity string // optional; empty matches any severity
	Raw      string // original string for diagnostics
}

// ParseCLI parses the CLI flag format "type:value".
func ParseCLI(s string) (Rule, error) {
	idx := strings.Index(s, ":")
	if idx < 0 {
		return Rule{}, fmt.Errorf("invalid suppress rule %q: expected type:value (e.g. messageId:dom-6)", s)
	}
	typ, val := s[:idx], s[idx+1:]
	if val == "" {
		return Rule{}, fmt.Errorf("invalid suppress rule %q: value is empty", s)
	}
	if err := validateType(typ); err != nil {
		return Rule{}, err
	}
	return Rule{Type: typ, Value: val, Raw: s}, nil
}

// ParseMap parses the YAML map format: {messageId|constraint|expression: value, severity?: sev}.
func ParseMap(m map[string]interface{}) (Rule, error) {
	r := Rule{}
	for _, typ := range []string{"messageId", "constraint", "expression"} {
		if v, ok := m[typ]; ok {
			r.Type = typ
			r.Value = fmt.Sprintf("%v", v)
			r.Raw = fmt.Sprintf("%s:%s", typ, r.Value)
			break
		}
	}
	if r.Type == "" {
		return Rule{}, fmt.Errorf("suppress rule must have one of: messageId, constraint, expression")
	}
	if sev, ok := m["severity"]; ok {
		r.Severity = fmt.Sprintf("%v", sev)
	}
	return r, nil
}

func validateType(typ string) error {
	switch typ {
	case "messageId", "constraint", "expression":
		return nil
	}
	return fmt.Errorf("unknown suppress type %q: use messageId, constraint, or expression", typ)
}

// Matches returns true when the rule matches the given issue.
func (r Rule) Matches(issue validator.Issue) bool {
	if r.Severity != "" && r.Severity != issue.Severity {
		return false
	}
	switch r.Type {
	case "messageId", "constraint":
		return issue.MessageID == r.Value
	case "expression":
		loc := issue.Location
		// strip " (line X, col Y)" suffix before comparing
		if i := strings.Index(loc, " (line "); i >= 0 {
			loc = loc[:i]
		}
		return loc == r.Value || strings.HasPrefix(loc, r.Value+".")
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
