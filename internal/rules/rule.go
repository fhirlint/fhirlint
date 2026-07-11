// Package rules implements custom FHIRPath-based lint rules. A rule asserts a
// boolean FHIRPath expression over every validated resource; when the assertion
// does not hold, the rule produces a validator.Issue that flows through the
// normal reporting, severity-filter, suppression and baseline machinery.
//
// Rule evaluation is delegated to an Evaluator so the FHIRPath engine can be
// chosen (and swapped) independently of the rule model and its wiring.
package rules

import (
	"fmt"
	"regexp"
	"strings"
)

// MessageIDPrefix is prepended to a rule's ID to form the Issue.MessageID
// ("rule:<id>"). It lets users suppress a rule with `--suppress messageId:rule:<id>`
// and keeps rule findings visually distinct from JAR message IDs.
const MessageIDPrefix = "rule:"

// validSeverities are the severities a rule finding may carry, matching the
// FHIR issue severities fhirlint reports (fatal is reserved for the JAR).
var validSeverities = map[string]bool{
	"error":       true,
	"warning":     true,
	"information": true,
}

// idPattern constrains rule IDs to characters that are safe inside a
// "rule:<id>" message ID and in suppress selectors.
var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Rule is a single custom FHIRPath lint rule.
type Rule struct {
	ID       string // stable identifier; forms the "rule:<id>" message ID and the suppression key
	Resource string // optional resourceType filter (empty = applies to every resource)
	Assert   string // FHIRPath expression expected to hold; a false/empty result is a finding
	Message  string // message shown when the assertion fails (defaulted if empty)
	Severity string // "error" | "warning" | "information" (default "error")
	Raw      string // original source for diagnostics
}

// MessageID returns the Issue.MessageID a failure of this rule carries.
func (r Rule) MessageID() string { return MessageIDPrefix + r.ID }

// failureMessage returns the message to report when the rule fails, defaulting
// to a description built from the assertion when no custom message is set.
func (r Rule) failureMessage() string {
	if strings.TrimSpace(r.Message) != "" {
		return r.Message
	}
	return fmt.Sprintf("rule %q failed: %s", r.ID, r.Assert)
}

// normalize fills defaults and trims fields in place.
func (r *Rule) normalize() {
	r.ID = strings.TrimSpace(r.ID)
	r.Resource = strings.TrimSpace(r.Resource)
	r.Assert = strings.TrimSpace(r.Assert)
	r.Message = strings.TrimSpace(r.Message)
	r.Severity = strings.ToLower(strings.TrimSpace(r.Severity))
	if r.Severity == "" {
		r.Severity = "error"
	}
}

// validate reports the first problem with the rule, or nil if it is well-formed.
func (r Rule) validate() error {
	switch {
	case r.ID == "":
		return fmt.Errorf("rule is missing an id")
	case !idPattern.MatchString(r.ID):
		return fmt.Errorf("rule id %q must contain only letters, digits, '.', '_' or '-' and start with a letter or digit", r.ID)
	case r.Assert == "":
		return fmt.Errorf("rule %q is missing an assert expression", r.ID)
	case !validSeverities[r.Severity]:
		return fmt.Errorf("rule %q has invalid severity %q — use error, warning or information", r.ID, r.Severity)
	}
	return nil
}

// ParseMap builds a Rule from the YAML/JSON map form used in fhirlint.yml and
// --rules-file. Viper lowercases config keys, so both camelCase and lowercase
// keys are accepted.
func ParseMap(m map[string]interface{}) (Rule, error) {
	r := Rule{
		ID:       mapString(m, "id"),
		Resource: mapString(m, "resource"),
		Assert:   mapString(m, "assert"),
		Message:  mapString(m, "message"),
		Severity: mapString(m, "severity"),
	}
	r.Raw = fmt.Sprintf("rule:%s", r.ID)
	r.normalize()
	if err := r.validate(); err != nil {
		return Rule{}, err
	}
	return r, nil
}

// Validate checks a parsed rule set for structural errors and duplicate IDs.
// It is used both when loading rules and by `config check`.
func Validate(rules []Rule) error {
	seen := make(map[string]bool, len(rules))
	for i := range rules {
		rules[i].normalize()
		if err := rules[i].validate(); err != nil {
			return err
		}
		if seen[rules[i].ID] {
			return fmt.Errorf("duplicate rule id %q", rules[i].ID)
		}
		seen[rules[i].ID] = true
	}
	return nil
}

// mapString reads key (or its lowercase form) from m as a trimmed string.
func mapString(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok {
		v, ok = m[strings.ToLower(key)]
	}
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", v))
}
