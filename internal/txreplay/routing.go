// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package txreplay

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// Registry routing (#465).
//
// Without -tx, the validator asks a tx registry which terminology server is
// authoritative for a code system, and sends the request there: tx.fhir.de for
// the German SNOMED CT edition, for example. A recorder that is the
// validator's only -tx gets all requests instead, so a replay reproduced a
// single-server run that nobody validates live.
//
// From 7.0.4 the registry is a separate option, -ecosystem. With -tx pointing
// at the recorder and -ecosystem at its /tx-reg, the validator still routes:
//
//   - /tx-reg/* is forwarded to the real registry. In each answer, every
//     server other than tx.fhir.org is rewritten to /route/<host>/…, so the
//     validator sends those requests here too. tx.fhir.org needs no rewrite:
//     the validator substitutes its -tx for it in registry answers itself.
//   - /route/<host>/… is forwarded to https://<host>/…, but only to a host a
//     registry answer named during this run, so the recorder is never an
//     open proxy.
//
// The registry's answers are stored as they came and rewritten when served,
// because the recorder's and the player's ports differ. Replaying them pins
// the routing decisions as well as the answers: the registry changes over
// time, independently of the resources.
//
// Measured with 7.0.5 on the kbv.basis Medication from the issue: the
// validator still sends nocache=<timestamp> on every resolve, a candidate
// rewritten to the loopback recorder is contacted over plain HTTP, and
// tx.fhir.de, authoritative for the German SNOMED CT edition, was otherwise
// contacted directly.

// DefaultEcosystem is the tx registry validator 7.0.4+ uses by default.
const DefaultEcosystem = "https://ecosystem.hl7.org/tx-reg"

// RoutingRegistry is Manifest.Routing for a recording made with the tx
// registry in use, as a live run without --terminology-server has it.
const RoutingRegistry = "registry"

const (
	registryPrefix = "/tx-reg"
	routePrefix    = "/route/"
	// selfSubstitutedHost is the server the validator itself replaces with
	// its -tx in registry answers.
	selfSubstitutedHost = "tx.fhir.org"
)

// WithRegistry turns on registry routing. A recorder forwards /tx-reg to
// ecosystem (DefaultEcosystem when empty); a player serves the recorded
// registry answers. Either way the answers are rewritten to route through
// this server.
func (s *Server) WithRegistry(ecosystem string) *Server {
	if ecosystem == "" {
		ecosystem = DefaultEcosystem
	}
	s.ecosystem = strings.TrimSuffix(ecosystem, "/")
	s.routed = map[string]bool{}
	s.contacted = map[string]bool{}
	return s
}

// RoutedHosts are the servers other than tx.fhir.org that requests were routed
// to during this run, sorted. A registry answer names many candidates; this
// is the ones the validator actually asked.
func (s *Server) RoutedHosts() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.contacted))
	for h := range s.contacted {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

func isRegistryPath(p string) bool {
	return p == registryPrefix || strings.HasPrefix(p, registryPrefix+"/")
}

// registryQuery is the query a registry request is keyed by. The validator
// adds nocache=<timestamp> to every resolve; keyed with it, no registry
// answer would ever replay.
func registryQuery(raw string) string {
	if raw == "" {
		return ""
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return raw
	}
	values.Del("nocache")
	return values.Encode()
}

// rewriteRegistryAnswer points every https server in a registry answer other
// than tx.fhir.org at base/route/<host>/…, and returns the hosts it rewrote.
// An answer it cannot read is returned unchanged.
func rewriteRegistryAnswer(body []byte, base string) ([]byte, []string) {
	var doc map[string]any
	if json.Unmarshal(body, &doc) != nil {
		return body, nil
	}
	var hosts []string
	for _, field := range []string{"authoritative", "candidates"} {
		list, ok := doc[field].([]any)
		if !ok {
			continue
		}
		for _, item := range list {
			server, ok := item.(map[string]any)
			if !ok {
				continue
			}
			raw, ok := server["url"].(string)
			if !ok {
				continue
			}
			u, err := url.Parse(raw)
			if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil ||
				strings.EqualFold(u.Hostname(), selfSubstitutedHost) {
				continue
			}
			server["url"] = base + routePrefix + u.Host + u.EscapedPath()
			hosts = append(hosts, strings.ToLower(u.Host))
		}
	}
	if len(hosts) == 0 {
		return body, nil
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return body, nil
	}
	return out, hosts
}

// routeTarget splits /route/<host>/rest into the host and https://<host>/rest.
func routeTarget(p, rawQuery string) (host, target string, ok bool) {
	rest := strings.TrimPrefix(p, routePrefix)
	host, path, _ := strings.Cut(rest, "/")
	if host == "" {
		return "", "", false
	}
	target = "https://" + host + "/" + path
	if rawQuery != "" {
		target += "?" + rawQuery
	}
	return strings.ToLower(host), target, true
}

// serveRegistry answers a /tx-reg request: from the store when replaying,
// from the registry when recording. The stored answer is the registry's own;
// the rewrite happens on the way out.
func (s *Server) serveRegistry(w http.ResponseWriter, r *http.Request, body []byte) {
	query := registryQuery(r.URL.RawQuery)
	var in *Interaction
	if s.upstream == "" {
		found, ok := s.store.Lookup(r.Method, r.URL.Path, query, body)
		if !ok {
			s.miss(w, r, body)
			return
		}
		in = found
	} else {
		target := s.ecosystem + strings.TrimPrefix(r.URL.Path, registryPrefix)
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		// No upstream credentials: the registry is not the server they are for.
		fetched, ok := s.forward(w, r, body, target, map[string]string{})
		if !ok {
			return
		}
		fetched.Query = query
		if !s.keep(w, fetched, body) {
			return
		}
		in = fetched
	}
	out := *in
	if len(in.Response) > 0 {
		rewritten, hosts := rewriteRegistryAnswer(in.Response, s.base)
		out.Response = rewritten
		s.mu.Lock()
		for _, h := range hosts {
			s.routed[h] = true
		}
		s.mu.Unlock()
	}
	writeInteraction(w, &out)
}

// serveRoute answers a /route/<host>/… request. Recording forwards it to a
// host a registry answer named in this run, and nowhere else.
func (s *Server) serveRoute(w http.ResponseWriter, r *http.Request, body []byte) {
	if s.upstream == "" {
		s.replay(w, r, body)
		return
	}
	host, target, ok := routeTarget(r.URL.Path, r.URL.RawQuery)
	s.mu.Lock()
	allowed := ok && s.routed[host]
	s.mu.Unlock()
	if !allowed {
		writeOutcome(w, http.StatusForbidden, "not a server the tx registry named in this run: "+host)
		return
	}
	// No upstream credentials: they belong to the recording's own upstream,
	// not to the servers the registry names.
	in, ok := s.forward(w, r, body, target, map[string]string{})
	if !ok {
		return
	}
	s.mu.Lock()
	s.contacted[host] = true
	s.mu.Unlock()
	if !s.keep(w, in, body) {
		return
	}
	writeInteraction(w, in)
}

// SplitUpstream splits a terminology endpoint such as https://tx.fhir.org/r4
// into its origin and path, for recording with registry routing: the
// validator keeps the path when it substitutes its -tx for tx.fhir.org, so
// the path belongs on -tx and the recorder forwards to the origin.
func SplitUpstream(upstream string) (origin, path string) {
	u, err := url.Parse(strings.TrimSuffix(upstream, "/"))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return upstream, ""
	}
	return u.Scheme + "://" + u.Host, u.EscapedPath()
}
