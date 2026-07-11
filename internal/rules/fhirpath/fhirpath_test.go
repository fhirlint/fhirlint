package fhirpath

import "testing"

const patient = `{
  "resourceType": "Patient",
  "id": "example-1",
  "active": true,
  "name": [
    {"use": "official", "family": "Mustermann", "given": ["Max", "Erika"]},
    {"use": "nickname", "given": ["Maxi"]}
  ],
  "identifier": [
    {"system": "http://hospital.example/mrn", "value": "12345"},
    {"system": "http://fhir.de/sid/gkv/kvid-10", "value": "A123456789"}
  ],
  "telecom": [{"system": "phone", "value": "+49 30 123456"}]
}`

func evalBool(t *testing.T, expr, resource string) bool {
	t.Helper()
	p, err := Compile(expr)
	if err != nil {
		t.Fatalf("Compile(%q): %v", expr, err)
	}
	got, err := p.EvalBool([]byte(resource))
	if err != nil {
		t.Fatalf("EvalBool(%q): %v", expr, err)
	}
	return got
}

func TestEvalBoolTrueCases(t *testing.T) {
	exprs := []string{
		"identifier.exists()",
		"Patient.identifier.exists()",
		"name.exists()",
		"name.given.exists()",
		"identifier.where(system='http://hospital.example/mrn').exists()",
		"identifier.where(system = 'http://hospital.example/mrn').value = '12345'",
		"active = true",
		"active",
		"name.count() = 2",
		"name.given.count() = 3",
		"name.first().use = 'official'",
		"name[1].given = 'Maxi'",
		"name.family.startsWith('Muster')",
		"identifier.all(system.exists())",
		"identifier.all(value.exists() and system.exists())",
		"telecom.where(system='phone').exists()",
		"name.where(use='official').family = 'Mustermann'",
		"active = true and identifier.exists()",
		"identifier.exists() or name.empty()",
		"name.family.matches('^Muster.*')",
		"id = 'example-1'",
		"'phone' in telecom.system",
		"name.given.exists() implies name.exists()",
		"identifier.where(system='http://hospital.example/mrn').value.length() = 5",
		"name.family.hasValue()",
	}
	for _, e := range exprs {
		if !evalBool(t, e, patient) {
			t.Errorf("expected TRUE for %q", e)
		}
	}
}

func TestEvalBoolFalseCases(t *testing.T) {
	exprs := []string{
		"identifier.where(system='http://does-not-exist').exists()",
		"name.count() = 5",
		"active = false",
		"deceased.exists()",
		"name.family = 'Nobody'",
		"name.given.count() = 1",
		"identifier.all(value = '12345')",
		"'fax' in telecom.system",
		"communication.exists()",
		"name.family.startsWith('X')",
	}
	for _, e := range exprs {
		if evalBool(t, e, patient) {
			t.Errorf("expected FALSE for %q", e)
		}
	}
}

func TestCompileErrors(t *testing.T) {
	bad := []string{
		"identifier +",           // dangling operator / unsupported '+'
		"name.given &",           // unsupported '&'
		"a | b",                  // unsupported union
		"value ~ 'x'",            // unsupported '~'
		"where(",                 // unbalanced
		"name.[0]",               // malformed
		"exists(",                // unbalanced
		"identifier.unknownFn()", // this parses; error surfaces at eval, see below
	}
	// The last one parses fine; drop it from compile-error expectations.
	for _, e := range bad[:len(bad)-1] {
		if _, err := Compile(e); err == nil {
			t.Errorf("expected compile error for %q", e)
		}
	}
}

func TestEvalErrors(t *testing.T) {
	cases := []string{
		"identifier.unknownFn()",  // unsupported function
		"name",                    // not a boolean predicate (multiple objects)
		"identifier.system",       // non-boolean result
		"name.given < identifier", // ordering complex/multi
	}
	for _, e := range cases {
		p, err := Compile(e)
		if err != nil {
			continue // acceptable: rejected at compile time
		}
		if _, err := p.EvalBool([]byte(patient)); err == nil {
			t.Errorf("expected eval error for %q", e)
		}
	}
}

func TestResourceTypeFilter(t *testing.T) {
	// A path prefixed with a non-matching resourceType selects nothing.
	if evalBool(t, "Observation.identifier.exists()", patient) {
		t.Error("Observation.* must not match a Patient resource")
	}
}

func TestEmptyPredicateIsFalse(t *testing.T) {
	// Comparing a missing element yields empty, which gates to false.
	if evalBool(t, "deceased = true", patient) {
		t.Error("comparison with a missing element should be false")
	}
}
