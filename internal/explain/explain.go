// Package explain provides built-in, offline explanations for common FHIR
// validation message IDs (core invariants) and remediation advice. It backs the
// `fhirlint explain` command and the inline hints in terminal output.
package explain

import (
	"fmt"
	"sort"
	"strings"
)

// Rule describes a single validation message ID.
type Rule struct {
	ID          string
	Title       string
	DefinedIn   string
	Description string
	HowToFix    string
}

// Lookup returns the rule for id (case-insensitive) and whether it is known.
func Lookup(id string) (Rule, bool) {
	r, ok := rules[strings.ToLower(strings.TrimSpace(id))]
	return r, ok
}

// Known reports whether a built-in explanation exists for id.
func Known(id string) bool {
	_, ok := rules[strings.ToLower(strings.TrimSpace(id))]
	return ok
}

// IDs returns the sorted list of message IDs that have a built-in explanation.
func IDs() []string {
	out := make([]string, 0, len(rules))
	for id := range rules {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Format renders a rule as the multi-line text shown by `fhirlint explain`.
func Format(r Rule) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s — %s\n", r.ID, r.Title)
	fmt.Fprintf(&b, "Defined in: %s\n\n", r.DefinedIn)
	b.WriteString(indent(r.Description))
	b.WriteString("\n\nHow to fix:\n")
	b.WriteString(indent(r.HowToFix))
	b.WriteString("\n\nSuppress if intentional:\n")
	fmt.Fprintf(&b, "  fhirlint validate ... --suppress messageId:%s\n", r.ID)
	b.WriteString("  # or in fhirlint.yml:\n")
	b.WriteString("  suppress:\n")
	fmt.Fprintf(&b, "    - messageId: %s\n", r.ID)
	return b.String()
}

// indent prefixes every non-empty line of s with two spaces.
func indent(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = "  " + l
		}
	}
	return strings.Join(lines, "\n")
}
