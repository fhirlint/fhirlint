package validator

import (
	"os"
	"strings"
	"testing"
)

// profile-fetch-errors.json holds the four VALIDATION_VAL_PROFILE_UNKNOWN_ERROR
// messages validator 6.10.4 produced for Patient instances claiming a KBV, an
// MII, an ISiK and a Basisprofil-DE profile with no --ig (#445). Only the
// fhir.de one quotes a whole page.
func TestParseOutput_TrimsQuotedResponsePage(t *testing.T) {
	data, err := os.ReadFile("testdata/profile-fetch-errors.json")
	if err != nil {
		t.Fatal(err)
	}
	results, err := parseOutput(data, []string{"patient.json"}, "")
	if err != nil {
		t.Fatal(err)
	}
	issues := results[0].Issues
	if len(issues) != 4 {
		t.Fatalf("want 4 issues, got %d", len(issues))
	}

	var fhirDE string
	for _, iss := range issues {
		if strings.Contains(iss.Message, "patient-de-basis") {
			fhirDE = iss.Message
		}
	}
	want := "Profile reference 'http://fhir.de/StructureDefinition/patient-de-basis' has not been checked " +
		"because it could not be found, and fetching it resulted in the error " +
		"org.hl7.fhir.r4.utils.client.EFhirClientException: Error from https://fhir.de: " +
		"(response from fhir.de omitted, "
	if !strings.HasPrefix(fhirDE, want) || !strings.HasSuffix(fhirDE, " characters)") {
		t.Errorf("fhir.de message not trimmed:\n%s", fhirDE)
	}
	if strings.Contains(fhirDE, "window.dataLayer") {
		t.Error("page script survived the trim")
	}
}

func TestParseOutput_KeepsShortFetchReasons(t *testing.T) {
	data, err := os.ReadFile("testdata/profile-fetch-errors.json")
	if err != nil {
		t.Fatal(err)
	}
	results, err := parseOutput(data, []string{"patient.json"}, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, iss := range results[0].Issues {
		if strings.Contains(iss.Message, "patient-de-basis") {
			continue
		}
		if strings.Contains(iss.Message, "omitted") {
			t.Errorf("short reason was trimmed: %s", iss.Message)
		}
	}
}

func TestTrimFetchErrorBody(t *testing.T) {
	long := strings.Repeat("x", maxFetchBody+1)
	cases := []struct{ name, in, want string }{
		{"no fetch error", "Patient.birthDate: minimum required = 1", "Patient.birthDate: minimum required = 1"},
		{
			"body at the limit",
			"E: x.EFhirClientException: Error from https://a.example/fhir: " + strings.Repeat("y", maxFetchBody),
			"E: x.EFhirClientException: Error from https://a.example/fhir: " + strings.Repeat("y", maxFetchBody),
		},
		{
			"body over the limit",
			"E: x.EFhirClientException: Error from https://a.example:8443/fhir: " + long,
			"E: x.EFhirClientException: Error from https://a.example:8443/fhir: (response from a.example:8443 omitted, 301 characters)",
		},
		{
			"marker without a body separator",
			"x.EFhirClientException: Error from https://a.example",
			"x.EFhirClientException: Error from https://a.example",
		},
	}
	for _, tc := range cases {
		if got := trimFetchErrorBody(tc.in); got != tc.want {
			t.Errorf("%s:\n got %q\nwant %q", tc.name, got, tc.want)
		}
	}
}
