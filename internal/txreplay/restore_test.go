// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package txreplay

import "testing"

// The forms the validator quoted in a scan of German packages (#663).
func TestRestoreURLs(t *testing.T) {
	const base = "http://127.0.0.1:40797"
	registry := &Manifest{Upstream: "https://tx.fhir.org/r4", Routing: RoutingRegistry, Ecosystem: "https://ecosystem.hl7.org/tx-reg"}
	single := &Manifest{Upstream: "https://tx.example.org/fhir/", Routing: RoutingSingleServer}
	for _, tc := range []struct {
		name, text string
		m          *Manifest
		want       string
	}{
		{"registry: the base is the origin", "Error from " + base + ": Error: not found", registry, "Error from https://tx.fhir.org: Error: not found"},
		{"registry: with the path", "Error from " + base + "/r4: Error: x", registry, "Error from https://tx.fhir.org/r4: Error: x"},
		{"a routed server", "Error from " + base + "/route/tx.fhir.de/fhir: Error: y", registry, "Error from https://tx.fhir.de/fhir: Error: y"},
		{"the registry", "asked " + base + "/tx-reg/resolve?x=1", registry, "asked https://ecosystem.hl7.org/tx-reg/resolve?x=1"},
		{"single server: the base is the upstream", "Error from " + base + ": z", single, "Error from https://tx.example.org/fhir: z"},
		{"another port is left alone", "Error from http://127.0.0.1:407971: z", registry, "Error from http://127.0.0.1:407971: z"},
		{"at the end of the text", "server " + base, single, "server https://tx.example.org/fhir"},
		{"no manifest: only route and registry", "Error from " + base + "/route/tx.fhir.de/fhir and " + base, nil, "Error from https://tx.fhir.de/fhir and " + base},
		{"nothing to do", "All OK", registry, "All OK"},
	} {
		if got := RestoreURLs(tc.text, base, tc.m); got != tc.want {
			t.Errorf("%s:\n got %q\nwant %q", tc.name, got, tc.want)
		}
	}
	if got := RestoreURLs("Error from "+base, "", registry); got != "Error from "+base {
		t.Errorf("without a base nothing changes: %q", got)
	}
}
