// Package qualify runs fhirlint against a set of known-good and known-bad FHIR
// resources and produces an Operational Qualification (OQ) report — documented
// evidence, for Computer System Validation under ISO 13485 / IEC 62304 / FDA
// 21 CFR Part 11, that the tool correctly accepts valid resources and rejects
// invalid ones.
package qualify

import (
	"github.com/fhirlint/fhirlint/internal/validator"
)

// Expected declares the outcome a test case must produce to pass.
type Expected struct {
	Description string   `json:"description"`
	Valid       bool     `json:"valid"`                // true: must have no errors; false: must have an error
	MessageIDs  []string `json:"messageIds,omitempty"` // optional: these message IDs must all be present
}

// Case is a single qualification test case: a FHIR file on disk plus its
// expected outcome. Name is a stable, human-readable identifier.
type Case struct {
	Name     string
	Path     string
	Expected Expected
}

// CaseResult records how one case fared against its expectation.
type CaseResult struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Expectation string   `json:"expectation"` // "valid" or "invalid"
	Pass        bool     `json:"pass"`
	Detail      string   `json:"detail"`
	Errors      int      `json:"errors"`
	Warnings    int      `json:"warnings"`
	MessageIDs  []string `json:"expectedMessageIds,omitempty"`
}

// Report is the full qualification record, including traceability metadata.
type Report struct {
	ToolVersion string       `json:"toolVersion"`
	JARVersion  string       `json:"jarVersion"`
	JARSHA256   string       `json:"jarSha256"`
	FHIRVersion string       `json:"fhirVersion"`
	Terminology string       `json:"terminology"` // "offline" or the server URL used
	Timestamp   string       `json:"timestamp"`
	Cases       []CaseResult `json:"cases"`
	Passed      int          `json:"passed"`
	Failed      int          `json:"failed"`
	Qualified   bool         `json:"qualified"`
}

// Evaluate compares each case against its validation result (keyed by the path
// passed to the validator) and returns the per-case outcomes in input order.
func Evaluate(cases []Case, results map[string]*validator.Result) []CaseResult {
	out := make([]CaseResult, 0, len(cases))
	for _, c := range cases {
		cr := CaseResult{
			Name:        c.Name,
			Description: c.Expected.Description,
			MessageIDs:  c.Expected.MessageIDs,
		}
		if c.Expected.Valid {
			cr.Expectation = "valid"
		} else {
			cr.Expectation = "invalid"
		}

		r, ok := results[c.Path]
		if !ok || r == nil {
			cr.Pass = false
			cr.Detail = "no validation result produced"
			out = append(out, cr)
			continue
		}

		cr.Errors, cr.Warnings = countSeverities(r.Issues)
		hasError := cr.Errors > 0
		evaluateCase(&cr, c.Expected, hasError, r.Issues)
		out = append(out, cr)
	}
	return out
}

func evaluateCase(cr *CaseResult, exp Expected, hasError bool, issues []validator.Issue) {
	if exp.Valid {
		if hasError {
			cr.Pass = false
			cr.Detail = "unexpected error(s) in a known-valid resource"
			return
		}
		cr.Pass = true
		cr.Detail = "accepted, no errors (expected)"
	} else {
		if !hasError {
			cr.Pass = false
			cr.Detail = "expected an error but the resource was accepted"
			return
		}
		cr.Pass = true
		cr.Detail = "error detected (expected)"
	}

	// Optional: every declared message ID must be present.
	for _, want := range exp.MessageIDs {
		if !hasMessageID(issues, want) {
			cr.Pass = false
			cr.Detail = "missing expected messageId: " + want
			return
		}
	}
}

// Summarize tallies pass/fail counts. A run is qualified when every case passes
// (and there is at least one case).
func Summarize(results []CaseResult) (passed, failed int, qualified bool) {
	for _, r := range results {
		if r.Pass {
			passed++
		} else {
			failed++
		}
	}
	qualified = failed == 0 && passed > 0
	return passed, failed, qualified
}

func countSeverities(issues []validator.Issue) (errors, warnings int) {
	for _, iss := range issues {
		switch iss.Severity {
		case "error", "fatal":
			errors++
		case "warning":
			warnings++
		}
	}
	return errors, warnings
}

func hasMessageID(issues []validator.Issue, id string) bool {
	for _, iss := range issues {
		if iss.MessageID == id {
			return true
		}
	}
	return false
}
