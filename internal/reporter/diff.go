package reporter

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/fhirlint/fhirlint/internal/diff"
	"github.com/fhirlint/fhirlint/internal/validator"
)

// diffJSON is the structured JSON shape emitted by DiffJSON.
type diffJSON struct {
	New       []diff.Issue `json:"new"`
	Resolved  []diff.Issue `json:"resolved"`
	Unchanged []diff.Issue `json:"unchanged"`
	Summary   diffSummary  `json:"summary"`
}

type diffSummary struct {
	New       int `json:"new"`
	Resolved  int `json:"resolved"`
	Unchanged int `json:"unchanged"`
}

// DiffTerminal renders a human-readable diff. Unchanged issues are summarised
// unless showUnchanged is set.
func DiffTerminal(d *diff.Result, showUnchanged bool) {
	fmt.Println(errorStyle.Render(fmt.Sprintf("New issues (%d)", len(d.New))))
	if len(d.New) == 0 {
		fmt.Println(dimStyle.Render("  (none)"))
	}
	for _, iss := range d.New {
		fmt.Println(diffLine(severityMark(iss.Severity), iss))
	}
	fmt.Println()

	fmt.Println(successStyle.Render(fmt.Sprintf("Resolved issues (%d)", len(d.Resolved))))
	if len(d.Resolved) == 0 {
		fmt.Println(dimStyle.Render("  (none)"))
	}
	for _, iss := range d.Resolved {
		fmt.Println(diffLine(successStyle.Render("✓     "), iss))
	}
	fmt.Println()

	fmt.Println(dimStyle.Render(fmt.Sprintf("Unchanged issues (%d)", len(d.Unchanged))))
	if showUnchanged {
		for _, iss := range d.Unchanged {
			fmt.Println(diffLine(dimStyle.Render("·     "), iss))
		}
	} else if len(d.Unchanged) > 0 {
		fmt.Println(dimStyle.Render("  (use --show-unchanged to list)"))
	}
	fmt.Println()

	fmt.Printf("Summary: %s · %s · %s\n",
		errorStyle.Render(fmt.Sprintf("%d new", len(d.New))),
		successStyle.Render(fmt.Sprintf("%d resolved", len(d.Resolved))),
		dimStyle.Render(fmt.Sprintf("%d unchanged", len(d.Unchanged))),
	)
}

func diffLine(mark string, iss diff.Issue) string {
	id := iss.MessageID
	if id == "" {
		id = "—"
	}
	loc := ""
	if iss.Location != "" {
		loc = dimStyle.Render(" @ " + iss.Location)
	}
	return fmt.Sprintf("  %s %s  %s%s", mark, fileStyle.Render(iss.File), id, loc)
}

func severityMark(severity string) string {
	switch severity {
	case "fatal":
		return fatalStyle.Render("✗ FATAL")
	case "error":
		return errorStyle.Render("✗ ERROR")
	case "warning":
		return warningStyle.Render("⚠ WARN ")
	default:
		return infoStyle.Render("ℹ INFO ")
	}
}

// DiffJSON writes the diff as structured JSON. When dest is empty it prints to stdout.
func DiffJSON(d *diff.Result, dest string) error {
	out := diffJSON{
		New:       d.New,
		Resolved:  d.Resolved,
		Unchanged: d.Unchanged,
		Summary: diffSummary{
			New:       len(d.New),
			Resolved:  len(d.Resolved),
			Unchanged: len(d.Unchanged),
		},
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if dest == "" {
		fmt.Print(string(data))
		return nil
	}
	return os.WriteFile(dest, data, 0600)
}

// DiffSARIF writes only the new issues as a SARIF report, so GitHub Code
// Scanning surfaces just the regressions a change introduced — pre-existing
// issues are excluded. It reuses the standard SARIF builder.
func DiffSARIF(d *diff.Result, fhirlintVersion, dest string) error {
	byFile := map[string]*validator.Result{}
	order := []string{}
	for _, iss := range d.New {
		r, ok := byFile[iss.File]
		if !ok {
			r = &validator.Result{Filename: iss.File, Label: iss.File}
			byFile[iss.File] = r
			order = append(order, iss.File)
		}
		r.Issues = append(r.Issues, validator.Issue{
			Severity:  iss.Severity,
			Message:   iss.Message,
			Location:  iss.Location,
			MessageID: iss.MessageID,
		})
	}
	results := make([]*validator.Result, 0, len(order))
	for _, f := range order {
		results = append(results, byFile[f])
	}
	return SARIF(results, "information", fhirlintVersion, dest)
}
