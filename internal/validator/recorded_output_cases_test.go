// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package validator

import (
	"path/filepath"
	"regexp"
	"strings"
)

// recordedOutputDir holds real validator output, one directory per validator
// version, and the inputs it was recorded from (#715). The unit tests parse it
// on every `go test`; scripts/record-validator-output.sh records it again from
// a real JAR. See its README.md.
var recordedOutputDir = filepath.Join("testdata", "validator-output")

// recordedInputsDir is where each case keeps the files it validates, laid out
// as the recorder's working directory: in/ for the resources, ig/ for a local
// IG.
const recordedInputsDir = "inputs"

// recordedCase is one run of the JAR the recordings keep. The recorder and the
// tests share the list, so a case that is added here and not yet recorded
// fails the test for every version instead of going untested.
type recordedCase struct {
	name string
	// paths are what fhirlint passes the JAR, relative to the working
	// directory, so the recording names them as a user's run would.
	paths []string
	// opts are the case's options on top of FHIR R4. The recorder gives them
	// to the JAR; the test gives them to readRun, whose error messages name
	// the terminology server.
	opts Options
}

var recordedCases = []recordedCase{
	// One file: the JAR writes a single OperationOutcome.
	{name: "single-file", paths: []string{"in/bad-gender.json"}, opts: Options{NoTerminologyServer: true}},
	// Two files: the JAR writes a Bundle, mapped back to the paths by position.
	{name: "two-files", paths: []string{"in/patient.json", "in/bad-gender.json"}, opts: Options{NoTerminologyServer: true}},
	// hapifhir/org.hl7.fhir.core#2736: the R5 cross-version extension makes
	// the JAR throw on the second file and write no output at all (#471).
	{name: "crash", paths: []string{"in/patient.json", "in/bodystructure.json"}, opts: Options{NoTerminologyServer: true}},
	// A profile that slices the non-repeating Patient.maritalStatus. The JAR
	// cannot generate its snapshot and says why on stdout only (#521).
	{name: "snapshot-failure", paths: []string{"in/patient.json", "in/patient2.json"}, opts: Options{NoTerminologyServer: true, IGs: []string{"./ig"}}},
	// A plain-HTTP server on loopback: the JAR refuses it before it asks
	// (SSRF protection, 6.10.0 and later), which is not a connection failure.
	{name: "tx-refused", paths: []string{"in/patient.json"}, opts: Options{TerminologyServer: "http://127.0.0.1:9"}},
	// A server whose host does not resolve: the JAR gives up on its
	// capability statement before it validates anything.
	{name: "tx-unreachable", paths: []string{"in/patient.json"}, opts: Options{TerminologyServer: "https://tx.invalid"}},
	// German codes against the live terminology servers. From 7.0 on the
	// validator routes them to tx.fhir.de, whose answers come without message
	// ids (#449, #465).
	{name: "tx-no-id", paths: []string{"in/medication.json"}},
}

// recordedRun is what a recording keeps about the process besides its
// streams and its report.
type recordedRun struct {
	Paths  []string `json:"paths"`
	Failed bool     `json:"failed"` // non-zero exit
}

// recordingNormaliser takes out of a recording what belongs to the machine and
// the moment it was made on, so that a re-recording diffs only where the
// validator changed: the working directory, the home directory, the temp file
// fhirlint writes the report to, timings, the JAR's age, the JVM and its
// memory.
type recordingNormaliser struct {
	work []string // the working directory, also with symlinks resolved
	home string
}

var (
	reportTempFile = regexp.MustCompile(`\S*fhirlint-result-\d+\.json`)
	logTiming      = regexp.MustCompile(`\b\d{2}:\d{2}\.\d{3}\b`)
	jarAge         = regexp.MustCompile(`\((?:-?\d+ (?:days|hours|mins|ms) old|\?\?)\)`)
	javaLine       = regexp.MustCompile(`(?m)^(\s*Java:\s+).*$`)
	memoryLine     = regexp.MustCompile(`\b\d+MB available\b`)
)

func (n recordingNormaliser) normalise(s string) string {
	s = reportTempFile.ReplaceAllString(s, "out.json")
	for _, w := range n.work {
		if w != "" {
			s = strings.ReplaceAll(s, w, "/work")
		}
	}
	if n.home != "" {
		s = strings.ReplaceAll(s, n.home, "/home/user")
	}
	s = logTiming.ReplaceAllString(s, "00:00.100")
	s = jarAge.ReplaceAllString(s, "(1 days old)")
	s = javaLine.ReplaceAllString(s, "${1}<java> from /usr/lib/jvm on amd64 (64bit). 1000MB available")
	return memoryLine.ReplaceAllString(s, "1000MB available")
}
