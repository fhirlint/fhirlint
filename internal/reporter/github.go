package reporter

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fhirlint/fhirlint/internal/validator"
)

// GitHub writes findings as GitHub Actions workflow commands, which the runner
// turns into annotations shown inline in a pull request's "Files changed" view
// and in the job log.
//
// Unlike the other reporters this always writes to stdout: the runner parses
// the job's output stream, so a file destination would produce no annotations.
func GitHub(results []*validator.Result, minSeverity string) error {
	return writeGitHub(os.Stdout, results, minSeverity)
}

func writeGitHub(w io.Writer, results []*validator.Result, minSeverity string) error {
	for _, r := range results {
		for _, iss := range filterIssues(r.Issues, minSeverity) {
			if _, err := fmt.Fprintln(w, gitHubCommand(r.Filename, iss)); err != nil {
				return err
			}
		}
	}
	return nil
}

// gitHubCommand renders one issue as a single workflow-command line.
func gitHubCommand(filename string, iss validator.Issue) string {
	var b strings.Builder
	b.WriteString("::")
	b.WriteString(gitHubLevel(iss.Severity))

	var params []string
	if filename != "" {
		params = append(params, "file="+escapeGitHubProperty(filepath.ToSlash(filename)))
	}
	expression, line, col := parseLocationString(iss.Location)
	if line > 0 {
		params = append(params, fmt.Sprintf("line=%d", line))
		if col > 0 {
			params = append(params, fmt.Sprintf("col=%d", col))
		}
	}
	// The expression is where in the resource the issue sits. It is not a
	// workflow-command parameter, so it goes in the title where GitHub shows it
	// as the annotation heading.
	if title := gitHubTitle(iss, expression); title != "" {
		params = append(params, "title="+escapeGitHubProperty(title))
	}
	if len(params) > 0 {
		b.WriteString(" ")
		b.WriteString(strings.Join(params, ","))
	}
	b.WriteString("::")
	b.WriteString(escapeGitHubData(iss.Message))
	return b.String()
}

// gitHubTitle labels the annotation with the message id and, when available,
// the FHIRPath expression — the two things that make a finding actionable.
func gitHubTitle(iss validator.Issue, expression string) string {
	switch {
	case iss.MessageID != "" && expression != "":
		return iss.MessageID + " @ " + expression
	case iss.MessageID != "":
		return iss.MessageID
	default:
		return expression
	}
}

// gitHubLevel maps a FHIR issue severity onto the three annotation levels the
// runner understands. "fatal" has no counterpart and folds into error.
func gitHubLevel(severity string) string {
	switch severity {
	case "error", "fatal":
		return "error"
	case "warning":
		return "warning"
	default:
		return "notice"
	}
}

// escapeGitHubData escapes the message body of a workflow command. Without this
// a newline would end the command and the rest of the message would be printed
// as plain log output instead of becoming part of the annotation.
func escapeGitHubData(s string) string {
	s = strings.ReplaceAll(s, "%", "%25")
	s = strings.ReplaceAll(s, "\r", "%0D")
	s = strings.ReplaceAll(s, "\n", "%0A")
	return s
}

// escapeGitHubProperty escapes a command parameter value. Parameters are comma
// separated and colon terminated, so those need escaping on top of the message
// escapes — an unescaped comma in a filename would be read as a new parameter.
func escapeGitHubProperty(s string) string {
	s = escapeGitHubData(s)
	s = strings.ReplaceAll(s, ":", "%3A")
	s = strings.ReplaceAll(s, ",", "%2C")
	return s
}
