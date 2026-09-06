package fhirpkg

import (
	"fmt"
	"sort"
	"strings"
)

// Node is one package in a resolved dependency tree.
type Node struct {
	Name string `json:"name"`
	// Constraint is what the parent declared, verbatim — "4.0.1", but also
	// "1.5.x". Empty at the root.
	Constraint string `json:"constraint,omitempty"`
	// Version is what is installed and was walked into. Empty when the package
	// is missing from the cache.
	Version   string `json:"version,omitempty"`
	Installed bool   `json:"installed"`
	// Exact is false when the constraint is a range, so the manifest alone does
	// not determine which version a run resolves to.
	Exact bool `json:"exact"`
	// Repeated marks a package already expanded elsewhere in the tree.
	Repeated bool `json:"repeated,omitempty"`
	// Truncated marks a node whose dependencies exist but were cut off by maxDepth.
	Truncated bool `json:"truncated,omitempty"`
	// Ambiguous marks a range whose candidates do not order against each other,
	// so the version shown is a stable guess and not a resolution.
	Ambiguous bool    `json:"ambiguous,omitempty"`
	Children  []*Node `json:"children,omitempty"`
}

// Tree resolves a cached package's dependency tree from the manifests on disk.
//
// maxDepth counts levels of dependencies below the root, so 1 yields the direct
// ones; 0 means unlimited.
func Tree(name, version string, maxDepth int) (*Node, error) {
	if _, ok := Load(name, version); !ok {
		return nil, fmt.Errorf(
			"%s#%s is not in the FHIR package cache — validate against it once to install it, "+
				"or run 'fhirlint packages' to see what is there", name, version)
	}
	// One index for the whole walk: a tree of sixty packages otherwise re-reads
	// the cache directory once per range constraint.
	idx, err := List(false)
	if err != nil {
		return nil, err
	}
	return buildNode(name, version, "", 0, maxDepth, idx, map[string]bool{}), nil
}

// buildNode walks depth-first. seen guards against both repeated subtrees — the
// r4 core package is a dependency of nearly everything — and cycles, which
// published packages should not contain but must not hang the walk if they do.
func buildNode(name, version, constraint string, depth, maxDepth int, idx []Package, seen map[string]bool) *Node {
	n := &Node{
		Name:       name,
		Constraint: constraint,
		Version:    version,
		Installed:  version != "",
		Exact:      constraint == "" || IsExactVersion(constraint),
	}
	if version == "" {
		return n
	}
	key := strings.ToLower(name) + "#" + version
	if seen[key] {
		n.Repeated = true
		return n
	}
	seen[key] = true

	pkg, ok := Load(name, version)
	if !ok || len(pkg.Dependencies) == 0 {
		return n
	}
	if maxDepth > 0 && depth >= maxDepth {
		n.Truncated = true
		return n
	}

	deps := make([]string, 0, len(pkg.Dependencies))
	for d := range pkg.Dependencies {
		deps = append(deps, d)
	}
	sort.Strings(deps)

	for _, dep := range deps {
		want := pkg.Dependencies[dep]
		resolved, certain := ResolveInstalled(dep, want, idx)
		child := buildNode(dep, resolved, want, depth+1, maxDepth, idx, seen)
		child.Ambiguous = !certain
		n.Children = append(n.Children, child)
	}
	return n
}

// ResolveInstalled picks the installed version satisfying a constraint, or ""
// when nothing matches. certain is false when the pick had to fall back on
// string order because the candidates do not order against each other.
//
// An exact constraint is looked up directly. A range ("1.5.x") is matched by
// prefix against what is installed, newest first — making visible the
// "whatever is on disk" behaviour a run ends up with, rather than pretending
// the manifest decided it.
//
// "Newest" is a numeric comparison, not a string one. Sorting "1.5.9" and
// "1.5.10" as strings puts 1.5.9 last and quietly resolves the range to the
// older package, which is what this function did until #390. The MII modules
// version as YYYY.0.N and reach double digits in normal use.
func ResolveInstalled(name, constraint string, idx []Package) (version string, certain bool) {
	if IsExactVersion(constraint) {
		for _, p := range idx {
			if strings.EqualFold(p.Name, name) && p.Version == constraint {
				return constraint, true
			}
		}
		return "", true
	}
	prefix := strings.TrimRight(constraint, "x*")
	var candidates []string
	for _, p := range idx {
		if !strings.EqualFold(p.Name, name) {
			continue
		}
		if prefix == "" || strings.HasPrefix(p.Version, prefix) {
			candidates = append(candidates, p.Version)
		}
	}
	if len(candidates) == 0 {
		return "", true
	}
	return newest(candidates)
}

// newest returns the highest of a set of versions. certain is false when some
// pair could not be ordered, in which case the answer is a stable guess rather
// than a fact: a publisher that changed versioning scheme mid-prefix leaves
// versions that genuinely have no order between them, and picking one silently
// is the habit this file is being cured of.
func newest(versions []string) (version string, certain bool) {
	// Stable starting point so the result does not depend on directory order.
	sorted := append([]string(nil), versions...)
	sort.Strings(sorted)

	best, certain := sorted[0], true
	for _, v := range sorted[1:] {
		cmp, ok := CompareVersions(v, best)
		if !ok {
			certain = false
			continue // keep the string-order winner for this pair
		}
		if cmp > 0 {
			best = v
		}
	}
	return best, certain
}

// IsExactVersion reports whether a dependency constraint names exactly one version.
func IsExactVersion(c string) bool {
	return c != "" && !strings.ContainsAny(c, "x*^~><| ")
}

// Counts walks a tree and reports what a caller needs to summarise it.
func Counts(n *Node) (missing, inexact int, truncated bool) {
	var walk func(*Node, bool)
	walk = func(n *Node, isRoot bool) {
		if !isRoot {
			if !n.Installed {
				missing++
			} else if !n.Exact {
				inexact++
			}
		}
		if n.Truncated {
			truncated = true
		}
		for _, c := range n.Children {
			walk(c, false)
		}
	}
	walk(n, true)
	return missing, inexact, truncated
}
