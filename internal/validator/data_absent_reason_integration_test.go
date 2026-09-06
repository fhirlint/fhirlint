//go:build integration

package validator

import (
	"strings"
	"testing"
)

// A FHIR primitive may carry a data-absent-reason extension and no value:
//
//	"_birthDate": {"extension": [{"url": ".../data-absent-reason", "valueCode": "unknown"}]}
//
// It is the standard way to say "this is missing, and here is why", and German
// profiles use it throughout: de.basisprofil.r4 alone mentions it in 14 files.
//
// Two independently built validators broke on it at the same time. The HL7 Java
// validator fixed it in 6.10.3, and Firely's .NET engine has the same bug open
// as FirelyTeam/firely-validator-api#677. When two teams get one construct
// wrong, it is a shape the specification makes easy to get wrong rather than an
// exotic input (#389).
//
// **What actually breaks is a comparison, not the element.** Upstream commit
// 707ddf1f added a hasNoPrimitiveValue guard to opLessThan, opGreater and the
// other comparison operators in FHIRPathEngine. Merely parsing, validating or
// navigating such a resource never failed, which was worth finding out: the
// first version of these tests exercised structural validation and passed
// against 6.10.2 as happily as against 6.10.3, so they would have guarded
// nothing.
//
// Reproduced against 6.10.2, the release before the fix:
//
//	Patient.birthDate < today()
//	  Cannot invoke "java.lang.Integer.intValue()" because the return value of
//	  "org.hl7.fhir.r5.model.BaseDateTimeType.getYear()" is null
//
// and against 6.10.3, where all three comparisons return empty.
//
// fhirlint pins a validator and bumps it deliberately, so a reintroduced
// regression would sit between two bumps with nothing to catch it. On the Java
// side the failure is an uncaught exception, which reaches a user as a dead
// validator rather than as a finding — the class of failure #351 was about.

// darPatient is a Patient whose birthDate is present but has no value.
func darPatient() string { return testdataPath("data-absent-reason", "patient-birthdate.json") }

// A comparison against a value-less primitive must yield empty, not an
// exception. This is the regression itself.
func TestIntegration_DataAbsentReason_ComparisonDoesNotThrow(t *testing.T) {
	for _, expr := range []string{
		"Patient.birthDate < today()",
		"Patient.birthDate > @1900-01-01",
		"Patient.birthDate <= @2000-01-01",
	} {
		t.Run(expr, func(t *testing.T) {
			r, err := RunFHIRPath(expr, darPatient(), FHIRPathOptions{FHIRVersion: "4.0.1"})
			if err != nil {
				// Naming the old symptom makes a re-broken JAR obvious rather
				// than leaving a Java stack trace to be decoded again.
				if strings.Contains(err.Error(), "NullPointerException") ||
					strings.Contains(err.Error(), "getYear()") {
					t.Fatalf("the validator threw on a value-less primitive, as it did before 6.10.3: %v", err)
				}
				t.Fatalf("RunFHIRPath(%q) error: %v", expr, err)
			}
			// The FHIR spec says such an element has no value in the FHIRPath
			// type system, so a comparison has nothing to compare.
			if !r.Empty() {
				t.Errorf("got %v, want an empty result: the element has no value to compare", r.Items)
			}
		})
	}
}

// The guard that keeps the above from being satisfied by a validator that meets
// the construct and quietly stops working. exists() must still be true, because
// the element *is* present — only its value is absent.
func TestIntegration_DataAbsentReason_ElementIsStillPresent(t *testing.T) {
	r, err := RunFHIRPath("Patient.birthDate.exists()", darPatient(), FHIRPathOptions{FHIRVersion: "4.0.1"})
	if err != nil {
		t.Fatalf("RunFHIRPath() error: %v", err)
	}
	if len(r.Items) != 1 || r.Items[0] != "true" {
		t.Errorf("got %q, want [true]: the element is present, only its value is not", r.Items)
	}
}

// And validation of the same resource must still run and still check things.
// A validator that survived the construct by skipping the resource would pass
// the tests above.
func TestIntegration_DataAbsentReason_ValidationStillChecksTheResource(t *testing.T) {
	valid, err := Run(darPatient(), Options{FHIRVersion: "4.0.1", NoTerminologyServer: true})
	if err != nil {
		t.Fatalf("validating a resource with a value-less primitive must not kill the run: %v", err)
	}
	if !valid.Valid {
		t.Errorf("the resource is valid FHIR; got issues: %v", valid.Issues)
	}

	// Same shape, plus a plainly invalid gender. If that is still reported, the
	// validator was doing its job and not merely surviving.
	broken, err := Run(testdataPath("data-absent-reason", "patient-birthdate-bad-gender.json"),
		Options{FHIRVersion: "4.0.1", NoTerminologyServer: true})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if broken.Valid {
		t.Error("the resource has an invalid gender; a validator that passed it stopped checking")
	}
	if !containsMessageFragment(broken, "gender") {
		t.Errorf("want the gender finding alongside the data-absent-reason element, got: %v", broken.Issues)
	}
}
