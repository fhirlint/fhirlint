package resultcache

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/fhirlint/fhirlint/internal/validator"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func defaultOpts() KeyOpts {
	return KeyOpts{
		FhirlintVersion:  "0.2.0",
		ValidatorVersion: "6.5.13",
		Options: validator.Options{
			FHIRVersion: "4.0.1",
			Profiles:    []string{"kbv-basis"},
			IGs:         []string{"kbv.basis#1.5.0"},
		},
	}
}

func TestKey_SameContentSameOpts(t *testing.T) {
	dir := t.TempDir()
	p1 := writeFile(t, dir, "a.json", `{"resourceType":"Patient"}`)
	p2 := writeFile(t, dir, "b.json", `{"resourceType":"Patient"}`)

	k1, err := Key(p1, defaultOpts())
	if err != nil {
		t.Fatalf("Key error: %v", err)
	}
	k2, err := Key(p2, defaultOpts())
	if err != nil {
		t.Fatalf("Key error: %v", err)
	}
	if k1 != k2 {
		t.Error("identical content + options should produce same key")
	}
}

func TestKey_DifferentContent(t *testing.T) {
	dir := t.TempDir()
	p1 := writeFile(t, dir, "a.json", `{"resourceType":"Patient"}`)
	p2 := writeFile(t, dir, "b.json", `{"resourceType":"Observation"}`)

	k1, _ := Key(p1, defaultOpts())
	k2, _ := Key(p2, defaultOpts())
	if k1 == k2 {
		t.Error("different content should produce different keys")
	}
}

func TestKey_DifferentFHIRVersion(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "a.json", `{"resourceType":"Patient"}`)

	opts1 := defaultOpts()
	opts2 := defaultOpts()
	opts2.Options.FHIRVersion = "5.0.0"

	k1, _ := Key(p, opts1)
	k2, _ := Key(p, opts2)
	if k1 == k2 {
		t.Error("different FHIR version should produce different keys")
	}
}

func TestKey_ProfileOrderIndependent(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "a.json", `{"resourceType":"Patient"}`)

	opts1 := defaultOpts()
	opts2 := defaultOpts()
	opts1.Options.Profiles = []string{"a", "b"}
	opts2.Options.Profiles = []string{"b", "a"}

	k1, _ := Key(p, opts1)
	k2, _ := Key(p, opts2)
	if k1 != k2 {
		t.Error("profile order should not affect key")
	}
}

func TestKey_MissingFile(t *testing.T) {
	_, err := Key("/nonexistent/file.json", defaultOpts())
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestGetPut_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	entry := Entry{
		CachedAt:        time.Now().UTC().Truncate(time.Second),
		FhirlintVersion: "0.2.0",
		Result: validator.Result{
			Label: "patient.json",
			Valid: true,
			Issues: []validator.Issue{
				{Severity: "warning", Message: "test", MessageID: "dom-6"},
			},
		},
	}

	if err := Put(dir, "abc123", entry); err != nil {
		t.Fatalf("Put error: %v", err)
	}

	got, err := Get(dir, "abc123")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if got.Result.Label != "patient.json" {
		t.Errorf("label = %q, want patient.json", got.Result.Label)
	}
	if !got.Result.Valid {
		t.Error("result should be valid")
	}
	if len(got.Result.Issues) != 1 {
		t.Errorf("issues = %d, want 1", len(got.Result.Issues))
	}
}

func TestGet_MissingEntry(t *testing.T) {
	dir := t.TempDir()
	_, err := Get(dir, "doesnotexist")
	if err == nil {
		t.Error("expected error for missing cache entry")
	}
}

func TestPut_CreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "cache")
	entry := Entry{FhirlintVersion: "0.2.0", Result: validator.Result{}}
	if err := Put(dir, "key1", entry); err != nil {
		t.Fatalf("Put should create missing dir: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Error("cache dir should have been created")
	}
}

func TestClear_RemovesEntries(t *testing.T) {
	dir := t.TempDir()
	entry := Entry{FhirlintVersion: "0.2.0", Result: validator.Result{}}
	_ = Put(dir, "key1", entry)
	_ = Put(dir, "key2", entry)

	n, err := Clear(dir)
	if err != nil {
		t.Fatalf("Clear error: %v", err)
	}
	if n != 2 {
		t.Errorf("Clear removed %d entries, want 2", n)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".json" {
			t.Errorf("unexpected file after Clear: %s", e.Name())
		}
	}
}

func TestClear_NonExistentDir(t *testing.T) {
	n, err := Clear("/nonexistent/dir")
	if err != nil {
		t.Errorf("Clear on non-existent dir should not error: %v", err)
	}
	if n != 0 {
		t.Errorf("Clear on non-existent dir should return 0")
	}
}

// A cache entry that cannot be removed must be reported. Silently counting only
// the successes produced "Removed 0 cached result(s)" over a directory that was
// still full, which reads as success (#316).
func TestClear_UnremovableEntryIsReported(t *testing.T) {
	// Windows enforces neither: Chmod there only toggles the read-only
	// attribute, and root is denied nothing.
	if runtime.GOOS == "windows" {
		t.Skip("Windows: Chmod does not deny writes to a directory")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root: directory modes do not deny access")
	}
	dir := t.TempDir()
	entry := Entry{FhirlintVersion: "0.2.0", Result: validator.Result{}}
	if err := Put(dir, "key1", entry); err != nil {
		t.Fatal(err)
	}
	// Removal needs write permission on the directory, not on the file.
	if err := os.Chmod(dir, 0500); err != nil { //nolint:gosec // a directory needs the execute bit; this is deliberately read-only, not writable
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) }) //nolint:gosec // restoring a temp directory, which needs the execute bit

	n, err := Clear(dir)
	if err == nil {
		t.Fatal("expected an error when an entry cannot be removed")
	}
	if n != 0 {
		t.Errorf("removed count = %d, want 0 — nothing was actually removed", n)
	}
	if !strings.Contains(err.Error(), "1 cache entry could not be removed") {
		t.Errorf("error should say how many failed, got: %v", err)
	}
}

// A directory whose name ends in .json is not a cache entry and must be left
// alone, without stopping the real entries from being cleared.
func TestClear_SkipsDirectoriesNamedLikeEntries(t *testing.T) {
	dir := t.TempDir()
	entry := Entry{FhirlintVersion: "0.2.0", Result: validator.Result{}}
	for _, k := range []string{"key1", "key2"} {
		if err := Put(dir, k, entry); err != nil {
			t.Fatal(err)
		}
	}
	sub := filepath.Join(dir, "key3.json")
	if err := os.Mkdir(sub, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "blocker"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	n, err := Clear(dir)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 2 {
		t.Errorf("removed %d, want both real entries", n)
	}
	if _, statErr := os.Stat(sub); statErr != nil {
		t.Error("a directory named like an entry must survive")
	}
}

func TestClear_CountsOnlyWhatItRemoved(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	n, err := Clear(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("removed = %d, want 0 — non-entry files are left alone", n)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "notes.txt")); statErr != nil {
		t.Error("Clear must not remove files that are not cache entries")
	}
}

// Pinning a code system to a different edition changes which codes validate, so
// the entry must not be shared. The fingerprint goes into the key rather than
// the path, so relocating the same file keeps the cache warm (#407).
func TestKey_ExpansionParametersChangeTheKey(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "patient.json")
	if err := os.WriteFile(file, []byte(`{"resourceType":"Patient"}`), 0600); err != nil {
		t.Fatal(err)
	}

	base := KeyOpts{FhirlintVersion: "1.12.1", Options: validator.Options{FHIRVersion: "4.0.1"}}
	pinned2026 := base
	pinned2026.ExpansionParameters = "aaaaaaaaaaaa"
	pinned2027 := base
	pinned2027.ExpansionParameters = "bbbbbbbbbbbb"

	none, err := Key(file, base)
	if err != nil {
		t.Fatal(err)
	}
	k2026, err := Key(file, pinned2026)
	if err != nil {
		t.Fatal(err)
	}
	k2027, err := Key(file, pinned2027)
	if err != nil {
		t.Fatal(err)
	}

	if k2026 == k2027 {
		t.Error("two different expansion-parameter files produced the same cache key")
	}
	if none == k2026 {
		t.Error("pinned and unpinned runs produced the same cache key")
	}

	again, err := Key(file, pinned2026)
	if err != nil {
		t.Fatal(err)
	}
	if again != k2026 {
		t.Error("the same fingerprint produced a different key on a second call")
	}
}

// The four options named in #417. Each one changes what the validator reports,
// and each was absent from the key, so two runs differing only in one of them
// shared an entry — the second inheriting the first one's findings and its exit
// code. --best-practice is the sharpest case: at `error` it flips Valid.
func TestKey_OptionsThatChangeTheResult(t *testing.T) {
	dir := t.TempDir()
	file := writeFile(t, dir, "patient.json", `{"resourceType":"Patient"}`)

	cases := []struct {
		name   string
		mutate func(*validator.Options)
	}{
		{"BestPractice", func(o *validator.Options) { o.BestPractice = "error" }},
		{"Jurisdiction", func(o *validator.Options) { o.Jurisdiction = "urn:iso:std:iso:3166#DE" }},
		{"DisplayIssuesAreWarnings", func(o *validator.Options) { o.DisplayIssuesAreWarnings = true }},
		{"Locale", func(o *validator.Options) { o.Locale = "de" }},

		// Called arguable in #417 and decided the same way: a run with no
		// terminology server finds fewer code problems, so it is a different
		// result and must not reuse the entry.
		{"NoTerminologyServer", func(o *validator.Options) { o.NoTerminologyServer = true }},
		{"TerminologyServer", func(o *validator.Options) { o.TerminologyServer = "https://tx.example.org" }},
		{"Offline", func(o *validator.Options) { o.Offline = true }},
		{"TxCache", func(o *validator.Options) { o.TxCache = "n/a" }},
		{"ExtraArgs", func(o *validator.Options) { o.ExtraArgs = []string{"-some-flag"} }},
		{"AllowExampleURLs", func(o *validator.Options) { o.AllowExampleURLs = true }},
		{"CodeSystemSizeLimit", func(o *validator.Options) { n := 10; o.CodeSystemSizeLimit = &n }},

		// Both cut a run short and return partial results, which is a different
		// answer rather than the same answer sooner.
		{"ValidationTimeout", func(o *validator.Options) { o.ValidationTimeout = 30 * time.Second }},
		{"MaxMessages", func(o *validator.Options) { o.MaxMessages = 50 }},
	}

	baseKey, err := Key(file, defaultOpts())
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := defaultOpts()
			tc.mutate(&opts.Options)
			k, err := Key(file, opts)
			if err != nil {
				t.Fatal(err)
			}
			if k == baseKey {
				t.Errorf("%s does not reach the cache key: a run with it set would be served the result of a run without it", tc.name)
			}
		})
	}
}

// An empty --validator-version means "whatever is installed", which `fhirlint
// update` changes without the command line changing. The effective version is
// what the key has to carry.
func TestKey_EffectiveValidatorVersionChangesTheKey(t *testing.T) {
	dir := t.TempDir()
	file := writeFile(t, dir, "patient.json", `{"resourceType":"Patient"}`)

	older := defaultOpts()
	older.ValidatorVersion = "6.5.13"
	newer := defaultOpts()
	newer.ValidatorVersion = "6.6.0"

	k1, err := Key(file, older)
	if err != nil {
		t.Fatal(err)
	}
	k2, err := Key(file, newer)
	if err != nil {
		t.Fatal(err)
	}
	if k1 == k2 {
		t.Error("a different JAR version must not share a cache entry")
	}
}

// The other half of the denylist: excluding a field is only correct if it
// genuinely cannot change the result, and these must not cost a cache hit.
func TestKey_ExcludedOptionsDoNotChangeTheKey(t *testing.T) {
	dir := t.TempDir()
	file := writeFile(t, dir, "patient.json", `{"resourceType":"Patient"}`)

	cases := []struct {
		name   string
		mutate func(*KeyOpts)
	}{
		// How long fhirlint waits before giving up, which yields an error
		// rather than a different verdict.
		{"Timeout", func(k *KeyOpts) { k.Options.Timeout = 5 * time.Minute }},
		// A debug log written beside the run.
		{"TxLog", func(k *KeyOpts) { k.Options.TxLog = "/tmp/tx.log" }},
		// Superseded by the resolved ValidatorVersion the key already carries.
		{"JARPath", func(k *KeyOpts) { k.Options.JARPath = "/opt/validator.jar" }},
		{"ValidatorVersion flag", func(k *KeyOpts) { k.Options.ValidatorVersion = "6.5.13" }},
		// Represented by fingerprints, so the paths must not leak in: moving a
		// file must not throw the cache away (#407).
		{"ExpansionParameters path", func(k *KeyOpts) { k.Options.ExpansionParameters = "/elsewhere/params.json" }},
		{"FHIRSettings path", func(k *KeyOpts) { k.Options.FHIRSettings = "/elsewhere/fhir-settings.json" }},
		{"POFiles paths", func(k *KeyOpts) { k.Options.POFiles = []string{"/elsewhere/de.po"} }},
	}

	baseKey, err := Key(file, defaultOpts())
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := defaultOpts()
			tc.mutate(&opts)
			k, err := Key(file, opts)
			if err != nil {
				t.Fatal(err)
			}
			if k != baseKey {
				t.Errorf("%s changed the cache key, costing a hit for something that cannot change the result", tc.name)
			}
		})
	}
}

// The file-valued options reach the key as content fingerprints.
func TestKey_FileFingerprintsChangeTheKey(t *testing.T) {
	dir := t.TempDir()
	file := writeFile(t, dir, "patient.json", `{"resourceType":"Patient"}`)

	cases := []struct {
		name string
		set  func(*KeyOpts, string)
	}{
		{"FHIRSettings", func(k *KeyOpts, fp string) { k.FHIRSettings = fp }},
		{"POFiles", func(k *KeyOpts, fp string) { k.POFiles = []string{fp} }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, b := defaultOpts(), defaultOpts()
			tc.set(&a, "aaaaaaaaaaaa")
			tc.set(&b, "bbbbbbbbbbbb")
			ka, err := Key(file, a)
			if err != nil {
				t.Fatal(err)
			}
			kb, err := Key(file, b)
			if err != nil {
				t.Fatal(err)
			}
			if ka == kb {
				t.Errorf("two different %s files produced the same cache key", tc.name)
			}
		})
	}
}

// A later .po file overrides an earlier one, so order is meaning, not noise.
func TestKey_POFileOrderMatters(t *testing.T) {
	dir := t.TempDir()
	file := writeFile(t, dir, "patient.json", `{"resourceType":"Patient"}`)

	a, b := defaultOpts(), defaultOpts()
	a.POFiles = []string{"aaaaaaaaaaaa", "bbbbbbbbbbbb"}
	b.POFiles = []string{"bbbbbbbbbbbb", "aaaaaaaaaaaa"}

	ka, err := Key(file, a)
	if err != nil {
		t.Fatal(err)
	}
	kb, err := Key(file, b)
	if err != nil {
		t.Fatal(err)
	}
	if ka == kb {
		t.Error("reordering --po files must not share a cache entry: a later file overrides an earlier one")
	}
}

// The key is derived from the whole validator.Options value, so a new option
// joins it without anyone remembering to add it. That is the point of the
// denylist, and it leaves exactly one thing automation cannot decide: whether a
// newly added field is one of the few that must be *excluded*, or one that
// needs a fingerprint rather than its raw value.
//
// This test fails on any change to the shape of validator.Options. When it
// does, make that decision, then update this list — do not update it reflexively
// to make the build green.
func TestKeyedOptions_EveryFieldHasBeenConsidered(t *testing.T) {
	considered := map[string]string{
		// In the key: each of these changes what the validator reports.
		"FHIRVersion":              "keyed",
		"Profiles":                 "keyed (sorted: order is not meaning)",
		"IGs":                      "keyed (sorted: order is not meaning)",
		"NoTerminologyServer":      "keyed",
		"TerminologyServer":        "keyed",
		"BestPractice":             "keyed",
		"TxCache":                  "keyed",
		"Locale":                   "keyed",
		"AllowExampleURLs":         "keyed",
		"AllowInsecureTx":          "keyed",
		"Jurisdiction":             "keyed",
		"DisplayIssuesAreWarnings": "keyed",
		"ExtraArgs":                "keyed (unsorted: passed to the JAR verbatim)",
		"Proxy":                    "keyed",
		"ValidationTimeout":        "keyed: returns partial results",
		"MaxMessages":              "keyed: returns partial results",
		"Offline":                  "keyed",
		"CodeSystemSizeLimit":      "keyed",

		// Excluded: cannot change what the validator reports.
		"Timeout": "excluded",
		"TxLog":   "excluded",

		// Excluded because KeyOpts carries them in a better form.
		"JARPath":             "excluded: KeyOpts.ValidatorVersion",
		"ValidatorVersion":    "excluded: KeyOpts.ValidatorVersion resolves the effective one",
		"ExpansionParameters": "excluded: KeyOpts carries the fingerprint",
		"FHIRSettings":        "excluded: KeyOpts carries the fingerprint",
		"POFiles":             "excluded: KeyOpts carries the fingerprints",
	}

	typ := reflect.TypeOf(validator.Options{})
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		if _, ok := considered[name]; !ok {
			t.Errorf("validator.Options.%s is new and nothing says whether it belongs in the cache key.\n"+
				"Decide: does it change what the validator reports? Then it is keyed by default and you need only add it here.\n"+
				"Is it a file path? It needs a fingerprint in KeyOpts, not its raw value.\n"+
				"Can it provably not change the result? Exclude it in keyedOptions. See #417.", name)
		}
		delete(considered, name)
	}
	for name := range considered {
		t.Errorf("validator.Options.%s is listed here but no longer exists — remove it from this list and from keyedOptions", name)
	}
}
