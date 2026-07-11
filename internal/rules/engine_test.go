package rules

import (
	"fmt"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

// fakeEvaluator compiles an expression into a fakeProgram whose boolean/err are
// looked up by the expression text, decoupling engine tests from a real
// FHIRPath implementation.
type fakeEvaluator struct {
	results     map[string]bool
	evalErrs    map[string]error
	compileErrs map[string]error
}

func (f fakeEvaluator) Compile(expr string) (Program, error) {
	if err, ok := f.compileErrs[expr]; ok {
		return nil, err
	}
	return fakeProgram{expr: expr, ev: f}, nil
}

type fakeProgram struct {
	expr string
	ev   fakeEvaluator
}

func (p fakeProgram) EvalBool([]byte) (bool, error) {
	if err, ok := p.ev.evalErrs[p.expr]; ok {
		return false, err
	}
	return p.ev.results[p.expr], nil
}

func TestEngineEvaluate(t *testing.T) {
	ruleset := []Rule{
		{ID: "has-id", Resource: "Patient", Assert: "identifier.exists()", Severity: "error"},
		{ID: "has-name", Assert: "name.exists()", Severity: "warning"},
		{ID: "obs-only", Resource: "Observation", Assert: "status.exists()", Severity: "error"},
	}
	eval := fakeEvaluator{results: map[string]bool{
		"identifier.exists()": false, // fails
		"name.exists()":       true,  // holds
		"status.exists()":     false, // would fail, but Patient != Observation so skipped
	}}
	eng, err := NewEngine(ruleset, eval)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	issues := eng.Evaluate([]byte(`{"resourceType":"Patient"}`))
	if len(issues) != 1 {
		t.Fatalf("got %d issues, want 1: %+v", len(issues), issues)
	}
	got := issues[0]
	if got.MessageID != "rule:has-id" || got.Severity != "error" || got.Location != "Patient" {
		t.Fatalf("unexpected issue: %+v", got)
	}
}

func TestNewEngineCompileErrorSurfaces(t *testing.T) {
	ruleset := []Rule{{ID: "broken", Assert: "bad(", Severity: "warning"}}
	eval := fakeEvaluator{compileErrs: map[string]error{"bad(": fmt.Errorf("syntax error")}}
	if _, err := NewEngine(ruleset, eval); err == nil {
		t.Fatal("expected compile error at engine construction")
	}
}

func TestEngineEvaluationErrorSurfaces(t *testing.T) {
	ruleset := []Rule{{ID: "runtime", Assert: "x.y", Severity: "warning"}}
	eval := fakeEvaluator{evalErrs: map[string]error{"x.y": fmt.Errorf("not a boolean")}}
	eng, _ := NewEngine(ruleset, eval)

	issues := eng.Evaluate([]byte(`{"resourceType":"Patient"}`))
	if len(issues) != 1 || issues[0].Severity != "error" {
		t.Fatalf("expected one error-severity issue, got %+v", issues)
	}
}

func TestEvaluateResultMergesAndInvalidates(t *testing.T) {
	ruleset := []Rule{{ID: "has-id", Assert: "identifier.exists()", Severity: "error"}}
	eval := fakeEvaluator{results: map[string]bool{"identifier.exists()": false}}
	eng, _ := NewEngine(ruleset, eval)

	res := &validator.Result{Valid: true}
	eng.EvaluateResult(res, []byte(`{"resourceType":"Patient"}`))
	if res.Valid {
		t.Fatal("expected Valid=false after an error-severity rule finding")
	}
	if len(res.Issues) != 1 || res.Issues[0].MessageID != "rule:has-id" {
		t.Fatalf("expected merged rule issue, got %+v", res.Issues)
	}
}

func TestEvaluateResultWarningKeepsValid(t *testing.T) {
	ruleset := []Rule{{ID: "soft", Assert: "name.exists()", Severity: "warning"}}
	eval := fakeEvaluator{results: map[string]bool{"name.exists()": false}}
	eng, _ := NewEngine(ruleset, eval)

	res := &validator.Result{Valid: true}
	eng.EvaluateResult(res, []byte(`{"resourceType":"Patient"}`))
	if !res.Valid {
		t.Fatal("a warning finding must not invalidate the result")
	}
	if len(res.Issues) != 1 {
		t.Fatalf("expected one warning issue, got %+v", res.Issues)
	}
}
