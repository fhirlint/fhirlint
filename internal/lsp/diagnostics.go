package lsp

import (
	"strings"

	"github.com/fhirlint/fhirlint/internal/reporter"
	"github.com/fhirlint/fhirlint/internal/validator"
)

// diagnosticSource labels findings in the editor's problems view.
const diagnosticSource = "fhirlint"

// toDiagnostics converts a validation result into editor diagnostics against
// the given document text.
//
// Suppressed issues are deliberately left out: a suppression is a decision the
// project already made, and re-surfacing it in the editor would undo the point
// of having made it.
func toDiagnostics(res *validator.Result, text string) []diagnostic {
	if res == nil {
		return nil
	}
	lines := strings.Split(text, "\n")
	out := make([]diagnostic, 0, len(res.Issues))
	for _, iss := range res.Issues {
		expr, line, col := reporter.ParseLocation(iss.Location)
		out = append(out, diagnostic{
			Range:    rangeFor(lines, line, col),
			Severity: severityFor(iss.Severity),
			Code:     iss.MessageID,
			Source:   diagnosticSource,
			Message:  iss.Message,
			Data:     diagnosticData{MessageID: iss.MessageID, Expression: expr},
		})
	}
	return out
}

// rangeFor turns the validator's 1-based line/column into a zero-based LSP
// range. The validator reports a point, not a span, so the range runs from that
// point to the end of the line — a zero-width range is easy to miss in an
// editor, and highlighting the rest of the line is the closest honest
// approximation of "the problem is here".
//
// A missing position (line 0) anchors to the start of the document rather than
// being dropped: an issue the user cannot see is worse than one in the wrong
// place.
func rangeFor(lines []string, line, col int) textRange {
	if line <= 0 || line > len(lines) {
		return textRange{Start: position{0, 0}, End: position{0, lineLen(lines, 0)}}
	}
	idx := line - 1
	end := lineLen(lines, idx)
	start := col - 1
	// A column past the end of the line means the validator counted the
	// document differently than the buffer reads — it reports positions against
	// the resource as it parsed it, which need not match a reformatted or
	// partially edited buffer. Clamping to the line end would leave an empty
	// range that is invisible in the editor and cannot be hovered, so fall back
	// to marking the whole line.
	if start < 0 || start >= end {
		start = 0
	}
	return textRange{
		Start: position{Line: idx, Character: start},
		End:   position{Line: idx, Character: end},
	}
}

func lineLen(lines []string, idx int) int {
	if idx < 0 || idx >= len(lines) {
		return 0
	}
	return len([]rune(strings.TrimRight(lines[idx], "\r")))
}

func severityFor(sev string) int {
	switch sev {
	case "fatal", "error":
		return severityError
	case "warning":
		return severityWarning
	case "information":
		return severityInformation
	default:
		return severityHint
	}
}

// diagnosticAt returns the first diagnostic covering a position, which is what
// hover and code actions need.
func diagnosticAt(diags []diagnostic, pos position) (diagnostic, bool) {
	for _, d := range diags {
		if covers(d.Range, pos) {
			return d, true
		}
	}
	return diagnostic{}, false
}

func covers(r textRange, pos position) bool {
	if pos.Line < r.Start.Line || pos.Line > r.End.Line {
		return false
	}
	if pos.Line == r.Start.Line && pos.Character < r.Start.Character {
		return false
	}
	// The end column is exclusive in LSP, but a hover just past the last
	// character of the highlighted span still means "this finding".
	if pos.Line == r.End.Line && pos.Character > r.End.Character {
		return false
	}
	return true
}
