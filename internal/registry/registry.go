// Package registry knows which FHIR package registries the validator consults,
// and in which order, so that the commands that ask a registry on their own —
// audit, coverage — reach the same answer the validator would.
//
// There are two hosts, and they are not equal. The validator's PackageServer
// lists packages2.fhir.org as PRIMARY_SERVER and packages.fhir.org as
// SECONDARY_SERVER; packages2 mirrors everything Simplifier publishes and
// additionally carries HL7's own pre-releases, which reach packages.fhir.org
// late or never (hl7.fhir.r6.core#6.0.0-snapshot1 was on packages2 for a week
// while packages.fhir.org answered 404). A 404 from the fallback alone is
// therefore not evidence that a package does not exist (#427).
package registry

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Primary is the registry the validator tries first.
const Primary = "https://packages2.fhir.org/packages"

// Secondary is the registry the validator falls back to. It is also the host
// iglock records in each lock entry's URL: that URL is documentation, and this
// is the stable, browsable form of it.
const Secondary = "https://packages.fhir.org"

// Default returns the registries in the order the validator tries them. A fresh
// slice each time, so a caller can append or reorder without touching the
// package-level order.
func Default() []string {
	return []string{Primary, Secondary}
}

// ErrNotFound means every registry answered 404. It is deliberately not
// returned when one registry answered 404 and another could not be reached: an
// unreachable registry says nothing about the package.
var ErrNotFound = errors.New("package not found in any registry")

// Get requests path from each registry in turn and returns the first 200
// response together with the registry that produced it. The caller owns the
// body.
//
// A 404 moves on to the next registry. So does any other failure — a transport
// error, a 5xx — but those are remembered: if no registry answers, the result
// is that failure rather than ErrNotFound, because only a 404 from every
// registry establishes that the package is missing.
func Get(ctx context.Context, client *http.Client, registries []string, path, accept string) (*http.Response, string, error) {
	if len(registries) == 0 {
		registries = Default()
	}
	if client == nil {
		client = http.DefaultClient
	}

	var failure error
	for _, base := range registries {
		endpoint := strings.TrimSuffix(base, "/") + "/" + path
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, "", err
		}
		if accept != "" {
			req.Header.Set("Accept", accept)
		}

		resp, err := client.Do(req)
		if err != nil {
			failure = fmt.Errorf("%s: %w", Host(base), err)
			continue
		}
		switch resp.StatusCode {
		case http.StatusOK:
			return resp, base, nil
		case http.StatusNotFound:
			_ = resp.Body.Close()
			continue
		default:
			_ = resp.Body.Close()
			failure = fmt.Errorf("%s: HTTP %d", Host(base), resp.StatusCode)
		}
	}
	if failure != nil {
		return nil, "", failure
	}
	return nil, "", ErrNotFound
}

// Host renders a registry URL the way a person names it — "packages2.fhir.org"
// rather than the full base URL with its path — for messages that list which
// registries were asked.
func Host(base string) string {
	s := strings.TrimPrefix(strings.TrimPrefix(base, "https://"), "http://")
	if i := strings.Index(s, "/"); i >= 0 {
		s = s[:i]
	}
	return s
}

// Hosts renders every registry with Host, joined for a message.
func Hosts(registries []string) string {
	if len(registries) == 0 {
		registries = Default()
	}
	hosts := make([]string, len(registries))
	for i, r := range registries {
		hosts[i] = Host(r)
	}
	return strings.Join(hosts, ", ")
}
