package stats

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Terminal renders the stats report as aligned, human-readable text.
func Terminal(r *Report) string {
	var lines []string

	lines = append(lines, "Resource types")
	lines = append(lines, histogram(typeRows(r.ResourceTypes))...)

	lines = append(lines, "", "Profiles declared")
	lines = append(lines, histogram(profileRows(r.Profiles))...)

	if r.Validation != nil {
		v := r.Validation
		lines = append(lines, "", "Validation summary")
		lines = append(lines, fmt.Sprintf("  Files  %d   Valid  %d (%s)   Warnings  %d   Errors  %d",
			v.Files, v.Valid, percent(v.Valid, v.Files), v.Warnings, v.Errors))
	}

	return strings.Join(lines, "\n") + "\n"
}

type row struct {
	label string
	count int
}

func typeRows(ts []TypeCount) []row {
	out := make([]row, len(ts))
	for i, t := range ts {
		out[i] = row{t.Type, t.Count}
	}
	return out
}

func profileRows(ps []ProfileCount) []row {
	out := make([]row, len(ps))
	for i, p := range ps {
		out[i] = row{p.Profile, p.Count}
	}
	return out
}

// histogram formats rows as "  <label padded>  <count>", aligning counts.
func histogram(rows []row) []string {
	if len(rows) == 0 {
		return []string{"  (none)"}
	}
	width := 0
	for _, r := range rows {
		if len(r.label) > width {
			width = len(r.label)
		}
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = fmt.Sprintf("  %-*s  %d", width, r.label, r.count)
	}
	return out
}

func percent(n, total int) string {
	if total == 0 {
		return "0%"
	}
	return fmt.Sprintf("%d%%", n*100/total)
}

// JSON writes the report as indented JSON. Empty dest writes to stdout.
func JSON(r *Report, dest string) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if dest == "" {
		_, err := os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(dest, data, 0600)
}
