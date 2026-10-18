package validator

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// fuzzExcerptCatalog loads the real 7.0.1 templates kept in testdata.
func fuzzExcerptCatalog(f *testing.F) *messageCatalog {
	f.Helper()
	data, err := os.ReadFile("testdata/messages-7.0.1-excerpt.properties")
	if err != nil {
		f.Fatal(err)
	}
	c, err := parseMessageCatalog(bytes.NewReader(data))
	if err != nil {
		f.Fatal(err)
	}
	return c
}

// FuzzMessageCatalog builds a catalog from fuzzed .properties content and
// infers an id for a fuzzed text, against both that catalog and the real 7.0.1
// templates. The texts come from the validator and, through it, from
// terminology servers (#449), so their shape is not ours to rely on. An
// inferred id must be one the catalog actually holds.
func FuzzMessageCatalog(f *testing.F) {
	excerpt := fuzzExcerptCatalog(f)
	props, err := os.ReadFile("testdata/messages-7.0.1-excerpt.properties")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(props, "A definition for CodeSystem 'http://fhir.de/CodeSystem/bfarm/atc' version '2026' could not be found, so the code cannot be validated. Valid versions: [2026]")
	f.Add([]byte("A_KEY = a ''quoted'' value with {0} and '{literal}' and {1,number}\\\n  continued\nB_one=x\\u0041\\"), "a 'quoted' value with x and {literal} and 1 continued")
	f.Add([]byte("UNBALANCED = text {0 that never closes long enough"), "text")
	f.Add([]byte("! comment\n# comment\nK:{0}{1}{2}"), "")

	ids := func(c *messageCatalog) map[string]bool {
		m := map[string]bool{}
		for _, t := range c.templates {
			m[t.id] = true
		}
		return m
	}
	excerptIDs := ids(excerpt)

	f.Fuzz(func(t *testing.T, props []byte, text string) {
		if id := excerpt.infer(text); id != "" && !excerptIDs[id] {
			t.Errorf("inferred %q, which the 7.0.1 catalog does not hold", id)
		}
		c, err := parseMessageCatalog(bytes.NewReader(props))
		if err != nil {
			return // a line over the scanner's 1 MiB limit
		}
		if id := c.infer(text); id != "" && !ids(c)[id] {
			t.Errorf("inferred %q, which the catalog does not hold", id)
		}
	})
}

// FuzzParseOutput parses a fuzzed validator output, with the 7.0.1 catalog so
// that id recovery runs as well. A malformed document is an error, never a
// panic.
func FuzzParseOutput(f *testing.F) {
	c := fuzzExcerptCatalog(f)
	seeds, _ := filepath.Glob("../../testdata/fixtures/oo-*.json")
	seeds = append(seeds, "testdata/oo-7.0.1-missing-message-ids.json", "testdata/profile-fetch-errors.json")
	for _, p := range seeds {
		if data, err := os.ReadFile(p); err == nil { //nolint:gosec // fixed test fixtures
			f.Add(data)
		}
	}
	f.Add([]byte(`{"resourceType":"Bundle","entry":[{"fullUrl":"file:///a.json","resource":{"resourceType":"OperationOutcome"}},{}]}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		results, err := parseOutputWith(data, []string{"a.json"}, "", c)
		if err != nil {
			return
		}
		for _, r := range results {
			if r == nil {
				t.Fatal("a parsed output must not contain a nil result")
			}
			for _, is := range r.Issues {
				if is.MessageIDInferred && strings.TrimSpace(is.MessageID) == "" {
					t.Errorf("issue marked as inferred without an id: %+v", is)
				}
			}
		}
	})
}

// FuzzParseCompareMessages extracts the difference table from fuzzed
// comparison HTML. The JAR writes it, but it echoes element paths and texts
// from the profiles being compared, which come from third-party packages.
// The result is never nil, and every message has the severity the header row
// is skipped for lacking.
func FuzzParseCompareMessages(f *testing.F) {
	f.Add(messagesHTML(`<tr style="background-color: #ffcccc"><td>Error</td><td>StructureDefinition.version</td><td>Values for version differ: &#39;1.0.0&#39; vs &#39;2.0.0&#39;</td></tr>`))
	f.Add(messagesHTML(`<tr><td><b>Warning</b></td><td>Patient.<i>name</i></td><td>a &lt; b &amp;&amp; c</td></tr>`))
	f.Add(messagesHTML(""))
	f.Add(`<h3>Metadata</h3><table class="grid"></table>`)
	f.Add(`>Messages<<table`)
	f.Add(`>Messages< no table </table>`)

	f.Fuzz(func(t *testing.T, doc string) {
		msgs := parseCompareMessages(doc)
		if msgs == nil {
			t.Fatal("nil slice: callers range and marshal it, and JSON must say [] not null")
		}
		for _, m := range msgs {
			if m.Severity == "" || m.Severity != strings.ToLower(m.Severity) {
				t.Errorf("severity %q is empty or not lower-case", m.Severity)
			}
		}
	})
}

// FuzzParseFHIRPathOutput locates the result block in fuzzed JAR stdout. The
// block echoes values from the user's resource, so markers can appear inside
// it and anywhere else.
func FuzzParseFHIRPathOutput(f *testing.F) {
	f.Add(sampleFHIRPathOutput("Patient.name.given", "Erika,Maria,Eri"), "Patient.name.given")
	f.Add(sampleFHIRPathOutput("x", ""), "x")
	f.Add(sampleFHIRPathOutput("x", "Error evaluating FHIRPath expression.\norg.hl7.fhir.exceptions.PathEngineException: boom"), "x")
	f.Add(sampleFHIRPathOutput("x", "Error evaluating FHIRPath expression."), "x")
	f.Add("\x1b[0;39m ...evaluating x\nDone.", "x")
	f.Add(" ...evaluating x", "x")
	f.Add("Unable to parse command line arguments: -foo", "")

	f.Fuzz(func(t *testing.T, stdout, expr string) {
		r, err := parseFHIRPathOutput(stdout, expr, nil)
		if err != nil {
			return
		}
		if r == nil || r.Items == nil || r.Expression != expr {
			t.Errorf("result = %+v, want items for %q", r, expr)
		}
	})
}

// FuzzCrashedFile names the file a crashed run was on from fuzzed JAR stdout
// (#471). The stdout echoes paths and exception texts the user's files can
// influence. A file it names is one of the inputs, and the message is plain,
// bounded text, since it goes into every report format.
func FuzzCrashedFile(f *testing.F) {
	f.Add(crashLog)
	f.Add("  Validate /work/in/a-patient.json\n 00:00.294\n  Validate /work/in/b-crash.json\n")
	f.Add("  Validate /work/in/b-crash.json\n\n\n" + strings.Repeat("ä", 2*maxCrashMessageLen))
	f.Add("Validate \n")
	f.Add("  Validate ../in/./a-patient.json\nboom")

	paths := []string{"/work/in/a-patient.json", "/work/in/b-crash.json", "in/rel.json"}
	f.Fuzz(func(t *testing.T, stdout string) {
		idx, msg, ok := crashedFile(stdout, paths)
		if !ok {
			return
		}
		if idx < 0 || idx >= len(paths) {
			t.Fatalf("index %d is not one of %d inputs", idx, len(paths))
		}
		if msg == "" || utf8.RuneCountInString(msg) > maxCrashMessageLen+1 {
			t.Errorf("message has %d runes, want 1..%d", utf8.RuneCountInString(msg), maxCrashMessageLen+1)
		}
		for _, r := range msg {
			if unicode.IsControl(r) {
				t.Errorf("message carries control character %U: %q", r, msg)
				break
			}
		}
	})
}
