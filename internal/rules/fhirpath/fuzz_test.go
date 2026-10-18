package fhirpath

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzEval compiles a fuzzed expression and evaluates it against a fuzzed
// resource. The expression comes from a rule file and the resource from a
// repository the user did not write, and both reach a hand-written lexer,
// parser and evaluator. A malformed expression is a compile error and an
// expression that does not yield a boolean is an evaluation error; neither may
// panic, and evaluating the same program twice must give the same answer.
func FuzzEval(f *testing.F) {
	exprs := []string{
		"identifier.exists()",
		"Patient.identifier.where(system = 'http://hospital.example/mrn').value = '12345'",
		"name.count() = 2 and name.given.count() >= 1",
		"name.family.startsWith('Muster') xor name.family.endsWith('mann')",
		"identifier.all(value.exists() and system.exists())",
		"name.given.exists() implies name.exists()",
		"name.family.matches('^Muster.*')",
		"identifier.where(value.matches($this.system)).empty()",
		"name[0].given[1].length() > 3",
		"gender in ('male' | 'female')",
		"birthDate < '2000-01-01' or deceased.hasValue().not()",
		"telecom.first().value.contains('@') != false",
		"extension.where(url = 'x').value.toString() = '1.0'",
		"((((((name))))))",
	}
	resources := [][]byte{[]byte(patient)}
	seeds, _ := filepath.Glob("../../../testdata/*/*.json")
	for _, p := range append(seeds, "../../../testdata/patient.json") {
		if data, err := os.ReadFile(p); err == nil { //nolint:gosec // fixed test fixtures
			resources = append(resources, data)
		}
	}
	for i, e := range exprs {
		f.Add(e, resources[i%len(resources)])
	}
	f.Add("a.b.c", []byte(`{"a":[{"b":[{"c":true}]},{"b":{"c":[1,2.5,"x",null]}}]}`))

	f.Fuzz(func(t *testing.T, expr string, resource []byte) {
		prog, err := Compile(expr)
		if err != nil {
			return
		}
		if prog.String() != expr {
			t.Errorf("String() = %q, want the source %q", prog.String(), expr)
		}
		got, err := prog.EvalBool(resource)
		again, err2 := prog.EvalBool(resource)
		if got != again || (err == nil) != (err2 == nil) {
			t.Errorf("two evaluations disagree: (%v, %v) then (%v, %v)", got, err, again, err2)
		}
	})
}
