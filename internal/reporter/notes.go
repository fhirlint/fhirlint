package reporter

import (
	"strings"

	"github.com/fhirlint/fhirlint/internal/validator"
)

// MessageWithNotes is the issue message with what fhirlint adds to it, each on
// a line of its own, for the reports that have one text field per issue: the
// cause the validator logged only on its console (#521) and the package that
// defines a canonical it could not find (#523). The terminal reports print
// them as lines of their own; JSON carries them as "cause" and "hint".
//
// The notes come last, so the length cap, which keeps the start and the end
// of a message, keeps them.
func MessageWithNotes(iss validator.Issue) string {
	msg := iss.Message
	if iss.Cause != "" {
		msg += "\nCause: " + iss.Cause
	}
	if h := HintText(iss.Hint); h != "" {
		msg += "\nHint: " + h
	}
	return msg
}

// HintText renders a package hint as one sentence, or "" for none.
func HintText(h *validator.PackageHint) string {
	if h == nil {
		return ""
	}
	if h.Package == "" {
		return h.Canonical + " is not in the local package cache (~/.fhir/packages); " +
			"find the package that defines it on https://registry.fhir.org and add it with --ig <id>#<version>"
	}
	var b strings.Builder
	b.WriteString("defined in " + h.Package)
	if len(h.Packages) > 1 {
		b.WriteString(" (also " + strings.Join(h.Packages[1:], ", ") + ")")
	}
	b.WriteString(": add --ig " + h.Package)
	if h.Alias != "" {
		b.WriteString(", or the built-in alias --profile " + h.Alias)
	}
	return b.String()
}
