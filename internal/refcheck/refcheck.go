// Package refcheck performs referential-integrity checking across a set of FHIR
// resources: it indexes every resource's identity (ResourceType/id, Bundle entry
// fullUrls) and then reports literal references that do not resolve within the
// validated set. Contained (#id) references are resolved within their own
// resource. It is pure Go — no JVM round-trip.
//
// Findings are emitted as validator issues with message ID "ref:unresolved"
// (a local reference with no matching target, reported as an error) or
// "ref:external" (an absolute reference to a server outside the set, which
// cannot be checked, reported as information).
package refcheck

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/fhirlint/fhirlint/internal/validator"
)

// Message IDs and default severities for reference findings.
const (
	MsgUnresolved = "ref:unresolved"
	MsgExternal   = "ref:external"

	sevUnresolved = "error"
	sevExternal   = "information"
)

// resourceTypeRe matches a FHIR resource type token (UpperCamelCase letters).
var resourceTypeRe = regexp.MustCompile(`^[A-Z][A-Za-z]+$`)

// Index holds every resolvable reference target across a dataset.
type Index struct {
	typeIDs  map[string]struct{} // "Patient/123"
	fullURLs map[string]struct{} // "urn:uuid:…", "http://host/fhir/Patient/123"
}

// NewIndex returns an empty index.
func NewIndex() *Index {
	return &Index{typeIDs: map[string]struct{}{}, fullURLs: map[string]struct{}{}}
}

func (ix *Index) addTypeID(rt, id string) {
	if rt != "" && id != "" {
		ix.typeIDs[rt+"/"+id] = struct{}{}
	}
}

// Add indexes a resource's identity and, for Bundles, each entry's fullUrl and
// nested resource identity. Malformed JSON is ignored.
func (ix *Index) Add(resourceJSON []byte) {
	var root map[string]interface{}
	if err := json.Unmarshal(resourceJSON, &root); err != nil {
		return
	}
	rt, _ := root["resourceType"].(string)
	id, _ := root["id"].(string)
	ix.addTypeID(rt, id)

	if rt != "Bundle" {
		return
	}
	entries, ok := root["entry"].([]interface{})
	if !ok {
		return
	}
	for _, e := range entries {
		entry, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		if fu, ok := entry["fullUrl"].(string); ok && fu != "" {
			ix.fullURLs[fu] = struct{}{}
		}
		if r, ok := entry["resource"].(map[string]interface{}); ok {
			nrt, _ := r["resourceType"].(string)
			nid, _ := r["id"].(string)
			ix.addTypeID(nrt, nid)
		}
	}
}

// Check returns findings for every unresolved or external reference in the
// resource. Contained (#id) references are resolved within the resource. The
// result is sorted by location for deterministic output.
func Check(resourceJSON []byte, ix *Index) []validator.Issue {
	var root map[string]interface{}
	if err := json.Unmarshal(resourceJSON, &root); err != nil {
		return nil
	}
	rt, _ := root["resourceType"].(string)
	if rt == "" {
		rt = "(resource)"
	}
	var issues []validator.Issue
	walk(root, rt, nil, ix, &issues)

	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Location != issues[j].Location {
			return issues[i].Location < issues[j].Location
		}
		return issues[i].Message < issues[j].Message
	})
	return issues
}

// walk recursively visits node, collecting reference findings. contained is the
// set of contained resource ids in scope for the enclosing resource; it is
// recomputed whenever a nested resource (an object with a resourceType) is
// entered.
func walk(node interface{}, path string, contained map[string]struct{}, ix *Index, issues *[]validator.Issue) {
	switch n := node.(type) {
	case map[string]interface{}:
		if _, ok := n["resourceType"]; ok {
			contained = containedIDs(n)
		}
		for k, v := range n {
			if k == "reference" {
				if ref, ok := v.(string); ok {
					if iss, ok := classify(ref, path+".reference", contained, ix); ok {
						*issues = append(*issues, iss)
					}
					continue
				}
			}
			walk(v, joinPath(path, k), contained, ix, issues)
		}
	case []interface{}:
		for i, e := range n {
			walk(e, fmt.Sprintf("%s[%d]", path, i), contained, ix, issues)
		}
	}
}

// containedIDs returns the ids declared in a resource's contained array.
func containedIDs(res map[string]interface{}) map[string]struct{} {
	out := map[string]struct{}{}
	arr, ok := res["contained"].([]interface{})
	if !ok {
		return out
	}
	for _, c := range arr {
		if cm, ok := c.(map[string]interface{}); ok {
			if id, ok := cm["id"].(string); ok && id != "" {
				out[id] = struct{}{}
			}
		}
	}
	return out
}

// classify resolves a single literal reference against the index (and the
// contained scope) and returns an issue when it does not resolve.
func classify(ref, location string, contained map[string]struct{}, ix *Index) (validator.Issue, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return validator.Issue{}, false
	}

	// Contained reference: resolves within the enclosing resource.
	if strings.HasPrefix(ref, "#") {
		id := ref[1:]
		if id == "" {
			return validator.Issue{}, false // "#" refers to the container itself
		}
		if _, ok := contained[id]; ok {
			return validator.Issue{}, false
		}
		return issue(sevUnresolved, MsgUnresolved, location,
			fmt.Sprintf("contained reference %q has no matching contained resource", ref)), true
	}

	// urn: reference (urn:uuid / urn:oid) — resolves against a Bundle entry fullUrl.
	if strings.HasPrefix(ref, "urn:") {
		if _, ok := ix.fullURLs[ref]; ok {
			return validator.Issue{}, false
		}
		return issue(sevUnresolved, MsgUnresolved, location,
			fmt.Sprintf("unresolved reference %q (no matching entry fullUrl in the validated set)", ref)), true
	}

	// Absolute URL reference.
	if isAbsoluteURL(ref) {
		if _, ok := ix.fullURLs[ref]; ok {
			return validator.Issue{}, false
		}
		if tid, ok := trailingTypeID(ref); ok {
			if _, ok := ix.typeIDs[tid]; ok {
				return validator.Issue{}, false
			}
		}
		return issue(sevExternal, MsgExternal, location,
			fmt.Sprintf("reference %q points outside the validated set (external — not checked)", ref)), true
	}

	// Relative literal reference: Type/id (optionally /_history/…).
	tid, ok := relativeTypeID(ref)
	if !ok {
		return validator.Issue{}, false // not a recognisable literal reference (e.g. logical)
	}
	if _, ok := ix.typeIDs[tid]; ok {
		return validator.Issue{}, false
	}
	return issue(sevUnresolved, MsgUnresolved, location,
		fmt.Sprintf("unresolved reference %q (target not found in the validated set)", ref)), true
}

func issue(sev, msgID, location, msg string) validator.Issue {
	return validator.Issue{Severity: sev, Message: msg, Location: location, MessageID: msgID}
}

func isAbsoluteURL(ref string) bool {
	return strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://")
}

// stripHistory removes a trailing /_history/<version> segment.
func stripHistory(s string) string {
	if i := strings.Index(s, "/_history/"); i >= 0 {
		return s[:i]
	}
	return s
}

// relativeTypeID extracts "Type/id" from a relative reference, or reports false
// when ref is not a recognisable Type/id literal.
func relativeTypeID(ref string) (string, bool) {
	ref = stripHistory(ref)
	parts := strings.Split(ref, "/")
	if len(parts) < 2 {
		return "", false
	}
	rt, id := parts[0], parts[1]
	if !resourceTypeRe.MatchString(rt) || id == "" {
		return "", false
	}
	return rt + "/" + id, true
}

// trailingTypeID extracts the trailing "Type/id" from an absolute URL, or reports
// false when the URL does not end in a resource-type/id pair.
func trailingTypeID(u string) (string, bool) {
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		u = u[:i]
	}
	u = stripHistory(u)
	parts := strings.Split(strings.TrimRight(u, "/"), "/")
	if len(parts) < 2 {
		return "", false
	}
	rt, id := parts[len(parts)-2], parts[len(parts)-1]
	if !resourceTypeRe.MatchString(rt) || id == "" {
		return "", false
	}
	return rt + "/" + id, true
}

// joinPath appends a field to a JSON-ish path.
func joinPath(base, field string) string {
	if base == "" {
		return field
	}
	return base + "." + field
}
