package qualify

import (
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

func result(path string, issues ...validator.Issue) *validator.Result {
	return &validator.Result{Filename: path, Issues: issues}
}

func errIssue() validator.Issue  { return validator.Issue{Severity: "error", Message: "boom"} }
func warnIssue() validator.Issue { return validator.Issue{Severity: "warning", Message: "meh"} }

func TestEvaluate_ValidCasePasses(t *testing.T) {
	cases := []Case{{Name: "v.json", Path: "/tmp/v.json", Expected: Expected{Valid: true}}}
	res := map[string]*validator.Result{"/tmp/v.json": result("/tmp/v.json", warnIssue())}
	out := Evaluate(cases, res)
	if !out[0].Pass {
		t.Errorf("valid case with only a warning should pass, got: %+v", out[0])
	}
	if out[0].Expectation != "valid" {
		t.Errorf("expected expectation=valid, got %q", out[0].Expectation)
	}
}

func TestEvaluate_ValidCaseFailsOnError(t *testing.T) {
	cases := []Case{{Name: "v.json", Path: "/tmp/v.json", Expected: Expected{Valid: true}}}
	res := map[string]*validator.Result{"/tmp/v.json": result("/tmp/v.json", errIssue())}
	if Evaluate(cases, res)[0].Pass {
		t.Error("valid case with an error should fail")
	}
}

func TestEvaluate_InvalidCasePassesOnError(t *testing.T) {
	cases := []Case{{Name: "i.json", Path: "/tmp/i.json", Expected: Expected{Valid: false}}}
	res := map[string]*validator.Result{"/tmp/i.json": result("/tmp/i.json", errIssue())}
	out := Evaluate(cases, res)[0]
	if !out.Pass {
		t.Errorf("invalid case with an error should pass, got: %+v", out)
	}
	if out.Expectation != "invalid" {
		t.Errorf("expected expectation=invalid, got %q", out.Expectation)
	}
}

func TestEvaluate_InvalidCaseFailsWhenAccepted(t *testing.T) {
	cases := []Case{{Name: "i.json", Path: "/tmp/i.json", Expected: Expected{Valid: false}}}
	res := map[string]*validator.Result{"/tmp/i.json": result("/tmp/i.json")}
	if Evaluate(cases, res)[0].Pass {
		t.Error("invalid case with no errors should fail")
	}
}

func TestEvaluate_MissingResultFails(t *testing.T) {
	cases := []Case{{Name: "x.json", Path: "/tmp/x.json", Expected: Expected{Valid: true}}}
	out := Evaluate(cases, map[string]*validator.Result{})[0]
	if out.Pass {
		t.Error("a case with no validation result should fail")
	}
}

func TestEvaluate_RequiredMessageIDMissing(t *testing.T) {
	cases := []Case{{Name: "i.json", Path: "/tmp/i.json", Expected: Expected{Valid: false, MessageIDs: []string{"dom-6"}}}}
	res := map[string]*validator.Result{"/tmp/i.json": result("/tmp/i.json",
		validator.Issue{Severity: "error", MessageID: "other"})}
	if Evaluate(cases, res)[0].Pass {
		t.Error("should fail when a required messageId is absent")
	}
}

func TestEvaluate_RequiredMessageIDPresent(t *testing.T) {
	cases := []Case{{Name: "i.json", Path: "/tmp/i.json", Expected: Expected{Valid: false, MessageIDs: []string{"dom-6"}}}}
	res := map[string]*validator.Result{"/tmp/i.json": result("/tmp/i.json",
		validator.Issue{Severity: "error", MessageID: "dom-6"})}
	if !Evaluate(cases, res)[0].Pass {
		t.Error("should pass when the required messageId is present")
	}
}

func TestSummarize(t *testing.T) {
	passed, failed, qualified := Summarize([]CaseResult{{Pass: true}, {Pass: true}})
	if passed != 2 || failed != 0 || !qualified {
		t.Errorf("expected 2/0/qualified, got %d/%d/%v", passed, failed, qualified)
	}
	passed, failed, qualified = Summarize([]CaseResult{{Pass: true}, {Pass: false}})
	if passed != 1 || failed != 1 || qualified {
		t.Errorf("expected 1/1/not-qualified, got %d/%d/%v", passed, failed, qualified)
	}
	if _, _, q := Summarize(nil); q {
		t.Error("empty results must not be qualified")
	}
}
