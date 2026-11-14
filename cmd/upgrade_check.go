// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fhirlint/fhirlint/internal/diff"
	"github.com/fhirlint/fhirlint/internal/reporter"
	"github.com/fhirlint/fhirlint/internal/termsafe"
	"github.com/fhirlint/fhirlint/internal/validator"
)

var upgradeCheckCmd = &cobra.Command{
	Use:   "upgrade-check --to <version> [--from <version>] [--json] <validate arguments...>",
	Short: "Show what a validator upgrade changes on your data before bumping the pin",
	Long: `upgrade-check validates the same input with two validator releases and
compares the findings: new, resolved and renamed issues per file, and the count
per message id before and after.

The "from" side is the validator validate would use (the pin or the installed
JAR), or --from. The "to" side is --to, downloaded and verified into its own
directory in the cache without becoming the default. Everything after the
upgrade-check flags is passed to validate unchanged, fhirlint.yml included, so
both runs use the same options.

Validation against a live terminology server can differ between two runs for
reasons that have nothing to do with the validator. Record the traffic with
'fhirlint tx warm' and pass --tx-offline for an exact comparison.

It costs two full validation runs. Exits 1 when the new version reports new
errors, so it can run as a scheduled job before the pin is bumped.`,
	Example: `  fhirlint upgrade-check --to 7.0.3 ./fhir/ --ig kbv.basis#1.10.0
  fhirlint upgrade-check --to 7.0.3 --from 6.10.3 ./fhir/ --tx-offline --tx-dir rec
  fhirlint upgrade-check --to 7.0.3 --json ./fhir/ > upgrade.json`,
	// The validate flags are passed through, so cobra must not reject them.
	DisableFlagParsing: true,
	SilenceUsage:       true,
	RunE:               runUpgradeCheck,
}

func init() {
	rootCmd.AddCommand(upgradeCheckCmd)
}

type upgradeCheckArgs struct {
	to, from    string
	json        bool
	passthrough []string
}

// upgradeCheckOwnFlags are flags upgrade-check sets on the validate runs
// itself; passing them through would make the two sides differ or write the
// report somewhere upgrade-check does not read it.
var upgradeCheckOwnFlags = []string{
	"--format", "-f", "--output", "-o", "--jar", "--validator-version",
	"--cache", "--lock", "--generate-baseline", "--watch", "-w",
}

func parseUpgradeCheckArgs(args []string) (upgradeCheckArgs, error) {
	var a upgradeCheckArgs
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, value, hasValue := strings.Cut(arg, "=")
		switch name {
		case "-h", "--help":
			return a, errUpgradeCheckHelp
		case "--to", "--from":
			if !hasValue {
				rest := args[i+1:]
				if len(rest) == 0 {
					return a, fmt.Errorf("%s needs a validator version", name)
				}
				value = rest[0]
				i++
			}
			if name == "--to" {
				a.to = value
			} else {
				a.from = value
			}
			continue
		case "--json":
			a.json = true
			continue
		}
		for _, own := range upgradeCheckOwnFlags {
			if name == own {
				return a, fmt.Errorf("%s is set by upgrade-check itself and cannot be passed through", own)
			}
		}
		a.passthrough = append(a.passthrough, arg)
	}
	if a.to == "" {
		return a, fmt.Errorf("--to <version> is required, e.g. --to 7.0.3")
	}
	return a, nil
}

var errUpgradeCheckHelp = fmt.Errorf("help requested")

func runUpgradeCheck(cmd *cobra.Command, args []string) error {
	a, err := parseUpgradeCheckArgs(args)
	if err == errUpgradeCheckHelp {
		return cmd.Help()
	}
	if err != nil {
		return &exitErr{code: 2, err: err}
	}

	toJAR, err := validator.EnsureSideJAR(a.to)
	if err != nil {
		return &exitErr{code: 2, err: err}
	}
	fromJAR := ""
	if a.from != "" {
		if fromJAR, err = validator.EnsureSideJAR(a.from); err != nil {
			return &exitErr{code: 2, err: err}
		}
	}

	if !usesRecordedTerminology(a.passthrough) {
		fmt.Fprintln(os.Stderr, "warn: the comparison runs against a live terminology server, whose answers can change between the two runs; "+
			"record them with 'fhirlint tx warm' and pass --tx-offline for an exact comparison")
	}

	dir, err := os.MkdirTemp("", "fhirlint-upgrade-check-")
	if err != nil {
		return &exitErr{code: 2, err: err}
	}
	defer func() { _ = os.RemoveAll(dir) }()

	fromReport, err := runValidateForUpgradeCheck("from", fromJAR, a.passthrough, filepath.Join(dir, "from.json"))
	if err != nil {
		return &exitErr{code: 2, err: err}
	}
	toReport, err := runValidateForUpgradeCheck("to", toJAR, a.passthrough, filepath.Join(dir, "to.json"))
	if err != nil {
		return &exitErr{code: 2, err: err}
	}

	report := compareUpgrade(fromReport, toReport)
	if a.json {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return &exitErr{code: 2, err: err}
		}
		fmt.Println(string(data))
	} else {
		printUpgradeReport(os.Stdout, report)
	}
	if report.NewErrors > 0 {
		return errValidationFailed
	}
	return nil
}

// usesRecordedTerminology reports whether the runs replay a recording or use
// no terminology server, so the two sides see the same answers.
func usesRecordedTerminology(args []string) bool {
	for _, a := range args {
		name, _, _ := strings.Cut(a, "=")
		if name == "--tx-offline" || name == "--no-terminology-server" {
			return true
		}
	}
	return false
}

// runValidateForUpgradeCheck runs this binary's validate with jar (empty: the
// pin or the installed JAR) and returns its JSON report. validate exits
// non-zero for findings, and possibly for a lock file that names the other
// validator; neither matters here as long as the report was written. Its
// stderr is shown only when it was not.
func runValidateForUpgradeCheck(side, jar string, passthrough []string, out string) (*reporter.JSONReport, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	args := append([]string{"validate"}, passthrough...)
	args = append(args, "--format", "json", "--output", out)
	if jar != "" {
		args = append(args, "--jar", jar)
	}
	fmt.Fprintf(os.Stderr, "Validating with the %q validator...\n", side)
	c := exec.Command(exe, args...) //nolint:gosec // this binary, with the user's own arguments
	var stderr bytes.Buffer
	c.Stdout = io.Discard
	c.Stderr = &stderr
	c.Env = append(os.Environ(), noUpdateNotifierEnvVar+"=1")
	runErr := c.Run()

	report, err := readReport(out)
	if err != nil {
		_, _ = os.Stderr.Write(stderr.Bytes())
		if runErr != nil {
			return nil, fmt.Errorf("validate with the %q validator wrote no report: %w", side, runErr)
		}
		return nil, fmt.Errorf("validate with the %q validator wrote no report: %w", side, err)
	}
	return report, nil
}

// upgradeReport is what upgrade-check reports, and its --json shape.
type upgradeReport struct {
	From      reporter.RunInfo `json:"from"`
	To        reporter.RunInfo `json:"to"`
	Files     []upgradeFile    `json:"files"`
	New       []diff.Issue     `json:"new"`
	Resolved  []diff.Issue     `json:"resolved"`
	Renamed   []renamedIssue   `json:"renamed"`
	ByID      []idCount        `json:"byMessageId"`
	NewErrors int              `json:"newErrors"`
}

type upgradeFile struct {
	File     string `json:"file"`
	New      int    `json:"new"`
	Resolved int    `json:"resolved"`
	Renamed  int    `json:"renamed"`
}

// renamedIssue is one finding at the same place with a new id: a resolved and
// a new issue, paired.
type renamedIssue struct {
	File     string     `json:"file"`
	Location string     `json:"location"`
	From     diff.Issue `json:"from"`
	To       diff.Issue `json:"to"`
}

type idCount struct {
	MessageID string `json:"messageId"`
	Before    int    `json:"before"`
	After     int    `json:"after"`
}

// compareUpgrade diffs the two reports with fhirlint diff's key (file, message
// id, location without line and column), inferred ids included (#449), and
// pairs a resolved and a new issue at the same place with similar text as a
// rename.
func compareUpgrade(from, to *reporter.JSONReport) upgradeReport {
	d := diff.Compute(from.Files, to.Files, "information")
	r := upgradeReport{From: metaOf(from), To: metaOf(to)}
	r.New, r.Resolved, r.Renamed = pairRenames(d.New, d.Resolved)

	files := map[string]*upgradeFile{}
	fileOf := func(name string) *upgradeFile {
		if files[name] == nil {
			files[name] = &upgradeFile{File: name}
		}
		return files[name]
	}
	for _, n := range r.New {
		fileOf(n.File).New++
		if n.Severity == "error" || n.Severity == "fatal" {
			r.NewErrors++
		}
	}
	for _, x := range r.Resolved {
		fileOf(x.File).Resolved++
	}
	for _, x := range r.Renamed {
		fileOf(x.File).Renamed++
	}
	for _, f := range files {
		r.Files = append(r.Files, *f)
	}
	sort.Slice(r.Files, func(i, j int) bool { return r.Files[i].File < r.Files[j].File })

	counts := map[string]*idCount{}
	count := func(results []*validator.Result, after bool) {
		for _, res := range results {
			for _, iss := range res.Issues {
				id := iss.MessageID
				if id == "" {
					id = "(no id)"
				}
				if counts[id] == nil {
					counts[id] = &idCount{MessageID: id}
				}
				if after {
					counts[id].After++
				} else {
					counts[id].Before++
				}
			}
		}
	}
	count(from.Files, false)
	count(to.Files, true)
	for _, c := range counts {
		if c.Before != c.After {
			r.ByID = append(r.ByID, *c)
		}
	}
	sort.Slice(r.ByID, func(i, j int) bool { return r.ByID[i].MessageID < r.ByID[j].MessageID })
	if r.New == nil {
		r.New = []diff.Issue{}
	}
	if r.Resolved == nil {
		r.Resolved = []diff.Issue{}
	}
	return r
}

// pairRenames moves a new and a resolved issue into a rename when they sit in
// the same file at the same place, have different ids and similar text. Each
// issue pairs at most once.
func pairRenames(newIssues, resolved []diff.Issue) (stillNew, stillResolved []diff.Issue, renamed []renamedIssue) {
	used := make([]bool, len(resolved))
	for _, n := range newIssues {
		paired := false
		for j, r := range resolved {
			if used[j] || r.File != n.File || r.MessageID == n.MessageID ||
				locationKey(r.Location) != locationKey(n.Location) || !similarText(r.Message, n.Message) {
				continue
			}
			used[j] = true
			renamed = append(renamed, renamedIssue{File: n.File, Location: n.Location, From: r, To: n})
			paired = true
			break
		}
		if !paired {
			stillNew = append(stillNew, n)
		}
	}
	for j, r := range resolved {
		if !used[j] {
			stillResolved = append(stillResolved, r)
		}
	}
	return stillNew, stillResolved, renamed
}

func locationKey(loc string) string {
	if i := strings.Index(loc, " (line "); i >= 0 {
		return loc[:i]
	}
	return loc
}

// similarText reports whether two messages share at least half of their
// words: a rewording, not a different finding.
func similarText(a, b string) bool {
	wa, wb := wordSet(a), wordSet(b)
	if len(wa) == 0 || len(wb) == 0 {
		return false
	}
	common := 0
	for w := range wa {
		if wb[w] {
			common++
		}
	}
	union := len(wa) + len(wb) - common
	return common*2 >= union
}

func wordSet(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		isWord := r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r > 127
		return !isWord
	}) {
		out[w] = true
	}
	return out
}

func printUpgradeReport(w io.Writer, r upgradeReport) {
	side := func(info reporter.RunInfo) string {
		if info.ValidatorBuild != "" {
			return info.ValidatorBuild
		}
		if info.Validator != "" {
			return info.Validator
		}
		return "unknown"
	}
	_, _ = fmt.Fprintf(w, "Validator upgrade check\n  from: %s\n  to:   %s\n\n", termsafe.Line(side(r.From)), termsafe.Line(side(r.To)))
	if len(r.New)+len(r.Resolved)+len(r.Renamed) == 0 {
		_, _ = fmt.Fprintln(w, "No change in the findings.")
		return
	}
	_, _ = fmt.Fprintln(w, "Per file:")
	for _, f := range r.Files {
		_, _ = fmt.Fprintf(w, "  %s  +%d new  -%d resolved  ~%d renamed\n", termsafe.Line(f.File), f.New, f.Resolved, f.Renamed)
	}
	section := func(title string, issues []diff.Issue) {
		if len(issues) == 0 {
			return
		}
		_, _ = fmt.Fprintf(w, "\n%s (%d):\n", title, len(issues))
		for _, i := range issues {
			_, _ = fmt.Fprintf(w, "  %-7s %s  %s @ %s\n", i.Severity, termsafe.Line(i.File), termsafe.Line(i.MessageID), termsafe.Line(i.Location))
			_, _ = fmt.Fprintf(w, "          %s\n", termsafe.Message(i.Message))
		}
	}
	section("New", r.New)
	section("Resolved", r.Resolved)
	if len(r.Renamed) > 0 {
		_, _ = fmt.Fprintf(w, "\nRenamed ids (%d):\n", len(r.Renamed))
		for _, x := range r.Renamed {
			_, _ = fmt.Fprintf(w, "  %s → %s  %s @ %s\n", termsafe.Line(x.From.MessageID), termsafe.Line(x.To.MessageID), termsafe.Line(x.File), termsafe.Line(x.Location))
		}
	}
	if len(r.ByID) > 0 {
		_, _ = fmt.Fprintln(w, "\nPer message id (changed counts only):")
		for _, c := range r.ByID {
			_, _ = fmt.Fprintf(w, "  %-50s %4d → %d\n", termsafe.Line(c.MessageID), c.Before, c.After)
		}
	}
	if r.NewErrors > 0 {
		_, _ = fmt.Fprintf(w, "\n%d new error(s) with the new validator.\n", r.NewErrors)
	}
}
