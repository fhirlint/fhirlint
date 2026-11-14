// Command fakejava stands in for "java" running the HL7 validator, so the
// tests of fhirlint's validate pipeline run without Java (#595). The tests of
// cmd and pkg/fhirlint build it into a directory as "java" and put that
// directory first on PATH; everything between flag parsing and the exit code
// then runs for real, while the integration tests keep covering the real JAR.
//
// It answers the calls fhirlint makes: "java -version", and a validation run
// that writes its result to -output. Each input gets an OperationOutcome whose
// issues are decided by markers in the input text:
//
//	FAKE_ERROR    an error, message id FAKE_ERROR_ID, at Patient.name, line 1
//	FAKE_WARNING  a warning, message id FAKE_WARNING_ID, at Patient, line 1
//	FAKE_CRASH    the JAR prints a Java exception and exits 1 without output
//
// An input without a marker gets the validator's "All OK" information issue.
// Several inputs come back as a Bundle of OperationOutcomes, one per input in
// order, as the real validator writes them.
//
// Two environment variables let a test look behind it:
//
//	FAKEJAVA_LOG     append each validation call's arguments to this file,
//	                 one JSON array per line
//	FAKEJAVA_REFUSE  answer "java -version" but fail every validation run,
//	                 to prove a result came from somewhere else (the cache)
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Environment variables a test sets for the fake.
const (
	LogEnv    = "FAKEJAVA_LOG"
	RefuseEnv = "FAKEJAVA_REFUSE"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.Getenv))
}

func run(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	if len(args) == 1 && args[0] == "-version" {
		_, _ = fmt.Fprintln(stderr, `openjdk version "21.0.5" 2024-10-15`)
		return 0
	}
	if log := getenv(LogEnv); log != "" {
		if err := appendCall(log, args); err != nil {
			_, _ = fmt.Fprintf(stderr, "fakejava: %v\n", err)
			return 2
		}
	}
	if getenv(RefuseEnv) != "" {
		_, _ = fmt.Fprintln(stderr, "fakejava: validation refused")
		return 3
	}
	jar, ver, out := -1, -1, ""
	for i, a := range args {
		switch {
		case a == "-jar" && jar < 0:
			jar = i
		case a == "-version" && ver < 0 && jar >= 0:
			ver = i
		case a == "-output" && i+1 < len(args):
			out = args[i+1]
		}
	}
	if jar < 0 || ver < 0 || ver < jar+2 || out == "" {
		_, _ = fmt.Fprintf(stderr, "fakejava: unsupported call %q\n", args)
		return 2
	}
	_, _ = fmt.Fprintln(stdout, "FHIR Validation tool Version 7.0.5 (fake)")
	inputs := args[jar+2 : ver]
	if len(inputs) == 0 {
		_, _ = fmt.Fprintln(stderr, "fakejava: no inputs")
		return 2
	}
	oos := make([]map[string]any, 0, len(inputs))
	for _, in := range inputs {
		data, err := os.ReadFile(in) //nolint:gosec // inputs named by fhirlint under test
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "fakejava: %v\n", err)
			return 2
		}
		if bytes.Contains(data, []byte("FAKE_CRASH")) {
			_, _ = fmt.Fprintln(stdout, `Exception in thread "main" java.lang.IllegalStateException: fake crash`)
			return 1
		}
		oos = append(oos, outcome(string(data)))
	}
	var doc any = oos[0]
	if len(oos) > 1 {
		entries := make([]map[string]any, len(oos))
		for i, oo := range oos {
			entries[i] = map[string]any{"fullUrl": "file://" + inputs[i], "resource": oo} //nolint:gosec // oos has one entry per input
		}
		doc = map[string]any{"resourceType": "Bundle", "type": "collection", "entry": entries}
	}
	b, err := json.Marshal(doc)
	if err == nil {
		err = os.WriteFile(out, b, 0o600) //nolint:gosec // -output as fhirlint under test passed it
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "fakejava: %v\n", err)
		return 2
	}
	return 0
}

func appendCall(log string, args []string) error {
	f, err := os.OpenFile(log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // path set by the test
	if err != nil {
		return err
	}
	b, _ := json.Marshal(args)
	_, err = f.Write(append(b, '\n'))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

func outcome(text string) map[string]any {
	var issues []map[string]any
	if strings.Contains(text, "FAKE_ERROR") {
		issues = append(issues, issue("error", "invalid", "Fake error found", "FAKE_ERROR_ID", "Patient.name"))
	}
	if strings.Contains(text, "FAKE_WARNING") {
		issues = append(issues, issue("warning", "business-rule", "Fake warning found", "FAKE_WARNING_ID", "Patient"))
	}
	if len(issues) == 0 {
		issues = append(issues, map[string]any{"severity": "information", "code": "informational", "details": map[string]any{"text": "All OK"}})
	}
	return map[string]any{"resourceType": "OperationOutcome", "issue": issues}
}

func issue(sev, code, msg, id, expr string) map[string]any {
	return map[string]any{
		"severity":   sev,
		"code":       code,
		"details":    map[string]any{"text": msg},
		"expression": []string{expr},
		"extension": []map[string]any{
			{"url": "http://hl7.org/fhir/StructureDefinition/operationoutcome-message-id", "valueCode": id},
			{"url": "http://hl7.org/fhir/StructureDefinition/operationoutcome-issue-line", "valueInteger": 1},
			{"url": "http://hl7.org/fhir/StructureDefinition/operationoutcome-issue-col", "valueInteger": 2},
		},
	}
}
