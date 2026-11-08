package reporter

import "github.com/fhirlint/fhirlint/internal/validator"

// MessageWithCause is the issue message with the cause the validator logged
// only on its console appended on a line of its own (#521), for the reports
// that have one text field per issue. The terminal reports print the cause as
// a line of their own; JSON carries it as the "cause" field.
//
// The cause comes last, so the length cap, which keeps the start and the end
// of a message, keeps it.
func MessageWithCause(iss validator.Issue) string {
	if iss.Cause == "" {
		return iss.Message
	}
	return iss.Message + "\nCause: " + iss.Cause
}
