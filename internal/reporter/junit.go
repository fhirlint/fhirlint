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
	Name      string          `xml:"name,attr"`
	Tests     int             `xml:"tests,attr"`
	Failures  int             `xml:"failures,attr"`
	TestCases []junitTestCase `xml:"testcase"`
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

func JUnit(results []*validator.Result, minSeverity, dest string) error {
	suites := buildJUnitReport(results, minSeverity)
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

func buildJUnitReport(results []*validator.Result, minSeverity string) junitTestSuites {
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
			Name:      "FHIR Validation",
			Tests:     len(results),
			Failures:  totalFailures,
			TestCases: cases,
		}},
	}
}
