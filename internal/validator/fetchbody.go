package validator

import (
	"fmt"
	"net/url"
	"strings"
)

// maxFetchBody is the longest server response the validator's fetch errors
// may quote before it is cut. The informative bodies seen in practice, such as
// "Unparseable HTML Source (Unable to read attribute '-1' value on <input> at
// line 446 column 282)", stay well below it.
const maxFetchBody = 300

// fetchErrorMarker introduces the response body in the validator's
// EFhirClientException text: "<Exception>: Error from <url>: <body>".
const fetchErrorMarker = "Exception: Error from "

// trimFetchErrorBody shortens a message that quotes a whole server response.
//
// When a profile cannot be resolved the validator fetches its canonical and
// puts the client error into the message. For http://fhir.de/ canonicals the
// host answers with Simplifier's resolve page, and the validator embeds that
// page's text — scripts, footer, cookie dialog — about 7 KB of it (#445). It is
// already flattened to text by then, so it cannot be recognised by its tags;
// length is what separates it from a useful one-line reason.
func trimFetchErrorBody(msg string) string {
	i := strings.Index(msg, fetchErrorMarker)
	if i < 0 {
		return msg
	}
	start := i + len(fetchErrorMarker)
	// A URL holds no ": ", so the first one after the marker ends it.
	sep := strings.Index(msg[start:], ": ")
	if sep < 0 {
		return msg
	}
	source := msg[start : start+sep]
	bodyStart := start + sep + len(": ")
	body := msg[bodyStart:]
	if len(body) <= maxFetchBody {
		return msg
	}
	host := source
	if u, err := url.Parse(source); err == nil && u.Host != "" {
		host = u.Host
	}
	return fmt.Sprintf("%s(response from %s omitted, %d characters)", msg[:bodyStart], host, len(body))
}
