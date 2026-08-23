package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/fhirlint/fhirlint/internal/igaudit"
	"github.com/fhirlint/fhirlint/internal/iglock"
	"github.com/fhirlint/fhirlint/internal/validator"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"path/filepath"
	"sort"
)

var flagAuditFormat string

// igAuditTimeout bounds the whole IG package check, however many packages the
// lock file holds. Auditing must not become the slow command that people stop
// running in CI.
const igAuditTimeout = 30 * time.Second

// Exit codes. Outdated IG packages are a maintenance signal, while an advisory
// against the JAR is a security one, so they must not be indistinguishable to a
// script. Code 1 keeps its existing meaning for callers that already depend on
// it; the IG check gets its own code rather than widening that one.
const (
	exitJARIssue = 1
	exitIGIssue  = 2
)

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Check the validator JAR and locked IG packages for updates and known security advisories",
	Long: `Check the inputs that determine what a validation run produces:

  - the validator JAR: whether it is current, and whether any published
    security advisory affects the installed version
  - the IG packages recorded in fhirlint.lock: whether newer versions exist
    upstream, and whether any pin no longer resolves

The IG section is skipped when there is no fhirlint.lock in the current
directory. Write one with 'fhirlint validate --lock'.

Exit codes: 0 clean, 1 a JAR problem or an affecting advisory, 2 only IG
package findings. With --format json the exit code is always 0 so the report
can be parsed.`,
	RunE:         runAudit,
	SilenceUsage: true,
}

func init() {
	auditCmd.Flags().StringVarP(&flagAuditFormat, "format", "f", "terminal",
		"Output format: terminal, json")
	_ = auditCmd.RegisterFlagCompletionFunc("format", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"terminal", "json"}, cobra.ShellCompDirectiveNoFileComp
	})
}

// auditJSON is the machine-readable shape emitted by `audit --format json`,
// consumed by the JAR security-monitor workflow.
type auditJSON struct {
	CurrentVersion string              `json:"currentVersion"`
	LatestVersion  string              `json:"latestVersion"`
	Outdated       bool                `json:"outdated"`
	AdvisoryCount  int                 `json:"advisoryCount"`
	Affecting      []auditAdvisoryJSON `json:"affecting"`
	VersionError   string              `json:"versionError,omitempty"`
	AdvisoryError  string              `json:"advisoryError,omitempty"`

	// LockFile is the lock file the IG packages were read from, empty when
	// there was none. IGPackages is always an array so that consumers can index
	// it without a nil check.
	LockFile string `json:"lockFile,omitempty"`
	// IGSource names the file the packages came from — the lock file, or the
	// config when falling back to its ig: list.
	IGSource   string                  `json:"igSource,omitempty"`
	IGPackages []igaudit.PackageReport `json:"igPackages"`
	// IGUnpinned lists ig: entries that name no registry version and so could
	// not be checked. Always an array.
	IGUnpinned []string `json:"igUnpinned"`
	IGError    string   `json:"igError,omitempty"`
}

type auditAdvisoryJSON struct {
	GHSAID   string `json:"ghsaId"`
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
	URL      string `json:"url"`
}

func runAudit(_ *cobra.Command, _ []string) error {
	report := validator.Audit()
	igReport, igSrc, igErr := auditIGPackages()

	if flagAuditFormat == "json" {
		return printAuditJSON(report, igReport, igSrc, igErr)
	}

	jarIssues := printJARTerminal(report)
	igIssues := printIGTerminal(igReport, igSrc, igErr)

	fmt.Println()
	switch auditExitCode(jarIssues, igIssues) {
	case exitJARIssue:
		fmt.Fprintf(os.Stderr, "%d issue(s) found.\n", jarIssues+igIssues)
		os.Exit(exitJARIssue)
	case exitIGIssue:
		fmt.Fprintf(os.Stderr, "%d IG package issue(s) found.\n", igIssues)
		os.Exit(exitIGIssue)
	}
	fmt.Println("No issues found.")
	return nil
}

// auditExitCode maps the two halves of the audit onto an exit code. A JAR
// problem outranks an IG one: both can be true at once, and a script that only
// knows to react to the security-relevant code must still see it.
func auditExitCode(jarIssues, igIssues int) int {
	switch {
	case jarIssues > 0:
		return exitJARIssue
	case igIssues > 0:
		return exitIGIssue
	default:
		return 0
	}
}

// igSource is where the packages being audited came from. The two sources
// answer different questions, so which one was used is part of the result.
type igSource struct {
	// Label names the file for the user, empty when nothing was found.
	Label string
	// IDs are auditable "name#version" packages.
	IDs []string
	// Unpinned are entries naming no version — an unpinned IG cannot be checked
	// against the registry, and that is itself worth reporting.
	Unpinned []string
	// FromConfig marks the fallback, which covers only what the config declares
	// and not the transitive packages a lock file would.
	FromConfig bool
}

// resolveIGSource picks what to audit: the lock file when there is one, the
// config's ig: list otherwise.
//
// The lock file is preferred because it pins exactly what a run resolved to,
// transitive packages included. The config only names direct dependencies — but
// most projects have no lock file, and auditing what they did declare beats
// skipping the half of the audit they ran the command for (#364).
func resolveIGSource() (igSource, error) {
	lf, err := iglock.Read(iglock.LockFileName)
	if err != nil {
		return igSource{}, fmt.Errorf("reading %s: %w", iglock.LockFileName, err)
	}
	if lf != nil && len(lf.Packages) > 0 {
		src := igSource{Label: iglock.LockFileName}
		for id := range lf.Packages {
			src.IDs = append(src.IDs, id)
		}
		sort.Strings(src.IDs)
		return src, nil
	}

	igs := viper.GetStringSlice("ig")
	if len(igs) == 0 {
		return igSource{}, nil
	}
	src := igSource{Label: configFileLabel(), FromConfig: true}
	for _, ig := range igs {
		// ParseIGID rejects local paths and "latest" as well as bare names —
		// none of them name a registry version to check.
		if name, version := iglock.ParseIGID(ig); name != "" && version != "" {
			src.IDs = append(src.IDs, ig)
			continue
		}
		src.Unpinned = append(src.Unpinned, ig)
	}
	sort.Strings(src.IDs)
	sort.Strings(src.Unpinned)
	return src, nil
}

// configFileLabel names the config the ig: list came from.
func configFileLabel() string {
	if used := viper.ConfigFileUsed(); used != "" {
		return filepath.Base(used)
	}
	return "the config"
}

// auditIGPackages checks each package in the resolved source against the
// registry. Finding no source at all is not an error: the JAR half of the audit
// is still worth running.
func auditIGPackages() (igaudit.Report, igSource, error) {
	src, err := resolveIGSource()
	if err != nil {
		return igaudit.Report{}, igSource{}, err
	}
	if len(src.IDs) == 0 {
		return igaudit.Report{}, src, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), igAuditTimeout)
	defer cancel()

	return igaudit.Audit(ctx, igaudit.NewClient(), src.IDs), src, nil
}

// printJARTerminal renders the JAR half of the audit and returns how many
// issues it found.
func printJARTerminal(report validator.AuditReport) int {
	issues := 0

	fmt.Println("Validator JAR")
	fmt.Printf("  current:  %s\n", report.CurrentVersion)
	if report.VersionError != "" {
		fmt.Fprintf(os.Stderr, "  ! could not fetch latest version: %s\n", report.VersionError)
	} else {
		fmt.Printf("  latest:   %s\n", report.LatestVersion)
		switch {
		case report.CurrentVersion == "unknown":
			fmt.Fprintln(os.Stderr, "  ✗ JAR not installed — run: fhirlint update")
			issues++
		case report.IsOutdated:
			fmt.Fprintln(os.Stderr, "  ✗ outdated — run: fhirlint update")
			issues++
		default:
			fmt.Println("  ✓ up to date")
		}
	}

	fmt.Println()
	fmt.Println("Security advisories (hapifhir/org.hl7.fhir.core)")
	if report.AdvisoryError != "" {
		fmt.Fprintf(os.Stderr, "  ! could not reach advisory API: %s\n", report.AdvisoryError)
	} else if len(report.Advisories) == 0 {
		fmt.Println("  ✓ no published advisories found")
	} else {
		affecting := report.AffectingAdvisories()
		if len(affecting) == 0 {
			fmt.Printf("  ✓ %d advisory/advisories published, none affect your version (%s)\n",
				len(report.Advisories), report.CurrentVersion)
		} else {
			for _, a := range affecting {
				fmt.Fprintf(os.Stderr, "  ✗ [%s] %s (%s)\n     %s\n",
					a.Severity, a.Summary, a.GHSAID, a.HTMLURL)
				issues++
			}
			fmt.Printf("  (%d advisory/advisories total, %d not affecting your version)\n",
				len(report.Advisories), len(report.Advisories)-len(affecting))
		}
	}

	return issues
}

// printIGTerminal renders the IG package half of the audit and returns how many
// packages need attention. Packages that could not be checked are reported but
// not counted — an unreachable registry says nothing about the package.
func printIGTerminal(r igaudit.Report, src igSource, igErr error) int {
	fmt.Println()
	label := src.Label
	if label == "" {
		label = iglock.LockFileName
	}
	fmt.Printf("IG packages (%s)\n", label)

	if igErr != nil {
		fmt.Fprintf(os.Stderr, "  ! %s\n", igErr)
		return 0
	}
	if len(r.Packages) == 0 && len(src.Unpinned) == 0 {
		fmt.Printf("  – no %s and no ig: list — record your IG packages with: fhirlint validate --lock\n",
			iglock.LockFileName)
		return 0
	}

	width := 0
	for _, p := range r.Packages {
		width = max(width, len(p.Name))
	}
	for _, ig := range src.Unpinned {
		width = max(width, len(ig))
	}

	for _, p := range r.Packages {
		switch {
		case p.Error != "":
			fmt.Fprintf(os.Stderr, "  ! %-*s  %s — could not check: %s\n", width, p.Name, p.Version, p.Error)
		case p.NotFound:
			fmt.Fprintf(os.Stderr, "  ✗ %-*s  %s — not found in the registry\n", width, p.Name, p.Version)
		case p.VersionMissing:
			fmt.Fprintf(os.Stderr, "  ✗ %-*s  %s — the package exists, but the registry has no such version\n",
				width, p.Name, p.Version)
		case p.Deprecated:
			fmt.Fprintf(os.Stderr, "  ✗ %-*s  %s — deprecated upstream%s\n", width, p.Name, p.Version,
				deprecationSuffix(p.DeprecationNote))
		case p.Outdated:
			fmt.Fprintf(os.Stderr, "  ✗ %-*s  %s → %s available\n", width, p.Name, p.Version, p.Latest)
		case p.Differs:
			fmt.Fprintf(os.Stderr, "  ! %-*s  %s — registry latest is %s (versions not comparable)\n",
				width, p.Name, p.Version, p.Latest)
		case p.Ahead:
			fmt.Printf("  ✓ %-*s  %s — ahead of registry latest (%s)\n", width, p.Name, p.Version, p.Latest)
		default:
			fmt.Printf("  ✓ %-*s  %s — current\n", width, p.Name, p.Version)
		}
	}

	// An unpinned IG cannot be checked against the registry, and that is worth
	// seeing in an audit rather than silently dropping.
	for _, ig := range src.Unpinned {
		fmt.Fprintf(os.Stderr, "  ! %-*s  not pinned — no version to check\n", width, ig)
	}

	if n := r.Problems(); n > 0 {
		fmt.Printf("  (%d of %d package(s) need attention)\n", n, len(r.Packages))
	}
	if src.FromConfig {
		// The config names direct dependencies only; a lock file also pins what
		// they pull in.
		fmt.Printf("  note: from the ig: list — run 'fhirlint validate --lock' to pin transitive packages too\n")
	}
	return r.Problems()
}

func deprecationSuffix(note string) string {
	if note == "" {
		return ""
	}
	return ": " + note
}

// printAuditJSON emits the audit report as JSON and always exits 0, so callers
// (e.g. the security-monitor workflow) can parse the result rather than rely on
// an exit code that conflates "outdated" with "security advisory".
func printAuditJSON(report validator.AuditReport, igReport igaudit.Report, src igSource, igErr error) error {
	affecting := make([]auditAdvisoryJSON, 0)
	for _, a := range report.AffectingAdvisories() {
		affecting = append(affecting, auditAdvisoryJSON{
			GHSAID:   a.GHSAID,
			Severity: a.Severity,
			Summary:  a.Summary,
			URL:      a.HTMLURL,
		})
	}

	packages := igReport.Packages
	if packages == nil {
		packages = []igaudit.PackageReport{}
	}

	out := auditJSON{
		CurrentVersion: report.CurrentVersion,
		LatestVersion:  report.LatestVersion,
		Outdated:       report.IsOutdated,
		AdvisoryCount:  len(report.Advisories),
		Affecting:      affecting,
		VersionError:   report.VersionError,
		AdvisoryError:  report.AdvisoryError,
		IGPackages:     packages,
	}
	// LockFile keeps naming the lock file only, so the workflow that reads this
	// does not start seeing a config file in a field named lockFile. IGSource
	// carries the general answer (#364).
	if len(packages) > 0 && !src.FromConfig {
		out.LockFile = iglock.LockFileName
	}
	if src.Label != "" {
		out.IGSource = src.Label
	}
	out.IGUnpinned = src.Unpinned
	if out.IGUnpinned == nil {
		out.IGUnpinned = []string{}
	}
	if igErr != nil {
		out.IGError = igErr.Error()
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
