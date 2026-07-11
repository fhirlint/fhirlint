package rules

import (
	"fmt"

	"github.com/fhirlint/fhirlint/internal/validator"
	"github.com/tidwall/gjson"
)

// Evaluator compiles a FHIRPath assertion into a reusable Program. Compiling up
// front lets the engine validate every expression when rules are loaded (so a
// malformed assert is reported by `config check`, not at validation time) and
// avoids re-parsing the expression for every resource.
//
// It is the only part of the rules package that depends on a concrete FHIRPath
// implementation, so the engine can be swapped independently of the rule model.
type Evaluator interface {
	Compile(expr string) (Program, error)
}

// Program is a compiled FHIRPath assertion that can be evaluated against a
// resource. EvalBool reports whether the assertion holds; an empty or false
// FHIRPath result is reported as false. A result that is not a single boolean
// returns an error.
type Program interface {
	EvalBool(resourceJSON []byte) (bool, error)
}

// compiledRule pairs a rule with its compiled assertion.
type compiledRule struct {
	rule Rule
	prog Program
}

// Engine applies a fixed set of rules to resources using compiled assertions.
type Engine struct {
	rules []compiledRule
}

// NewEngine validates the rule set, compiles every assertion with ev, and
// returns an Engine. A compile error is returned with the offending rule id so
// configuration mistakes surface at load time.
func NewEngine(ruleset []Rule, ev Evaluator) (*Engine, error) {
	if err := Validate(ruleset); err != nil {
		return nil, err
	}
	if ev == nil {
		return nil, fmt.Errorf("rules: nil evaluator")
	}
	e := &Engine{rules: make([]compiledRule, 0, len(ruleset))}
	for _, r := range ruleset {
		prog, err := ev.Compile(r.Assert)
		if err != nil {
			return nil, fmt.Errorf("rule %q: invalid assert %q: %w", r.ID, r.Assert, err)
		}
		e.rules = append(e.rules, compiledRule{rule: r, prog: prog})
	}
	return e, nil
}

// Len reports how many rules the engine holds.
func (e *Engine) Len() int { return len(e.rules) }

// Evaluate runs every applicable rule against one resource and returns the
// findings as issues. A rule whose resourceType filter does not match the
// resource is skipped. An assertion that fails to evaluate is reported as an
// error-severity finding so a broken rule is visible rather than silently dropped.
func (e *Engine) Evaluate(resourceJSON []byte) []validator.Issue {
	resourceType := gjson.GetBytes(resourceJSON, "resourceType").String()

	var issues []validator.Issue
	for _, cr := range e.rules {
		if cr.rule.Resource != "" && cr.rule.Resource != resourceType {
			continue
		}
		ok, err := cr.prog.EvalBool(resourceJSON)
		switch {
		case err != nil:
			issues = append(issues, validator.Issue{
				Severity:  "error",
				Message:   fmt.Sprintf("rule %q could not be evaluated: %v", cr.rule.ID, err),
				Location:  resourceType,
				MessageID: cr.rule.MessageID(),
			})
		case !ok:
			issues = append(issues, validator.Issue{
				Severity:  cr.rule.Severity,
				Message:   cr.rule.failureMessage(),
				Location:  resourceType,
				MessageID: cr.rule.MessageID(),
			})
		}
	}
	return issues
}

// EvaluateResult evaluates every applicable rule against a resource and merges
// the findings into an existing validator.Result, clearing Valid when an
// error-severity finding is produced. resourceJSON is the raw resource the
// result was produced from.
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
