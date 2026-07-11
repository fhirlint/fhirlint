// Package lint implements built-in style/convention checks for FHIR resources —
// naming conventions and URL patterns that profile validation does not enforce.
// Each rule is opt-in via the lint: config key, carries its own severity, and
// produces findings with the message ID "lint:<rule>" so they flow through the
// normal reporters, severity filter, suppression and baseline machinery.
package lint

import (
	"fmt"
	"sort"
	"strings"

	"github.com/fhirlint/fhirlint/internal/validator"
	"github.com/tidwall/gjson"
)

// MessageIDPrefix is prepended to a rule name to form Issue.MessageID.
const MessageIDPrefix = "lint:"

var validSeverities = map[string]bool{"error": true, "warning": true, "information": true}

// RuleMeta describes a built-in rule for tooling (config check, docs).
type RuleMeta struct {
	Name           string
	Description    string
	DefaultSev     string
	RequiredParams []string
}

// Rules returns metadata for every built-in rule, sorted by name.
func Rules() []RuleMeta {
	out := make([]RuleMeta, 0, len(registry))
	for _, r := range registry {
		out = append(out, RuleMeta{
			Name:           r.name,
			Description:    r.description,
			DefaultSev:     r.defaultSev,
			RequiredParams: r.requiredParams,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Known reports whether name is a built-in rule and returns its metadata.
func Known(name string) (RuleMeta, bool) {
	r, ok := registry[name]
	if !ok {
		return RuleMeta{}, false
	}
	return RuleMeta{Name: r.name, Description: r.description, DefaultSev: r.defaultSev, RequiredParams: r.requiredParams}, true
}

// RuleSetting is the resolved configuration for one enabled rule.
type RuleSetting struct {
	Severity string
	Params   map[string]string
}

// Config maps a rule name to its setting.
type Config map[string]RuleSetting

// ParseConfig builds a Config from the lint: config value (a mapping of
// rule -> severity string, or rule -> {severity, params...}). It validates rule
// names, severities and required parameters.
func ParseConfig(raw interface{}) (Config, error) {
	if raw == nil {
		return nil, nil
	}
	m, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("lint config must be a mapping of rule -> severity")
	}
	cfg := Config{}
	for name, v := range m {
		meta, known := Known(name)
		if !known {
			return nil, fmt.Errorf("unknown lint rule %q", name)
		}
		setting := RuleSetting{Severity: meta.DefaultSev, Params: map[string]string{}}
		switch vv := v.(type) {
		case string:
			setting.Severity = strings.ToLower(strings.TrimSpace(vv))
		case map[string]interface{}:
			for k, pv := range vv {
				if strings.ToLower(k) == "severity" {
					setting.Severity = strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", pv)))
					continue
				}
				setting.Params[k] = fmt.Sprintf("%v", pv)
			}
		default:
			return nil, fmt.Errorf("lint rule %q must be a severity string or a map", name)
		}
		if !validSeverities[setting.Severity] {
			return nil, fmt.Errorf("lint rule %q has invalid severity %q — use error, warning or information", name, setting.Severity)
		}
		for _, rp := range meta.RequiredParams {
			if strings.TrimSpace(setting.Params[rp]) == "" {
				return nil, fmt.Errorf("lint rule %q requires parameter %q", name, rp)
			}
		}
		cfg[name] = setting
	}
	return cfg, nil
}

// enabledRule pairs a built-in rule with its resolved setting.
type enabledRule struct {
	rule     builtinRule
	severity string
	params   map[string]string
}

// Engine runs a fixed set of enabled convention rules against resources.
type Engine struct {
	rules []enabledRule
}

// NewEngine builds an Engine from cfg. It returns nil (no error) when cfg is
// empty, so callers can skip linting cheaply.
func NewEngine(cfg Config) (*Engine, error) {
	if len(cfg) == 0 {
		return nil, nil
	}
	// Deterministic order for stable output.
	names := make([]string, 0, len(cfg))
	for name := range cfg {
		names = append(names, name)
	}
	sort.Strings(names)

	e := &Engine{rules: make([]enabledRule, 0, len(names))}
	for _, name := range names {
		r, ok := registry[name]
		if !ok {
			return nil, fmt.Errorf("unknown lint rule %q", name)
		}
		e.rules = append(e.rules, enabledRule{rule: r, severity: cfg[name].Severity, params: cfg[name].Params})
	}
	return e, nil
}

// Len reports how many rules the engine holds.
func (e *Engine) Len() int { return len(e.rules) }

// Evaluate runs every enabled rule against one resource and returns findings as
// issues with message ID "lint:<rule>".
func (e *Engine) Evaluate(resourceJSON []byte) []validator.Issue {
	root := gjson.ParseBytes(resourceJSON)
	var issues []validator.Issue
	for _, er := range e.rules {
		for _, f := range er.rule.check(root, er.params) {
			issues = append(issues, validator.Issue{
				Severity:  er.severity,
				Message:   f.message,
				Location:  f.location,
				MessageID: MessageIDPrefix + er.rule.name,
			})
		}
	}
	return issues
}

// EvaluateResult merges findings into an existing result, clearing Valid when an
// error-severity finding is produced.
func (e *Engine) EvaluateResult(res *validator.Result, resourceJSON []byte) {
	found := e.Evaluate(resourceJSON)
	if len(found) == 0 {
		return
	}
	res.Issues = append(res.Issues, found...)
	for _, iss := range found {
		if iss.Severity == "error" || iss.Severity == "fatal" {
			res.Valid = false
		}
	}
}
