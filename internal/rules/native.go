package rules

import "github.com/fhirlint/fhirlint/internal/rules/fhirpath"

// nativeEvaluator is the default Evaluator: it compiles assertions with the
// built-in native FHIRPath subset engine, which runs in-process against FHIR
// JSON (no JVM, version-agnostic).
type nativeEvaluator struct{}

// NewNativeEvaluator returns the default in-process FHIRPath evaluator.
func NewNativeEvaluator() Evaluator { return nativeEvaluator{} }

func (nativeEvaluator) Compile(expr string) (Program, error) {
	return fhirpath.Compile(expr)
}
