// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

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
//	FAKE_CRASH    the JAR throws while validating that input: a Java
//	              exception after its "Validate" line, exit 1, no output (#632)
//	FAKE_CRASH_LOADING  the same, but while loading, before any input
//
//	FAKE_TX:<code>  with -tx URL, ask that terminology server about <code>
//	                (GET <tx>/CodeSystem/$validate-code?system=http://example.org/cs&code=<code>):
//	                an error FAKE_TX_UNKNOWN when it answers result false, and
//	                FAKE_TX_UNAVAILABLE when it doesn't answer 200 (#612)
//
//	FAKE_ONLY_IN:<s>     an error FAKE_ONLY_ID, only when the -jar path contains <s>
//	FAKE_GONE_IN:<s>     an error FAKE_GONE_ID, except when the -jar path contains <s>
//	FAKE_RENAMED_IN:<s>  a warning at Patient.gender: FAKE_OLD_ID, or FAKE_NEW_ID with
//	                     a reworded text when the -jar path contains <s>
//
// The last three let two validator releases disagree, for upgrade-check (#629).
//
// An input without a marker gets the validator's "All OK" information issue.
// Each input is logged as the validator logs it: "  Validate <path>" before,
// its time (" 00:00.012") after, which fhirlint reads for --slowest and to
// tell which file a crash happened on.
// Several inputs come back as a Bundle of OperationOutcomes, one per input in
// order, as the real validator writes them.
//
// "-jar JAR compare … -left L -right R -dest D" writes the comparison page the
// validator writes, D/sd-<L>-<R>.html, with a Messages table (#615): no row
// when L and R are the same, one error row when they differ, and no page at
// all (an "Unable to resolve" line and exit 1) when either names "unresolvable".
//
// "-jar JAR fhirpath EXPR FILE …" evaluates a dotted path the way the
// validator prints it (#630): the log, " ...evaluating EXPR", the items joined
// by commas (an empty line for none), "Done.", all coloured as 6.10 colours
// them. EXPR is a plain path from the resource type (Patient.name.given);
// one ending in "(" is a parse error, a FILE that isn't JSON a resource
// error, both as the validator's error block, and FAKE_CRASH in FILE a crash
// before evaluation.
//
// "-jar JAR server PORT" starts the validator's HTTP service instead (#611):
// it prints the ready banner, answers POST /validateResource with the
// OperationOutcome of the posted resource, and exits on POST /stop.
//
// Environment variables let a test look behind it:
//
//	FAKEJAVA_LOG            append each call's arguments to this file, one
//	                        JSON array per line
//	FAKEJAVA_REFUSE         answer "java -version" but fail every validation
//	                        run, to prove a result came from somewhere else
//	FAKEJAVA_SERVER_FAIL    in server mode, exit 1 before the ready banner
//	FAKEJAVA_SERVER_SILENT  in server mode, never print the ready banner
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// Environment variables a test sets for the fake.
const (
	LogEnv          = "FAKEJAVA_LOG"
	RefuseEnv       = "FAKEJAVA_REFUSE"
	ServerFailEnv   = "FAKEJAVA_SERVER_FAIL"
	ServerSilentEnv = "FAKEJAVA_SERVER_SILENT"
)

// serverReady is the line the validator prints once its HTTP service accepts
// requests; fhirlint waits for it.
const serverReady = "FHIR Validator HTTP Service started"

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
	if jar := index(args, "-jar"); jar >= 0 && jar+3 < len(args) && args[jar+2] == "server" {
		return serve(args[jar+3], stdout, stderr, getenv)
	}
	if jar := index(args, "-jar"); jar >= 0 && jar+4 < len(args) && args[jar+2] == "fhirpath" {
		return fhirpath(args[jar+3], args[jar+4], stdout)
	}
	if jar := index(args, "-jar"); jar >= 0 && jar+2 < len(args) && args[jar+2] == "compare" {
		return compare(args, stdout, stderr)
	}
	if getenv(RefuseEnv) != "" {
		_, _ = fmt.Fprintln(stderr, "fakejava: validation refused")
		return 3
	}
	jar, ver, out, tx := -1, -1, "", ""
	for i, a := range args {
		switch {
		case a == "-jar" && jar < 0:
			jar = i
		case a == "-version" && ver < 0 && jar >= 0:
			ver = i
		case a == "-output" && i+1 < len(args):
			out = args[i+1]
		case a == "-tx" && i+1 < len(args) && args[i+1] != "n/a":
			tx = strings.TrimRight(args[i+1], "/")
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
	datas := make([][]byte, len(inputs))
	for i, in := range inputs {
		data, err := os.ReadFile(in) //nolint:gosec // inputs named by fhirlint under test
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "fakejava: %v\n", err)
			return 2
		}
		if bytes.Contains(data, []byte("FAKE_CRASH_LOADING")) {
			_, _ = fmt.Fprintln(stdout, `Exception in thread "main" java.lang.IllegalStateException: fake crash while loading`)
			return 1
		}
		datas[i] = data
	}
	oos := make([]map[string]any, 0, len(inputs))
	for i, in := range inputs {
		data := datas[i]
		_, _ = fmt.Fprintf(stdout, "  Validate %s\n", in)
		if bytes.Contains(data, []byte("FAKE_CRASH")) {
			_, _ = fmt.Fprintln(stdout, `Exception in thread "main" java.lang.IllegalStateException: fake crash`)
			return 1
		}
		oo := outcome(string(data), args[jar+1])
		if tx != "" {
			addTerminologyIssues(oo, tx, string(data))
		}
		oos = append(oos, oo)
		_, _ = fmt.Fprintf(stdout, " 00:00.%03d\n", 10+i)
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

// versionMarker matches the markers whose finding depends on the JAR.
var versionMarker = regexp.MustCompile(`FAKE_(ONLY|GONE|RENAMED)_IN:([A-Za-z0-9.]+)`)

func outcome(text, jar string) map[string]any {
	var issues []map[string]any
	for _, m := range versionMarker.FindAllStringSubmatch(text, -1) {
		in := strings.Contains(jar, m[2])
		switch {
		case m[1] == "ONLY" && in:
			issues = append(issues, issue("error", "invalid", "Fake error of the new release", "FAKE_ONLY_ID", "Patient.birthDate"))
		case m[1] == "GONE" && !in:
			issues = append(issues, issue("error", "invalid", "Fake error of the old release", "FAKE_GONE_ID", "Patient.active"))
		case m[1] == "RENAMED" && in:
			issues = append(issues, issue("warning", "code-invalid", "The gender code is not in the value set here", "FAKE_NEW_ID", "Patient.gender"))
		case m[1] == "RENAMED":
			issues = append(issues, issue("warning", "code-invalid", "The gender code is not in the value set", "FAKE_OLD_ID", "Patient.gender"))
		}
	}
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

func index(args []string, want string) int {
	for i, a := range args {
		if a == want {
			return i
		}
	}
	return -1
}

// serve runs the stand-in for the validator's HTTP service on port.
func serve(port string, stdout, stderr io.Writer, getenv func(string) string) int {
	if getenv(ServerFailEnv) != "" {
		_, _ = fmt.Fprintln(stderr, "fakejava: server failed to start")
		return 1
	}
	ln, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "fakejava: %v\n", err)
		return 2
	}
	stop := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/validateResource", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if bytes.Contains(body, []byte("FAKE_CRASH")) {
			http.Error(w, "java.lang.IllegalStateException: fake crash", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/fhir+json")
		_ = json.NewEncoder(w).Encode(outcome(string(body), "")) // one release: the markers by JAR stay off
	})
	mux.HandleFunc("/stop", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		close(stop)
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	if getenv(ServerSilentEnv) == "" {
		_, _ = fmt.Fprintln(stdout, serverReady+" on port "+port)
	}
	<-stop
	_ = ln.Close()
	return 0
}

var txCode = regexp.MustCompile(`FAKE_TX:([A-Za-z0-9-]+)`)

// addTerminologyIssues asks the terminology server at tx about every
// FAKE_TX:<code> in text, as the validator checks a code against its server.
func addTerminologyIssues(oo map[string]any, tx, text string) {
	matches := txCode.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return
	}
	client := &http.Client{Timeout: 10 * time.Second}
	var found []map[string]any
	for _, m := range matches {
		u := tx + "/CodeSystem/$validate-code?" + url.Values{"system": {"http://example.org/cs"}, "code": {m[1]}}.Encode()
		resp, err := client.Get(u) //nolint:gosec,noctx // the terminology server fhirlint under test passed
		if err != nil || resp.StatusCode != http.StatusOK {
			if resp != nil {
				_ = resp.Body.Close()
			}
			found = append(found, issue("error", "exception", "Terminology server unavailable for code "+m[1]+" (error message = Error from "+tx+": no answer)", "FAKE_TX_UNAVAILABLE", "Patient"))
			continue
		}
		var params struct {
			Parameter []struct {
				Name         string `json:"name"`
				ValueBoolean bool   `json:"valueBoolean"`
			} `json:"parameter"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&params)
		_ = resp.Body.Close()
		ok := false
		for _, p := range params.Parameter {
			if p.Name == "result" {
				ok = p.ValueBoolean
			}
		}
		if !ok {
			found = append(found, issue("error", "code-invalid", "Unknown code "+m[1], "FAKE_TX_UNKNOWN", "Patient"))
		}
	}
	if len(found) == 0 {
		return
	}
	issues := oo["issue"].([]map[string]any)
	if len(issues) == 1 && issues[0]["severity"] == "information" {
		issues = nil
	}
	oo["issue"] = append(issues, found...)
}

// value returns the argument after flag, or "".
func value(args []string, flag string) string {
	if i := index(args, flag); i >= 0 && i+1 < len(args) {
		return args[i+1]
	}
	return ""
}

// compare stands in for the validator's profile comparison.
func compare(args []string, stdout, stderr io.Writer) int {
	left, right, dest := value(args, "-left"), value(args, "-right"), value(args, "-dest")
	if left == "" || right == "" || dest == "" {
		_, _ = fmt.Fprintf(stderr, "fakejava: unsupported compare call %q\n", args)
		return 2
	}
	for _, side := range []string{left, right} {
		if strings.Contains(side, "unresolvable") {
			_, _ = fmt.Fprintf(stdout, "Unable to resolve profile %s\n", side)
			return 1
		}
	}
	rows := ""
	if left != right {
		rows = "<tr><td>Error</td><td>Patient.name</td><td>Fake difference between " + html.EscapeString(left) + " and " + html.EscapeString(right) + "</td></tr>"
	}
	page := "<html><body><h3>Messages</h3><div><table class=\"grid\"><tr><th>Severity</th><th>Path</th><th>Description</th></tr>" +
		rows + "</table></div><h3>Metadata</h3></body></html>"
	name := "sd-" + safeName(left) + "-" + safeName(right) + ".html"
	if err := os.WriteFile(dest+"/"+name, []byte(page), 0o600); err != nil { //nolint:gosec // -dest as fhirlint under test passed it
		_, _ = fmt.Fprintf(stderr, "fakejava: %v\n", err)
		return 2
	}
	return 0
}

func safeName(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return '_'
	}, s)
}

// fhirpath answers the validator's fhirpath mode; see the package comment.
func fhirpath(expr, file string, stdout io.Writer) int {
	colour := func(s string) string { return "\x1b[32m" + s + "\x1b[0m" }
	_, _ = fmt.Fprintln(stdout, colour("FHIR Validation tool Version 7.0.5 (fake)"))
	data, err := os.ReadFile(file) //nolint:gosec // the file fhirlint under test passed
	if err != nil || bytes.Contains(data, []byte("FAKE_CRASH")) {
		_, _ = fmt.Fprintln(stdout, "Error: fake crash before evaluation")
		return 1
	}
	_, _ = fmt.Fprintln(stdout, colour(" ...evaluating "+expr))
	fail := func(msg string) int {
		_, _ = fmt.Fprintln(stdout, "Error evaluating FHIRPath expression.")
		_, _ = fmt.Fprintln(stdout, msg)
		_, _ = fmt.Fprintln(stdout, "\tat org.hl7.fhir.r5.fhirpath.FHIRPathEngine.parse(FHIRPathEngine.java:1)")
		_, _ = fmt.Fprintln(stdout, colour("Done. Times: Loading: 00:00.001. Max Memory = 1Gb"))
		return 1
	}
	if strings.HasSuffix(expr, "(") {
		return fail("org.hl7.fhir.r5.fhirpath.FHIRLexer$FHIRLexerException: Error @1, " + fmt.Sprint(len(expr)+1) + ": Expression terminated unexpectedly")
	}
	var doc any
	if json.Unmarshal(data, &doc) != nil {
		return fail("java.io.IOException: Error parsing JSON source: JSON syntax error")
	}
	parts := strings.Split(expr, ".")
	if m, ok := doc.(map[string]any); ok && len(parts) > 0 && m["resourceType"] == parts[0] {
		parts = parts[1:]
	}
	items := []any{doc}
	for _, p := range parts {
		var next []any
		for _, it := range items {
			m, ok := it.(map[string]any)
			if !ok {
				continue
			}
			switch v := m[p].(type) {
			case nil:
			case []any:
				next = append(next, v...)
			default:
				next = append(next, v)
			}
		}
		items = next
	}
	out := make([]string, len(items))
	for i, it := range items {
		if s, ok := it.(string); ok {
			out[i] = s
		} else {
			b, _ := json.Marshal(it)
			out[i] = string(b)
		}
	}
	_, _ = fmt.Fprintln(stdout, strings.Join(out, ","))
	_, _ = fmt.Fprintln(stdout, colour("Done. Times: Loading: 00:00.001. Max Memory = 1Gb"))
	return 0
}
