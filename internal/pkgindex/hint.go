package pkgindex

import (
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/fhirlint/fhirlint/internal/fhirpkg"
	"github.com/fhirlint/fhirlint/internal/profiles"
	"github.com/fhirlint/fhirlint/internal/validator"
)

// notFoundIDs are the messages whose fix is loading the package that defines
// the canonical they name. Collected from Messages.properties of 6.10.3 and
// 7.0.3, which have all of them with the same ids; each names the canonical as
// {0}.
var notFoundIDs = map[string]bool{
	// Profiles.
	"Validation_VAL_Profile_Unknown":                                true,
	"VALIDATION_VAL_PROFILE_UNKNOWN_ERROR":                          true,
	"VALIDATION_VAL_PROFILE_UNKNOWN_ERROR_NETWORK":                  true,
	"VALIDATION_VAL_PROFILE_UNKNOWN_NOT_POLICY":                     true,
	"Unable_to_locate_the_profile__in_order_to_validate_against_it": true,
	"VALIDATION_VAL_GLOBAL_PROFILE_UNKNOWN":                         true,
	// Extensions.
	"Extension_EXT_Unknown":         true,
	"Extension_EXT_Unknown_NotHere": true,
	// Value sets and canonicals in general.
	"Unable_to_resolve_value_Set_":              true,
	"Terminology_TX_ValueSet_NotFound":          true,
	"TYPE_SPECIFIC_CHECKS_DT_CANONICAL_RESOLVE": true,
}

// NeedsHint reports whether an issue is one Annotate would add a hint to, so
// that a run without any builds no index.
func NeedsHint(iss validator.Issue) bool { return notFoundIDs[iss.MessageID] }

// canonicalInMessage is the first http(s) URL in a message, with its
// "|version" when it has one. The templates quote it in '…' or not at all.
var canonicalInMessage = regexp.MustCompile(`https?://[^\s'"(),]+`)

// Annotate adds a PackageHint to every issue that says a canonical could not
// be found. The index is built at most once, and only when such an issue exists:
// the cache can hold hundreds of packages (214 packages, 48 MB of index files:
// 0.4 s).
func Annotate(results []*validator.Result) { new(Annotator).Annotate(results) }

// Annotator is Annotate with an index kept across calls, for a long-running
// process such as the language server that validates on every save. A package
// installed after the first build is not seen until the process restarts.
type Annotator struct {
	once sync.Once
	ix   *Index
}

func (a *Annotator) index() *Index {
	a.once.Do(func() { a.ix = Build() })
	return a.ix
}

// Annotate adds hints to results, building the index on first need.
func (a *Annotator) Annotate(results []*validator.Result) {
	for _, r := range results {
		if r == nil {
			continue
		}
		for i := range r.Issues {
			iss := &r.Issues[i]
			if !NeedsHint(*iss) || iss.Redacted {
				continue
			}
			canonical := strings.TrimRight(canonicalInMessage.FindString(iss.Message), ".")
			if canonical == "" {
				continue
			}
			iss.Hint = hintFor(a.index(), canonical)
		}
	}
}

func hintFor(ix *Index, canonical string) *validator.PackageHint {
	h := &validator.PackageHint{Canonical: canonical, Packages: ix.Lookup(canonical)}
	if len(h.Packages) > 0 {
		h.Package = h.Packages[0]
		h.Alias = aliasFor(h.Package)
	}
	return h
}

// aliasFor is a built-in alias that loads the same package, preferring one
// that pins the same version and, among equals, one that loads nothing else.
func aliasFor(pkg string) string {
	name, _ := fhirpkg.SplitID(pkg)
	type cand struct {
		alias        string
		exact, alone bool
	}
	var cands []cand
	for alias, ids := range profiles.Aliases {
		for _, id := range ids {
			if n, _ := fhirpkg.SplitID(id); strings.EqualFold(n, name) {
				cands = append(cands, cand{alias, id == pkg, len(ids) == 1})
				break
			}
		}
	}
	if len(cands) == 0 {
		return ""
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].exact != cands[j].exact {
			return cands[i].exact
		}
		if cands[i].alone != cands[j].alone {
			return cands[i].alone
		}
		return cands[i].alias < cands[j].alias
	})
	return cands[0].alias
}
