package coverage

import (
	"reflect"
	"strings"
)

// node is one candidate value during a walk, together with where it came from.
type node struct {
	v any

	// companion marks a value taken from a FHIR primitive's sibling object —
	// the "_family" that carries id and extension for "family". Navigating
	// through it is required, because that is the only place an extension on a
	// primitive lives in JSON. Counting it as the primitive being populated is
	// not: a name whose family carries only an extension has no family value.
	companion bool
}

// Populated reports whether elementID has a value in the resource.
//
// The second return value is set when the answer could not be established —
// a slice whose members cannot be recognised. Such an element is neither
// covered nor uncovered, and reporting it as either would be a guess presented
// as a measurement.
func (r *Registry) Populated(sd *StructureDefinition, resource map[string]any, elementID string) (bool, string) {
	segs := parseElementID(elementID)
	if len(segs) < 2 {
		// The root segment alone is the resource itself, which is trivially
		// present and not a meaningful thing to measure.
		return true, ""
	}

	nodes := []node{{v: resource}}
	for i := 1; i < len(segs); i++ {
		seg := segs[i]
		nodes = step(nodes, seg.Name)

		if seg.Slice != "" {
			c := r.sliceCriteria(sd, joinSegments(segs[:i+1]))
			if len(c.tests) == 0 {
				return false, c.unresolvedReason
			}
			nodes = filterBySlice(nodes, c)
		}
		if len(nodes) == 0 {
			return false, ""
		}
	}

	for _, n := range nodes {
		if !n.companion && !isEmpty(n.v) {
			return true, ""
		}
	}
	return false, ""
}

// step advances every node by one field name, flattening arrays.
func step(nodes []node, name string) []node {
	var out []node
	for _, n := range nodes {
		obj, ok := n.v.(map[string]any)
		if !ok {
			continue
		}
		for _, key := range candidateKeys(obj, name) {
			appendValues(&out, obj[key], false)
		}
		// The primitive's companion object, which is where an extension on a
		// primitive is found.
		if v, ok := obj["_"+name]; ok {
			appendValues(&out, v, true)
		}
	}
	return out
}

func appendValues(out *[]node, v any, companion bool) {
	if v == nil {
		return
	}
	if arr, ok := v.([]any); ok {
		for _, item := range arr {
			if !isEmpty(item) {
				*out = append(*out, node{v: item, companion: companion})
			}
		}
		return
	}
	if !isEmpty(v) {
		*out = append(*out, node{v: v, companion: companion})
	}
}

// candidateKeys resolves an element name to the keys it can take in JSON.
//
// A choice element is written as "value[x]" in the profile and as the concrete
// type in the instance — valueQuantity, valueString and so on — so the name has
// to be matched by prefix rather than looked up directly.
func candidateKeys(obj map[string]any, name string) []string {
	if !strings.HasSuffix(name, "[x]") {
		if _, ok := obj[name]; ok {
			return []string{name}
		}
		return nil
	}

	prefix := strings.TrimSuffix(name, "[x]")
	var keys []string
	for k := range obj {
		if len(k) <= len(prefix) || !strings.HasPrefix(k, prefix) {
			continue
		}
		// The character after the prefix is the start of the type name, so it
		// is upper case. Without this check "value[x]" would also match a
		// sibling field such as "valueset".
		if c := k[len(prefix)]; c >= 'A' && c <= 'Z' {
			keys = append(keys, k)
		}
	}
	return keys
}

// filterBySlice keeps the nodes that satisfy every discriminator test.
func filterBySlice(nodes []node, c criteria) []node {
	var out []node
	for _, n := range nodes {
		ok := true
		for _, t := range c.tests {
			if !testPasses(n.v, t) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, n)
		}
	}
	return out
}

func testPasses(v any, t sliceTest) bool {
	if t.path == "" {
		return patternMatches(t.want, v)
	}
	for _, got := range navigate(v, t.path) {
		if patternMatches(t.want, got) {
			return true
		}
	}
	return false
}

// navigate follows a dotted discriminator path, flattening arrays.
func navigate(v any, path string) []any {
	current := []any{v}
	for _, part := range strings.Split(path, ".") {
		var next []any
		for _, c := range current {
			obj, ok := c.(map[string]any)
			if !ok {
				continue
			}
			for _, key := range candidateKeys(obj, part) {
				switch child := obj[key].(type) {
				case []any:
					next = append(next, child...)
				default:
					if child != nil {
						next = append(next, child)
					}
				}
			}
		}
		current = next
		if len(current) == 0 {
			return nil
		}
	}
	return current
}

// patternMatches implements FHIR pattern semantics: every value the pattern
// states must be present and equal in the instance, while anything the pattern
// does not mention is free. It is a subset test, not equality — a pattern of
// {"use": "official"} matches a name that also carries family and given.
func patternMatches(want, got any) bool {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return false
		}
		for k, wv := range w {
			gv, ok := g[k]
			if !ok || !patternMatches(wv, gv) {
				return false
			}
		}
		return true

	case []any:
		g, ok := got.([]any)
		if !ok {
			return false
		}
		// Each entry the pattern requires must be matched by some entry of the
		// instance; order and extra entries do not matter.
		for _, wv := range w {
			found := false
			for _, gv := range g {
				if patternMatches(wv, gv) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true

	default:
		return reflect.DeepEqual(want, got)
	}
}

// isEmpty reports whether a decoded JSON value carries no information. An empty
// string or object is present in the document but populates nothing, and
// counting it as coverage would let a placeholder example pass for a real one.
func isEmpty(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(t) == ""
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	default:
		return false
	}
}
