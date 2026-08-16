// Package redact strips resource-derived content from validation results
// before they reach a reporter.
//
// Findings carry more of the validated resource than is obvious. The validator
// quotes offending values into its message text, and --show-source renders the
// offending line verbatim. On real patient data both are PHI, and several
// output paths outlive the run: SARIF is uploaded to GitHub code scanning and
// kept in the security tab's history, while JSON, HTML and JUnit reports are
// routinely archived as CI artifacts.
//
// The guarantee here is deliberately narrow and total rather than broad and
// best-effort: message text is removed outright, not filtered. Filtering would
// mean recognising every shape in which the validator embeds a value, across a
// message catalogue that is large and changes between releases — a check that
// is wrong occasionally is worse than none, because it is trusted.
package redact

import "github.com/fhirlint/fhirlint/internal/validator"

// Placeholder replaces the message text of a redacted finding. It is a visible
// marker rather than an empty string so that a redacted report reads as
// deliberately emptied instead of broken, and so formats that expect message
// text (SARIF, JUnit) stay well-formed.
const Placeholder = "[redacted]"

// Apply removes resource-derived content from results, in place.
//
// What survives is everything that describes the finding rather than the data:
// severity, the FHIRPath location, the message ID, the re-levelling marker, the
// suppression reason (which comes from the user's own config), and the file
// path. That is enough to act on a finding via `fhirlint explain <messageId>`.
func Apply(results []*validator.Result) {
	for _, r := range results {
		if r == nil {
			continue
		}
		redactIssues(r.Issues)
		redactIssues(r.Suppressed)

		// SourcePath is what a reporter resolves line/col against to render the
		// offending line. Clearing it makes a source snippet impossible rather
		// than merely switched off, so no future reporter can reintroduce the
		// leak by honouring --show-source without knowing about redaction.
		r.SourcePath = ""
	}
}

func redactIssues(issues []validator.Issue) {
	for i := range issues {
		issues[i].Message = Placeholder
		issues[i].Redacted = true
	}
}
