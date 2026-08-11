package reporter

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/fhirlint/fhirlint/internal/explain"
	"github.com/fhirlint/fhirlint/internal/validator"
)

// maxGroupExamples bounds the file list under each group. Three is enough to
// recognise a pattern ("all my Patients") without the list becoming the wall of
// text grouping exists to remove; the count says how many were left out.
const maxGroupExamples = 3

// groupKey identifies findings that are the same problem.
//
// Severity is part of it because per-file overrides and severity-override rules
// can leave the same message at different levels in the same run, and merging
// those would report a level that is only true for some of the occurrences.
type groupKey struct {
	severity  string
	original  string
	messageID string
	message   string
}

// findingGroup is one distinct finding and everywhere it occurred.
type findingGroup struct {
	groupKey
	displayMessage string   // the message as first seen, before normalisation
	occurrences    int      // total findings in the group, matching the summary counts
	files          int      // distinct files, which is the number a reader acts on
	examples       []string // up to maxGroupExamples of "path" or "path:line"

	// firstSource locates the occurrence used for --show-source. Rendering a
	// snippet per occurrence would reproduce the noise being grouped away.
	firstSourcePath string
	firstLine       int
	firstCol        int
}

// TerminalGrouped prints one block per distinct finding instead of one block
// per file, for the run where the same finding repeats across a directory.
//
// It is presentation only: every occurrence is still counted, the summary and
// the exit code are computed from the same issues either way, and the
// machine-readable reporters are untouched.
func TerminalGrouped(results []*validator.Result, minSeverity string, showSuppressed, showSource bool) {
	groups := groupFindings(results, func(r *validator.Result) []validator.Issue {
		return filterIssues(r.Issues, minSeverity)
	})

	if len(groups) == 0 {
		fmt.Println(successStyle.Render("✓ No issues"))
	}
	for _, g := range groups {
		printGroup(g, showSource)
	}

	if showSuppressed {
		supp := groupFindings(results, func(r *validator.Result) []validator.Issue {
			return r.Suppressed
		})
		for _, g := range supp {
			fmt.Println(dimStyle.Render(fmt.Sprintf("↷ SUPP   %s  ·  %s", g.headline(), g.countLabel())))
			if g.messageID != "" {
				fmt.Println(dimStyle.Render("         " + g.displayMessage))
			}
			fmt.Println(dimStyle.Render("         " + g.exampleLine()))
		}
		if len(supp) > 0 {
			fmt.Println()
		}
	}
}

// groupFindings collects the issues each result contributes, as chosen by pick,
// into groups sorted most severe first and, within a severity, most frequent
// first — the order a reader works through them in.
func groupFindings(results []*validator.Result, pick func(*validator.Result) []validator.Issue) []findingGroup {
	index := map[groupKey]*findingGroup{}
	var order []groupKey

	for _, r := range results {
		seenInFile := map[groupKey]bool{}
		for _, issue := range pick(r) {
			key := groupKey{
				severity:  issue.Severity,
				original:  issue.OriginalSeverity,
				messageID: issue.MessageID,
				message:   normaliseMessage(issue.Message),
			}
			g, ok := index[key]
			if !ok {
				g = &findingGroup{groupKey: key, displayMessage: strings.TrimSpace(issue.Message)}
				index[key] = g
				order = append(order, key)
			}
			g.occurrences++
			if !seenInFile[key] {
				seenInFile[key] = true
				g.files++
				if len(g.examples) < maxGroupExamples {
					g.examples = append(g.examples, exampleLocation(r, issue))
				}
			}
			if g.firstSourcePath == "" {
				_, line, col := parseLocationString(issue.Location)
				g.firstSourcePath, g.firstLine, g.firstCol = r.SourcePath, line, col
			}
		}
	}

	out := make([]findingGroup, 0, len(index))
	for _, key := range order {
		out = append(out, *index[key])
	}
	sort.SliceStable(out, func(i, j int) bool {
		si, sj := severityRank(out[i].severity), severityRank(out[j].severity)
		if si != sj {
			return si > sj
		}
		if out[i].occurrences != out[j].occurrences {
			return out[i].occurrences > out[j].occurrences
		}
		return out[i].messageID < out[j].messageID
	})
	return out
}

func printGroup(g findingGroup, showSource bool) {
	prefix, style := severityPrefix(g.severity)
	fmt.Println(style.Render(prefix) + g.headline() + dimStyle.Render("  ·  "+g.countLabel()))
	// With no message ID the headline is already the message, so repeating it
	// here would just be the same line twice.
	if g.messageID != "" {
		fmt.Println("           " + g.displayMessage)
	}
	if g.original != "" {
		fmt.Println(dimStyle.Render("           ↕ reported as " + g.original))
	}
	fmt.Println(dimStyle.Render("           " + g.exampleLine()))
	if showSource {
		// One snippet, from the first occurrence: one per occurrence would put
		// back the noise this reporter exists to remove.
		for _, l := range sourceSnippet(g.firstSourcePath, g.firstLine, g.firstCol) {
			fmt.Println(dimStyle.Render("         " + l))
		}
	}
	if g.messageID != "" && explain.Known(g.messageID) {
		fmt.Println(dimStyle.Render("           ↳ Run: fhirlint explain " + g.messageID))
	}
	fmt.Println()
}

// headline is the message ID when there is one — the handle a reader searches,
// suppresses and explains by — and the message itself otherwise.
//
// Constraint IDs arrive as a full URI ("…/DomainResource#dom-6"). The header
// shows the fragment, which is both the readable name and exactly what a
// `constraint:` suppression matches on; the explain hint below still carries
// the full ID. Grouping keys use the full ID either way, so nothing merges that
// only shares a fragment.
func (g findingGroup) headline() string {
	if g.messageID == "" {
		return g.displayMessage
	}
	if _, fragment, found := strings.Cut(g.messageID, "#"); found && fragment != "" {
		return fragment
	}
	return g.messageID
}

// countLabel reports occurrences and files separately when they differ: one
// bundle can fail the same constraint many times, and "3 files" would then be
// a different number from the one in the summary line.
func (g findingGroup) countLabel() string {
	if g.occurrences == g.files {
		if g.files == 1 {
			return "1 file"
		}
		return fmt.Sprintf("%d files", g.files)
	}
	fileWord := "files"
	if g.files == 1 {
		fileWord = "file"
	}
	return fmt.Sprintf("%d occurrences in %d %s", g.occurrences, g.files, fileWord)
}

func (g findingGroup) exampleLine() string {
	line := strings.Join(g.examples, ", ")
	if remaining := g.files - len(g.examples); remaining > 0 {
		line += fmt.Sprintf("  … and %d more", remaining)
	}
	return line
}

// exampleLocation is the file, with the line number when the validator reported
// one, so an example can be opened directly.
func exampleLocation(r *validator.Result, issue validator.Issue) string {
	label := r.Label
	if label == "" {
		label = r.Filename
	}
	if _, line, _ := parseLocationString(issue.Location); line > 0 {
		return fmt.Sprintf("%s:%d", label, line)
	}
	return label
}

// normaliseMessage folds whitespace differences so the same finding wrapped or
// indented differently still groups. It deliberately keeps embedded values: two
// messages naming different codes are two different problems.
func normaliseMessage(msg string) string {
	return strings.Join(strings.Fields(msg), " ")
}

func severityRank(severity string) int {
	switch severity {
	case "fatal":
		return 3
	case "error":
		return 2
	case "warning":
		return 1
	default:
		return 0
	}
}

func severityPrefix(severity string) (string, lipgloss.Style) {
	switch severity {
	case "fatal":
		return "  ✗ FATAL  ", fatalStyle
	case "error":
		return "  ✗ ERROR  ", errorStyle
	case "warning":
		return "  ⚠ WARN   ", warningStyle
	default:
		return "  ℹ INFO   ", infoStyle
	}
}
