// Command rulescan runs fhirlint's own rules — the built-in lint rules
// (internal/lint) and the reference check (internal/refcheck) — over a corpus
// of real-world FHIR resources, and compares two such scans (#577).
//
// Unit tests only show that a rule does what its fixtures expect. A rule that
// fires on resources nobody chose for it is found by running it over data
// written by other people, and a change to a rule by diffing the findings of
// two builds over the same data. The validator is not involved: these rules
// run in-process on JSON, so the scan needs neither Java nor the JAR.
//
//	go run ./internal/tools/rulescan scan [-o FILE] CORPUS
//	go run ./internal/tools/rulescan diff [-n N] [-must FILE] A B
//	go run ./internal/tools/rulescan sample [-n N] [-seed S] -rule ID SCAN
//
// scan walks CORPUS/<dataset>/ (as unpacked by scripts/corpus-fetch.sh) and
// writes one JSON finding per line, sorted. Each dataset is one reference-check
// set, the way a directory run with --check-references treats its input, and
// its .rulescan-base file holds the base for canonical-url-pattern; without
// one that rule is skipped for the dataset. Every other rule runs everywhere.
//
// diff prints, per rule, the findings only A has (removed) and only B has
// (added). With -must it also fails when a finding in the must-stay-reported
// list is missing from B, so a false-positive fix cannot silently drop a true
// positive.
//
// sample draws N findings of one rule from distinct resources, for the
// precision step: judge each hit in context, true or false positive.
//
// scripts/rule-diff.sh builds this tool against two revisions and runs it.
// The tool is the measuring device, not the thing measured: the script
// always uses this revision's copy of it, also for the older build, so only
// the rules differ between A and B.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/fhirlint/fhirlint/internal/lint"
	"github.com/fhirlint/fhirlint/internal/refcheck"
)

// Finding is one line of a scan.
type Finding struct {
	Dataset  string `json:"dataset"`
	File     string `json:"file"` // relative to the dataset
	Rule     string `json:"rule"` // the message ID, e.g. lint:id-kebab-case, ref:unresolved
	Severity string `json:"severity"`
	Location string `json:"location"`
	Message  string `json:"message"`
}

func (f Finding) key() string {
	return strings.Join([]string{f.Dataset, f.File, f.Rule, f.Location, f.Message}, "\x00")
}

// baseFile names the per-dataset canonical base for canonical-url-pattern.
const baseFile = ".rulescan-base"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	switch args[0] {
	case "scan":
		return cmdScan(args[1:], stdout, stderr)
	case "diff":
		return cmdDiff(args[1:], stdout, stderr)
	case "sample":
		return cmdSample(args[1:], stdout, stderr)
	}
	usage(stderr)
	return 2
}

func usage(w io.Writer) {
	_, _ = fmt.Fprintln(w, `usage:
  rulescan scan [-o FILE] CORPUS
  rulescan diff [-n N] [-must FILE] A B
  rulescan sample [-n N] [-seed S] -rule ID SCAN`)
}

func cmdScan(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("scan", flag.ContinueOnError)
	flags.SetOutput(stderr)
	out := flags.String("o", "", "write the findings to FILE instead of stdout")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		usage(stderr)
		return 2
	}
	findings, err := scanCorpus(flags.Arg(0), stderr)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rulescan:", err)
		return 1
	}
	w := stdout
	if *out != "" {
		f, err := os.Create(*out) //nolint:gosec // output path from the command line
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "rulescan:", err)
			return 1
		}
		defer func() { _ = f.Close() }()
		w = f
	}
	if err := writeFindings(w, findings); err != nil {
		_, _ = fmt.Fprintln(stderr, "rulescan:", err)
		return 1
	}
	return 0
}

// scanCorpus scans every dataset directory directly under corpus.
func scanCorpus(corpus string, stderr io.Writer) ([]Finding, error) {
	entries, err := os.ReadDir(corpus)
	if err != nil {
		return nil, err
	}
	var all []Finding
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		found, files, err := scanDataset(filepath.Join(corpus, e.Name()), e.Name())
		if err != nil {
			return nil, err
		}
		n++
		_, _ = fmt.Fprintf(stderr, "%s: %d resources, %d findings\n", e.Name(), files, len(found))
		all = append(all, found...)
	}
	if n == 0 {
		return nil, fmt.Errorf("%s holds no dataset directories — run scripts/corpus-fetch.sh", corpus)
	}
	sortFindings(all)
	return all, nil
}

// engineFor enables every built-in rule. A rule that needs a parameter the
// dataset doesn't provide is left out: only "base" is known here.
func engineFor(base string) (*lint.Engine, error) {
	raw := map[string]interface{}{}
	for _, r := range lint.Rules() {
		params := map[string]interface{}{"severity": r.DefaultSev}
		ok := true
		for _, p := range r.RequiredParams {
			if p == "base" && base != "" {
				params["base"] = base
				continue
			}
			ok = false
		}
		if ok {
			raw[r.Name] = params
		}
	}
	cfg, err := lint.ParseConfig(raw)
	if err != nil {
		return nil, err
	}
	return lint.NewEngine(cfg)
}

type resource struct {
	rel string
	raw []byte
}

// scanDataset lints every JSON resource in dir and checks its references
// against the whole dataset. It returns the findings and the resource count.
func scanDataset(dir, name string) ([]Finding, int, error) {
	base := ""
	if b, err := os.ReadFile(filepath.Join(dir, baseFile)); err == nil { //nolint:gosec // corpus path from the command line
		base = strings.TrimSpace(string(b))
	}
	engine, err := engineFor(base)
	if err != nil {
		return nil, 0, err
	}

	var docs []resource
	index := refcheck.NewIndex()
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".json") {
			return nil
		}
		raw, err := os.ReadFile(path) //nolint:gosec // walking the corpus directory
		if err != nil {
			return err
		}
		if !isResource(raw) {
			return nil // package.json, .index.json, other non-resources
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		index.Add(raw)
		docs = append(docs, resource{rel: filepath.ToSlash(rel), raw: raw})
		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	var out []Finding
	for _, d := range docs {
		var issues []Finding
		if engine != nil {
			for _, iss := range engine.Evaluate(d.raw) {
				issues = append(issues, Finding{Rule: iss.MessageID, Severity: iss.Severity, Location: iss.Location, Message: iss.Message})
			}
		}
		found, _ := refcheck.CheckWithRefs(d.raw, index)
		for _, iss := range found {
			issues = append(issues, Finding{Rule: iss.MessageID, Severity: iss.Severity, Location: iss.Location, Message: iss.Message})
		}
		for _, f := range issues {
			f.Dataset, f.File = name, d.rel
			out = append(out, f)
		}
	}
	return out, len(docs), nil
}

// isResource reports whether raw is a JSON object with a resourceType.
func isResource(raw []byte) bool {
	var probe struct {
		ResourceType string `json:"resourceType"`
	}
	return json.Unmarshal(raw, &probe) == nil && probe.ResourceType != ""
}

func sortFindings(fs []Finding) {
	sort.Slice(fs, func(i, j int) bool { return fs[i].key() < fs[j].key() })
}

func writeFindings(w io.Writer, fs []Finding) error {
	bw := bufio.NewWriter(w)
	enc := json.NewEncoder(bw)
	enc.SetEscapeHTML(false)
	for _, f := range fs {
		if err := enc.Encode(f); err != nil {
			return err
		}
	}
	return bw.Flush()
}

func readFindings(path string) ([]Finding, error) {
	f, err := os.Open(path) //nolint:gosec // scan file from the command line
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []Finding
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for line := 1; sc.Scan(); line++ {
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		var fd Finding
		if err := json.Unmarshal(sc.Bytes(), &fd); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		out = append(out, fd)
	}
	return out, sc.Err()
}

func cmdDiff(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("diff", flag.ContinueOnError)
	flags.SetOutput(stderr)
	n := flags.Int("n", 12, "show up to N added and N removed findings per rule")
	must := flags.String("must", "", "must-stay-reported list; fail when B lacks one of its findings")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		usage(stderr)
		return 2
	}
	a, err := readFindings(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rulescan:", err)
		return 1
	}
	b, err := readFindings(flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rulescan:", err)
		return 1
	}
	printDiff(stdout, a, b, *n)
	if *must == "" {
		return 0
	}
	entries, err := readMustList(*must)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rulescan:", err)
		return 1
	}
	missing := missingMust(entries, b)
	for _, m := range missing {
		_, _ = fmt.Fprintf(stdout, "MISSING must-stay-reported: %s %s %s\n", m.Rule, m.File, m.Location)
	}
	if len(missing) > 0 {
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "must-stay-reported: all %d still reported\n", len(entries))
	return 0
}

// printDiff writes the per-rule counts and up to n added and removed findings.
func printDiff(w io.Writer, a, b []Finding, n int) {
	inA, inB := keySet(a), keySet(b)
	rules := map[string]bool{}
	countA, countB := map[string]int{}, map[string]int{}
	added, removed := map[string][]Finding{}, map[string][]Finding{}
	for _, f := range a {
		rules[f.Rule], countA[f.Rule] = true, countA[f.Rule]+1
		if !inB[f.key()] {
			removed[f.Rule] = append(removed[f.Rule], f)
		}
	}
	for _, f := range b {
		rules[f.Rule], countB[f.Rule] = true, countB[f.Rule]+1
		if !inA[f.key()] {
			added[f.Rule] = append(added[f.Rule], f)
		}
	}
	names := make([]string, 0, len(rules))
	for r := range rules {
		names = append(names, r)
	}
	sort.Strings(names)
	for _, r := range names {
		_, _ = fmt.Fprintf(w, "%s: A %d, B %d, added %d, removed %d\n", r, countA[r], countB[r], len(added[r]), len(removed[r]))
		for _, part := range []struct {
			tag string
			fs  []Finding
		}{{"+", added[r]}, {"-", removed[r]}} {
			for i, f := range part.fs {
				if i == n {
					_, _ = fmt.Fprintf(w, "  %s … %d more\n", part.tag, len(part.fs)-n)
					break
				}
				_, _ = fmt.Fprintf(w, "  %s %s/%s %s: %s\n", part.tag, f.Dataset, f.File, f.Location, mask(f.Message))
			}
		}
	}
}

func keySet(fs []Finding) map[string]bool {
	m := make(map[string]bool, len(fs))
	for _, f := range fs {
		m[f.key()] = true
	}
	return m
}

// tokenRe matches long runs that look like generated values: ids, hashes,
// UUIDs. The corpus is public example data, but the diff is meant to be pasted
// into pull requests, and a rule run over someone's own data must not leak it.
var tokenRe = regexp.MustCompile(`[A-Za-z0-9_+=-]{24,}`)

// mask shortens token-like values that mix letters and digits.
func mask(s string) string {
	s = tokenRe.ReplaceAllStringFunc(s, func(t string) string {
		if strings.ContainsAny(t, "0123456789") && strings.ContainsAny(t, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") {
			return t[:6] + "…"
		}
		return t
	})
	if len(s) > 160 {
		s = s[:160] + "…"
	}
	return s
}

// MustEntry is one line of the must-stay-reported list: a true positive in the
// corpus that every build has to keep reporting.
type MustEntry struct {
	Rule     string
	File     string // dataset/file
	Location string
}

// readMustList reads "rule<TAB>dataset/file<TAB>location" lines; # starts a
// comment, and every entry should say in one why it is a true positive.
func readMustList(path string) ([]MustEntry, error) {
	b, err := os.ReadFile(path) //nolint:gosec // list path from the command line
	if err != nil {
		return nil, err
	}
	var out []MustEntry
	for i, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 3 {
			return nil, fmt.Errorf("%s:%d: want rule, dataset/file and location, tab-separated", path, i+1)
		}
		out = append(out, MustEntry{Rule: parts[0], File: parts[1], Location: parts[2]})
	}
	if len(out) == 0 {
		return nil, errors.New(path + ": no entries")
	}
	return out, nil
}

func missingMust(entries []MustEntry, b []Finding) []MustEntry {
	have := map[MustEntry]bool{}
	for _, f := range b {
		have[MustEntry{Rule: f.Rule, File: f.Dataset + "/" + f.File, Location: f.Location}] = true
	}
	var out []MustEntry
	for _, e := range entries {
		if !have[e] {
			out = append(out, e)
		}
	}
	return out
}

func cmdSample(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("sample", flag.ContinueOnError)
	flags.SetOutput(stderr)
	n := flags.Int("n", 20, "number of findings to draw")
	seed := flags.Int64("seed", 1, "random seed, so a sample can be reproduced")
	rule := flags.String("rule", "", "message ID of the rule, e.g. lint:id-kebab-case")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 || *rule == "" {
		usage(stderr)
		return 2
	}
	fs, err := readFindings(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rulescan:", err)
		return 1
	}
	for _, f := range sample(fs, *rule, *n, *seed) {
		_, _ = fmt.Fprintf(stdout, "%s/%s %s: %s\n", f.Dataset, f.File, f.Location, f.Message)
	}
	return 0
}

// sample draws up to n findings of rule, at most one per resource, so one
// resource with many hits doesn't stand in for the whole corpus.
func sample(fs []Finding, rule string, n int, seed int64) []Finding {
	seen := map[string]bool{}
	var pool []Finding
	for _, f := range fs {
		res := f.Dataset + "/" + f.File
		if f.Rule != rule || seen[res] {
			continue
		}
		seen[res] = true
		pool = append(pool, f)
	}
	r := rand.New(rand.NewSource(seed)) //nolint:gosec // reproducible sampling, not security
	r.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	if len(pool) > n {
		pool = pool[:n]
	}
	sortFindings(pool)
	return pool
}
