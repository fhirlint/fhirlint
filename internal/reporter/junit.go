package reporter

import (
	"encoding/xml"
	"fmt"
	"os"

	"github.com/fhirlint/fhirlint/internal/validator"
)

type junitTestSuites struct {
	XMLName  xml.Name         `xml:"testsuites"`
	Name     string           `xml:"name,attr"`
	Tests    int              `xml:"tests,attr"`
	Failures int              `xml:"failures,attr"`
	Suites   []junitTestSuite `xml:"testsuite"`
}

type junitTestSuite struct {
	Name     string `xml:"name,attr"`
	Tests    int    `xml:"tests,attr"`
	Failures int    `xml:"failures,attr"`
	// Properties is where JUnit XML puts what the run knew that no test case
	// is about. It hangs off testsuite, not testsuites: that is where the
	// common schema (Ant, Jenkins, GitLab) allows it, and a properties element
	// one level up is dropped or rejected by strict readers.
	Properties *junitProperties `xml:"properties,omitempty"`
	TestCases  []junitTestCase  `xml:"testcase"`
}

type junitProperties struct {
	Property []junitProperty `xml:"property"`
}

type junitProperty struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

type junitTestCase struct {
	Name      string         `xml:"name,attr"`
	Classname string         `xml:"classname,attr"`
	Failures  []junitFailure `xml:"failure,omitempty"`
}

type junitFailure struct {
	Type    string `xml:"type,attr"`
	Message string `xml:"message,attr"`
	Body    string `xml:",chardata"`
}

func JUnit(results []*validator.Result, minSeverity string, info RunInfo, dest string) error {
	suites := buildJUnitReport(results, minSeverity, info)
	out, err := xml.MarshalIndent(suites, "", "  ")
	if err != nil {
		return err
	}
	data := []byte(xml.Header + string(out) + "\n")
	if dest == "" {
		fmt.Print(string(data))
		return nil
	}
	return os.WriteFile(dest, data, 0600)
}

func buildJUnitReport(results []*validator.Result, minSeverity string, info RunInfo) junitTestSuites {
	info = info.WithResults(results)
	cases := make([]junitTestCase, 0, len(results))
	totalFailures := 0

	for _, r := range results {
		issues := filterIssues(r.Issues, minSeverity)
		tc := junitTestCase{
			Name:      r.Label,
			Classname: "fhirlint",
		}
		for _, iss := range issues {
			body := iss.Message
			if iss.Location != "" {
				body += " @ " + iss.Location
			}
			tc.Failures = append(tc.Failures, junitFailure{
				Type:    iss.Severity,
				Message: iss.Message,
				Body:    body,
			})
			totalFailures++
		}
		cases = append(cases, tc)
	}

	return junitTestSuites{
		Name:     "fhirlint",
		Tests:    len(results),
		Failures: totalFailures,
		Suites: []junitTestSuite{{
			Name:       "FHIR Validation",
			Tests:      len(results),
			Failures:   totalFailures,
			Properties: junitPropertiesFor(info),
			TestCases:  cases,
		}},
	}
}

// junitPropertiesFor renders the run's provenance as properties, in the
// dotted style JUnit consumers expect, omitting what the run could not tell.
func junitPropertiesFor(info RunInfo) *junitProperties {
	var props []junitProperty
	add := func(name, value string) {
		if value != "" {
			props = append(props, junitProperty{Name: name, Value: value})
		}
	}
	add("fhirlint.version", info.Fhirlint)
	add("validator.version", info.Validator)
	add("validator.build", info.ValidatorBuild)
	add("fhir.version", info.FHIRVersion)
	if len(props) == 0 {
		return nil
	}
	return &junitProperties{Property: props}
}
