package validator

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// excerptCatalog loads the real 7.0.1 templates kept in testdata.
func excerptCatalog(t *testing.T) *messageCatalog {
	t.Helper()
	f, err := os.Open("testdata/messages-7.0.1-excerpt.properties")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	c, err := parseMessageCatalog(f)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCompileTemplate(t *testing.T) {
	cases := []struct {
		name, pattern, text string
		want                bool
	}{
		{"doubled quote is a literal quote", "ValueSet ''{0}'' not found anywhere", "ValueSet 'http://x/vs' not found anywhere", true},
		{"empty argument", "ValueSet ''{0}'' not found anywhere", "ValueSet '' not found anywhere", true},
		{"anchored at the end", "ValueSet ''{0}'' not found anywhere", "ValueSet 'x' not found anywhere, really", false},
		{"anchored at the start", "ValueSet ''{0}'' not found anywhere", "Note: ValueSet 'x' not found anywhere", false},
		{"formatted argument", "There are {0,number,integer} slices in this", "There are 12 slices in this", true},
		{"quoted braces are literal", "Use '{'{0}'}' in the template here", "Use {abc} in the template here", true},
		{"regex metacharacters are literal", "Found (a+b) in [{0}] at the end", "Found (a+b) in [x] at the end", true},
		{"argument spans lines", "Expression failed: {0} -- stop", "Expression failed: a\nb -- stop", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpl, ok := compileTemplate("X", tc.pattern)
			if !ok {
				t.Fatalf("compileTemplate(%q) rejected the pattern", tc.pattern)
			}
			if got := tmpl.re.MatchString(tc.text); got != tc.want {
				t.Errorf("match(%q) = %v, want %v (regex %s)", tc.text, got, tc.want, tmpl.re)
			}
		})
	}
}

func TestCompileTemplate_RejectsUnusablePatterns(t *testing.T) {
	for _, p := range []string{
		"{0}",                // matches anything
		"{0}: {1}",           // still almost nothing fixed
		"All OK",             // too little fixed text to be worth an id
		"Broken {0 template", // unbalanced brace
	} {
		if _, ok := compileTemplate("X", p); ok {
			t.Errorf("compileTemplate(%q) accepted a pattern it should reject", p)
		}
	}
}

func TestReadProperties(t *testing.T) {
	in := strings.Join([]string{
		"# comment",
		"! also a comment",
		"",
		"A = first value ",
		"B=no spaces",
		"C : colon separator",
		"D whitespace separator",
		`E = continued \`,
		`    over two lines`,
		`F = escaped é and \= and \\ backslash`,
		`G\:key = key with an escaped colon`,
	}, "\n")
	got, err := readProperties(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"A":     "first value ",
		"B":     "no spaces",
		"C":     "colon separator",
		"D":     "whitespace separator",
		"E":     "continued over two lines",
		"F":     `escaped é and = and \ backslash`,
		"G:key": "key with an escaped colon",
	}
	if len(got) != len(want) {
		t.Errorf("got %d properties, want %d: %v", len(got), len(want), got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestMessageCatalog_Infer(t *testing.T) {
	c := excerptCatalog(t)
	cases := []struct{ text, want string }{
		// The 7.0.1 forms from hapifhir/org.hl7.fhir.core#2708, which arrive without an id.
		{"A definition for CodeSystem 'http://fhir.de/CodeSystem/bfarm/atc' version '2026' could not be found, so the code cannot be validated. Valid versions: [2026]", "UNKNOWN_CODESYSTEM_VERSION"},
		{"A definition for CodeSystem 'http://fhir.de/CodeSystem/ifa/pzn' version '0.2' could not be found, so the code cannot be validated. Valid versions: []", "UNKNOWN_CODESYSTEM_VERSION"},
		{"Unable to check whether the code is in the value set '' because the code system http://fhir.de/CodeSystem/ifa/pzn|0.2 was not found", "UNABLE_TO_CHECK_IF_THE_PROVIDED_CODES_ARE_IN_THE_VALUE_SET_CS"},
		// The 6.10.x form of the same finding has its own template.
		{"A definition for CodeSystem 'http://fhir.de/CodeSystem/ifa/pzn' version '0.2' could not be found, so the code cannot be validated. No versions of this code system are known", "UNKNOWN_CODESYSTEM_VERSION_NONE"},
		{"A definition for CodeSystem 'http://fhir.de/CodeSystem/ask' could not be found, so the code cannot be validated", "UNKNOWN_CODESYSTEM"},
		// Surrounding whitespace does not matter.
		{"  ValueSet 'https://fhir.kbv.de/ValueSet/KBV_VS_SFHIR_KBV_NORMGROESSE' not found\n", "Terminology_TX_ValueSet_NotFound"},
		// Plural forms report under their base constant.
		{"Can't find 'Patient/1' in the bundle (entry 2). Note that there are 2 resources in the bundle with the same type and id, but they do not match because of the fullUrl based rules around matching relative references (must be 'http://a/Patient/1', but found these: x, y)", "BUNDLE_BUNDLE_ENTRY_NOTFOUND_APPARENT"},
		// Two templates with different ids fit: no id rather than a guess.
		{"The concept 'PKV' has a status of retired and inactive and its use should be reviewed", ""},
		// Too little fixed text in the template to say anything.
		{"All OK", ""},
		{"Something the validator never says", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := c.infer(tc.text); got != tc.want {
			t.Errorf("infer(%.70q) = %q, want %q", tc.text, got, tc.want)
		}
		// The memoised answer is the same.
		if got := c.infer(tc.text); got != tc.want {
			t.Errorf("second infer(%.70q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}

func TestMessageCatalog_NilInfersNothing(t *testing.T) {
	var c *messageCatalog
	if got := c.infer("ValueSet 'x' not found"); got != "" {
		t.Errorf("nil catalog inferred %q", got)
	}
}

// The raw OperationOutcome 7.0.1 produced for a kbv.basis#1.9.0 Medication
// example: 12 TerminologyEngine issues without an id, 4 with one.
func TestParseOutputWith_RecoversMissingIDs(t *testing.T) {
	data, err := os.ReadFile("testdata/oo-7.0.1-missing-message-ids.json")
	if err != nil {
		t.Fatal(err)
	}

	plain, err := parseOutput(data, []string{"med.json"}, "")
	if err != nil {
		t.Fatal(err)
	}
	empty := 0
	for _, is := range plain[0].Issues {
		if is.MessageID == "" {
			empty++
		}
		if is.MessageIDInferred {
			t.Errorf("without a catalog nothing is inferred, got %+v", is)
		}
	}
	if empty != 12 {
		t.Fatalf("fixture has %d id-less issues, want 12", empty)
	}

	results, err := parseOutputWith(data, []string{"med.json"}, "", excerptCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for i, is := range results[0].Issues {
		was := plain[0].Issues[i]
		if was.MessageID != "" {
			if is.MessageID != was.MessageID || is.MessageIDInferred {
				t.Errorf("a stated id changed: %q → %q (inferred %v)", was.MessageID, is.MessageID, is.MessageIDInferred)
			}
			continue
		}
		if !is.MessageIDInferred || is.MessageID == "" {
			t.Errorf("not recovered: %q", is.Message)
		}
		counts[is.MessageID]++
	}
	want := map[string]int{
		"UNKNOWN_CODESYSTEM_VERSION":                                    6,
		"UNABLE_TO_CHECK_IF_THE_PROVIDED_CODES_ARE_IN_THE_VALUE_SET_CS": 6,
	}
	for id, n := range want {
		if counts[id] != n {
			t.Errorf("%s inferred %d times, want %d (all: %v)", id, counts[id], n, counts)
		}
	}
}

func TestIssueJSON_MessageIDInferred(t *testing.T) {
	b, err := json.Marshal(Issue{MessageID: "X", MessageIDInferred: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"messageIdInferred":true`) {
		t.Errorf("inferred flag missing from %s", b)
	}
	b, _ = json.Marshal(Issue{MessageID: "X"})
	if strings.Contains(string(b), "messageIdInferred") {
		t.Errorf("a stated id must not carry the flag: %s", b)
	}
}

func writeMessagesJAR(t *testing.T, entries map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "validator_cli.jar")
	f, err := os.Create(path) //nolint:gosec // path is under t.TempDir()
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMessageCatalogFor(t *testing.T) {
	jar := writeMessagesJAR(t, map[string]string{
		"Messages.properties":    "Terminology_TX_ValueSet_NotFound = ValueSet ''{0}'' not found\n",
		"Messages_de.properties": "Terminology_TX_ValueSet_NotFound = ValueSet ''{0}'' nicht gefunden\n",
	})
	c := messageCatalogFor(jar)
	if c == nil {
		t.Fatal("no catalog from a JAR that has Messages.properties")
	}
	if got := c.infer("ValueSet 'x' not found"); got != "Terminology_TX_ValueSet_NotFound" {
		t.Errorf("infer = %q", got)
	}
	if c.infer("ValueSet 'x' nicht gefunden") != "" {
		t.Error("only the English bundle is the validator's output language")
	}
	if messageCatalogFor(jar) != c {
		t.Error("the catalog is not reused for the same JAR")
	}

	if messageCatalogFor(writeMessagesJAR(t, map[string]string{"other.txt": "x"})) != nil {
		t.Error("a JAR without Messages.properties must give no catalog")
	}
	if messageCatalogFor(filepath.Join(t.TempDir(), "missing.jar")) != nil {
		t.Error("a missing JAR must give no catalog")
	}
}
