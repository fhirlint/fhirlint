// Package fhirpath implements the subset of FHIRPath used by fhirlint's custom
// lint rules. It evaluates expressions directly against FHIR JSON (via gjson),
// so it is FHIR-version agnostic (R4/R4B/R5) and needs no JVM or generated
// structs.
//
// Supported: path navigation with an optional leading resourceType filter,
// indexers ([n]); the functions exists, empty, where, all, count, first, last,
// not, hasValue, length, toString, startsWith, endsWith, contains, matches;
// the operators = != < > <= >=, and/or/xor/implies, and membership with `in`;
// and boolean, string ('...') and numeric literals.
//
// Not supported (a rule using these fails to compile, rather than silently
// misbehaving): arithmetic (+ - * / div mod &), union (|), the ~/!~ operators,
// date/quantity literals, and functions outside the list above.
package fhirpath

import (
	"fmt"

	"github.com/tidwall/gjson"
)

// Program is a compiled FHIRPath assertion, safe for concurrent evaluation.
type Program struct {
	root node
	src  string
}

// Compile parses expr into a reusable Program, returning an error for syntax or
// constructs outside the supported subset.
func Compile(expr string) (*Program, error) {
	n, err := Parse(expr)
	if err != nil {
		return nil, err
	}
	return &Program{root: n, src: expr}, nil
}

// String returns the original expression source.
func (p *Program) String() string { return p.src }

// EvalBool evaluates the assertion against a FHIR resource and reports whether
// it holds. An empty or false result is reported as false; a result that is not
// a single boolean is an error, so a rule that does not express a predicate is
// surfaced rather than guessed at.
func (p *Program) EvalBool(resourceJSON []byte) (bool, error) {
	if !gjson.ValidBytes(resourceJSON) {
		return false, fmt.Errorf("resource is not valid JSON")
	}
	root := gjson.ParseBytes(resourceJSON)
	res, err := eval(p.root, []value{root}, []value{root})
	if err != nil {
		return false, err
	}
	switch len(res) {
	case 0:
		return false, nil
	case 1:
		b, ok := res[0].(bool)
		if !ok {
			return false, fmt.Errorf("expression did not evaluate to a boolean (got %s)", typeName(res[0]))
		}
		return b, nil
	default:
		return false, fmt.Errorf("expression evaluated to %d items, expected a boolean", len(res))
	}
}
