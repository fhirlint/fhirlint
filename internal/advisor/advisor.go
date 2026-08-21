// Package advisor converts fhirlint suppression rules into the HL7 validator's
// advisor file format, the one `-advisor-file` reads.
//
// The point is portability, not speed. A project's accepted findings live in
// fhirlint.yml and are applied by fhirlint after the JAR has spoken, so the raw
// `java -jar validator_cli.jar` run someone does to reproduce a finding, and
// the IG Publisher build next to it, still report everything the project has
// already decided to accept. An advisor file is the one format all three read.
//
// The JSON advisor (JsonDrivenPolicyAdvisor upstream) implements exactly one
// hook, isSuppressMessageId: it filters messages by id and path. It does not
// change severities and it does not skip validation work, so exporting buys
// shared rules and nothing else.
//
// The conversion is lossy by construction — see Convert for what survives.
package advisor

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/fhirlint/fhirlint/internal/suppress"
)

// File is the document the validator reads. The format has exactly one key.
type File struct {
	Suppress []string `json:"suppress"`
}

// Dropped is a rule that could not be expressed as an advisor entry, with the
// reason. Every dropped rule is reported: silently exporting three of seven
// rules would be worse than not offering the export at all, because the file
// would look complete.
type Dropped struct {
	Rule   suppress.Rule
	Reason string
}

// Reasons a rule does not survive the conversion. They are exported so callers
// can group by cause rather than by string matching.
const (
	// ReasonConstraint covers `constraint: dom-6`. fhirlint matches the short
	// key against the "#dom-6" *suffix* of a URI message id; the advisor's id
	// matching is equality or a trailing-`*` prefix, and neither can express a
	// suffix. Exporting the bare key would produce a rule that never fires.
	ReasonConstraint = "the advisor matches message ids by prefix, not by the #constraint suffix fhirlint uses"

	// ReasonPattern covers `pattern:`. fhirlint matches the message text; the
	// advisor's only regex applies to the element path. Same syntax, different
	// subject, so there is nothing to translate.
	ReasonPattern = "the advisor has no message-text matching (its regex matches the path)"

	// ReasonSeverityFilter covers a rule narrowed with `severity:`. The advisor
	// has no severity concept, so exporting it would drop the filter and
	// suppress more than the project asked for.
	ReasonSeverityFilter = "the advisor has no severity concept, and exporting without the filter would suppress more than intended"

	// ReasonExpired covers a rule whose `expires` date has passed. It is no
	// longer suppressing anything here, so writing it into a file that has no
	// expiry mechanism would quietly bring it back to life.
	ReasonExpired = "the rule has expired, and the advisor format has no expiry"

	// ReasonUnknownType is a belt-and-braces case: suppress.Rule types are
	// validated at parse time, so this only fires if a new type is added
	// without teaching the converter about it.
	ReasonUnknownType = "unknown suppression type"
)

// Convert turns suppression rules into an advisor file, plus the rules that
// have no equivalent.
//
// What survives, and how:
//
//	messageId: X                → "X"
//	expression: Patient.name    → "*@Patient.name" and "*@Patient.name.*"
//
// The expression case needs both entries. fhirlint's expression rule matches
// the location and everything below it, while an advisor path matches only its
// own depth unless the last segment is `*` — so one entry covers the element
// and the other covers its descendants. `*` as the message id matches any id,
// which is what an expression rule means: everything reported at this path.
//
// Entries are emitted in rule order and de-duplicated, so two rules covering
// the same ground produce one entry and the file stays diffable.
func Convert(rules []suppress.Rule) (File, []Dropped) {
	return ConvertAt(rules, time.Now())
}

// ConvertAt is Convert with an explicit clock, so expiry is testable.
func ConvertAt(rules []suppress.Rule, now time.Time) (File, []Dropped) {
	var (
		out     File
		dropped []Dropped
		seen    = map[string]bool{}
	)

	add := func(entry string) {
		if seen[entry] {
			return
		}
		seen[entry] = true
		out.Suppress = append(out.Suppress, entry)
	}
	drop := func(r suppress.Rule, reason string) {
		dropped = append(dropped, Dropped{Rule: r, Reason: reason})
	}

	for _, r := range rules {
		if r.ExpiredAt(now) {
			drop(r, ReasonExpired)
			continue
		}
		// Checked before the type switch: the filter narrows every type, so a
		// `messageId` rule with a severity filter is no more exportable than a
		// `pattern` one.
		if r.Severity != "" {
			drop(r, ReasonSeverityFilter)
			continue
		}

		switch r.Type {
		case "messageId":
			add(r.Value)
		case "expression":
			add("*@" + r.Value)
			add("*@" + r.Value + ".*")
		case "constraint":
			drop(r, ReasonConstraint)
		case "pattern":
			drop(r, ReasonPattern)
		default:
			drop(r, ReasonUnknownType)
		}
	}

	// The validator reads the key with forceArray, so an empty list is valid
	// and means "suppress nothing". Marshalling nil would write `null`, which
	// is a different thing to read back, so normalise.
	if out.Suppress == nil {
		out.Suppress = []string{}
	}
	return out, dropped
}

// Marshal renders the file as the validator expects it: indented JSON with a
// trailing newline, so it reads well in a diff and in a text editor.
func Marshal(f File) ([]byte, error) {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("rendering advisor file: %w", err)
	}
	return append(data, '\n'), nil
}
