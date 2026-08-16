package coverage

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

func sortByURL(sds []*StructureDefinition) {
	sort.Slice(sds, func(i, j int) bool { return sds[i].URL < sds[j].URL })
}

// maxBaseChain bounds the walk up baseDefinition. A malformed package can point
// two profiles at each other, and a coverage run must not hang on it.
const maxBaseChain = 32

// Profile is a StructureDefinition together with the mustSupport elements that
// apply to it, including those it inherits.
type Profile struct {
	SD *StructureDefinition

	// MustSupport holds the element IDs to check, in the order the profile
	// declares them.
	MustSupport []string

	// Warnings records what could not be established while assembling the
	// element set — an unresolvable base, most often. They are reported rather
	// than swallowed, because each one means the mustSupport list may be short.
	Warnings []string
}

// Resolve assembles the mustSupport element set for a profile.
//
// With a snapshot the answer is direct: the snapshot already contains every
// inherited element. Without one, the profile's own differential is the
// starting point and the baseDefinition chain is walked to pick up mustSupport
// elements the profile inherits rather than declares. A base that is not in the
// registry stops the walk and is recorded as a warning: the resulting list is
// then a lower bound, and saying so is the difference between a useful report
// and a misleading one.
func (r *Registry) Resolve(sd *StructureDefinition) *Profile {
	p := &Profile{SD: sd}
	seen := map[string]bool{}

	add := func(elems []*Element) {
		for _, e := range elems {
			if !e.MustSupport || e.ID == "" || seen[e.ID] {
				continue
			}
			seen[e.ID] = true
			p.MustSupport = append(p.MustSupport, e.ID)
		}
	}

	add(sd.Elements())
	if sd.HasSnapshot() {
		return p
	}

	current := sd
	for i := 0; i < maxBaseChain; i++ {
		if current.BaseDefinition == "" || current.BaseDefinition == coreBaseFor(sd.Type) {
			// The core resource definition is where every chain ends, and it
			// declares no mustSupport. Reaching it is completion, not a gap, so
			// it must not produce a warning even when core is not loaded.
			return p
		}
		base, ok := r.Get(current.BaseDefinition)
		if !ok {
			p.Warnings = append(p.Warnings,
				"base profile "+current.BaseDefinition+" is not loaded — inherited mustSupport elements may be missing")
			return p
		}
		// Only a profile on the same resource type contributes element IDs that
		// are meaningful here. A specialization is the base resource itself,
		// where mustSupport does not appear.
		if base.Derivation == "constraint" && base.Type == sd.Type {
			add(base.Elements())
		}
		current = base
	}
	p.Warnings = append(p.Warnings,
		"base profile chain exceeded "+strconv.Itoa(maxBaseChain)+" levels — stopped walking")
	return p
}

// coreBaseFor returns the canonical URL of the base FHIR definition for a
// resource or datatype name.
func coreBaseFor(typeName string) string {
	if typeName == "" {
		return ""
	}
	return "http://hl7.org/fhir/StructureDefinition/" + typeName
}

// segment is one step of an element ID: a field name, optionally naming a slice.
type segment struct {
	Name  string // "identifier", "value[x]"
	Slice string // "VersichertenId", empty when the segment is not sliced
}

// parseElementID splits an element ID such as
// "Patient.identifier:VersichertenId.value" into its segments. The first
// segment is the resource or datatype the ID is rooted at.
func parseElementID(id string) []segment {
	parts := strings.Split(id, ".")
	segs := make([]segment, 0, len(parts))
	for _, p := range parts {
		if i := strings.Index(p, ":"); i >= 0 {
			segs = append(segs, segment{Name: p[:i], Slice: p[i+1:]})
			continue
		}
		segs = append(segs, segment{Name: p})
	}
	return segs
}

// joinSegments is the inverse of parseElementID.
func joinSegments(segs []segment) string {
	parts := make([]string, len(segs))
	for i, s := range segs {
		parts[i] = s.Name
		if s.Slice != "" {
			parts[i] += ":" + s.Slice
		}
	}
	return strings.Join(parts, ".")
}

// criteria describes how to recognise the members of one slice.
type criteria struct {
	// tests are evaluated against a candidate node; all must pass.
	tests []sliceTest

	// unresolvedReason is set when no usable test could be derived, in which
	// case tests is empty and the element must be reported as unresolved rather
	// than as populated or missing.
	unresolvedReason string
}

// sliceTest is one discriminator turned into a check on a candidate node.
type sliceTest struct {
	// path is the discriminator path relative to the slice, "" for $this.
	path string
	// want is the pattern or fixed value the node must match.
	want any
}

// sliceCriteria works out how to recognise members of the slice named by the
// last segment of sliceID.
//
// The evidence usually sits on the slice element itself (pattern[x] / fixed[x])
// or on a child element named by the discriminator path. When neither is
// present the search continues inside the profile referenced by the nearest
// ancestor's type.profile: German profiles routinely constrain a datatype
// through a separate profile — a HumanName through humanname-de-basis, say —
// and leave the slice definitions there, so a lookup that stopped at the
// current StructureDefinition would give up on exactly the elements that matter.
func (r *Registry) sliceCriteria(sd *StructureDefinition, sliceID string) criteria {
	if c, ok := r.criteriaIn(sd, sliceID, 0); ok {
		return c
	}
	// A slice whose only distinguishing feature is a value set binding cannot be
	// decided without expanding that value set, which is terminology work and
	// would take coverage online. Naming the reason is more use than a generic
	// shrug: it tells the reader the gap is inherent, not a missing package.
	if e, ok := sd.index()[sliceID]; ok && e.BindingValueSet != "" {
		return criteria{unresolvedReason: "slice is identified by a value set binding (" +
			e.BindingValueSet + "), which coverage does not expand"}
	}
	return criteria{unresolvedReason: "no discriminator value found for this slice"}
}

// maxProfileHops bounds how far criteria lookup follows type.profile references.
const maxProfileHops = 8

func (r *Registry) criteriaIn(sd *StructureDefinition, sliceID string, hops int) (criteria, bool) {
	if hops > maxProfileHops {
		return criteria{}, false
	}

	idx := sd.index()
	if slice, ok := idx[sliceID]; ok {
		if c, ok := r.criteriaFromElement(sd, sliceID, slice); ok {
			return c, true
		}
	}

	// Not defined here. Follow the nearest ancestor that delegates to another
	// profile, and continue the lookup there against the remaining path.
	base, rest, ok := r.delegateFor(sd, sliceID)
	if !ok {
		return criteria{}, false
	}
	return r.criteriaIn(base, rest, hops+1)
}

// criteriaFromElement turns a slice element into tests, using the parent's
// discriminators to decide what to compare.
func (r *Registry) criteriaFromElement(sd *StructureDefinition, sliceID string, slice *Element) (criteria, bool) {
	segs := parseElementID(sliceID)
	parentID := joinSegments(append(append([]segment{}, segs[:len(segs)-1]...),
		segment{Name: segs[len(segs)-1].Name}))

	idx := sd.index()
	parent := idx[parentID]

	var discs []Discriminator
	if parent != nil && parent.Slicing != nil {
		discs = parent.Slicing.Discriminator
	}
	if len(discs) == 0 {
		// An unsliced parent still leaves one reliable reading for extensions,
		// whose url is fixed by definition.
		discs = []Discriminator{{Type: "value", Path: "url"}}
	}

	var c criteria
	for _, d := range discs {
		switch d.Type {
		case "value", "pattern":
			if t, ok := r.testFor(sd, sliceID, slice, d.Path); ok {
				c.tests = append(c.tests, t)
			}
		case "type", "profile", "exists":
			// Deliberately not derived. A "type" discriminator on a choice
			// element is already handled by the field name in the instance, and
			// the rest are rare enough (fewer than 1 in 300 discriminators
			// across every package examined) that guessing at them would buy
			// less than the wrong answers would cost.
		}
	}
	return c, len(c.tests) > 0
}

// testFor builds the check for one discriminator path.
func (r *Registry) testFor(sd *StructureDefinition, sliceID string, slice *Element, discPath string) (sliceTest, bool) {
	if discPath == "" || discPath == "$this" {
		if v, ok := decodeValue(slice.Pattern, slice.Fixed); ok {
			return sliceTest{path: "", want: v}, true
		}
		// An extension slice identifies itself by url, and the url of a defined
		// extension is the profile it points at.
		if url, ok := profileURL(slice); ok {
			return sliceTest{path: "url", want: url}, true
		}
		return sliceTest{}, false
	}

	if discPath == "url" {
		if url, ok := profileURL(slice); ok {
			return sliceTest{path: "url", want: url}, true
		}
	}

	child, ok := sd.index()[sliceID+"."+discPath]
	if !ok {
		return sliceTest{}, false
	}
	v, ok := decodeValue(child.Pattern, child.Fixed)
	if !ok {
		return sliceTest{}, false
	}
	return sliceTest{path: discPath, want: v}, true
}

// profileURL returns the single profile a slice element's type points at.
func profileURL(e *Element) (string, bool) {
	if e == nil {
		return "", false
	}
	for _, t := range e.Types {
		if len(t.Profile) == 1 && t.Profile[0] != "" {
			return t.Profile[0], true
		}
	}
	return "", false
}

// delegateFor finds the nearest ancestor of elementID whose type delegates to
// another profile, and returns that profile together with elementID rebased
// onto it.
func (r *Registry) delegateFor(sd *StructureDefinition, elementID string) (*StructureDefinition, string, bool) {
	segs := parseElementID(elementID)
	idx := sd.index()

	// Longest ancestor first: the closest delegation is the most specific one.
	for i := len(segs) - 1; i >= 1; i-- {
		ancestorID := joinSegments(segs[:i])
		anc, ok := idx[ancestorID]
		if !ok {
			continue
		}
		url, ok := profileURL(anc)
		if !ok {
			continue
		}
		target, ok := r.Get(url)
		if !ok {
			continue
		}
		rebased := joinSegments(append([]segment{{Name: target.Type}}, segs[i:]...))
		return target, rebased, true
	}

	// Nothing delegates. Fall back to the profile's own base, rebasing onto it
	// unchanged — a derived profile's slice may simply be declared one level up.
	if sd.BaseDefinition != "" {
		if base, ok := r.Get(sd.BaseDefinition); ok && base.Type == sd.Type && base.Derivation == "constraint" {
			return base, elementID, true
		}
	}
	return nil, "", false
}

// decodeValue returns the first of pattern or fixed that is present.
func decodeValue(pattern, fixed []byte) (any, bool) {
	for _, raw := range [][]byte{pattern, fixed} {
		if len(raw) == 0 {
			continue
		}
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			continue
		}
		return v, true
	}
	return nil, false
}
