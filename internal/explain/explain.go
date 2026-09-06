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
	key := strings.ToLower(strings.TrimSpace(id))
	if target, ok := aliases[key]; ok {
		key = target
	}
	r, ok := rules[key]
	return r, ok
}

// Known reports whether a built-in explanation exists for id.
func Known(id string) bool {
	_, ok := Lookup(id)
	return ok
}

// aliases map a message id onto the rule that explains it.
//
// The validator words one situation several ways depending on what it knows —
// whether a version was named, whether any version is known, whether it was
// expanding a value set. The distinctions matter to the validator and not to
// the reader, who needs the same explanation in every case. Only the id printed
// in the output is any use to look up, so all of them have to resolve (#391).
var aliases = map[string]string{
	"unknown_codesystem_version":            "unknown_codesystem",
	"unknown_codesystem_version_unk":        "unknown_codesystem",
	"unknown_codesystem_version_none":       "unknown_codesystem",
	"unknown_codesystem_coding_not_checked": "unknown_codesystem",
	"unknown_codesystem_exp":                "unknown_codesystem",
	"unknown_codesystem_version_exp":        "unknown_codesystem",
	"unknown_codesystem_version_exp_none":   "unknown_codesystem",
}

// IDs returns the sorted list of message IDs that have a built-in explanation.
func IDs() []string {
	out := make([]string, 0, len(rules))
	for _, r := range rules {
		out = append(out, r.ID)
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
