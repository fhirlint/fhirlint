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
	Truncated bool    `json:"truncated,omitempty"`
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
		n.Children = append(n.Children,
			buildNode(dep, ResolveInstalled(dep, want, idx), want, depth+1, maxDepth, idx, seen))
	}
	return n
}

// ResolveInstalled picks the installed version satisfying a constraint, or ""
// when nothing matches.
//
// An exact constraint is looked up directly. A range ("1.5.x") is matched by
// prefix against what is installed, newest first — making visible the
// "whatever is on disk" behaviour a run ends up with, rather than pretending
// the manifest decided it.
func ResolveInstalled(name, constraint string, idx []Package) string {
	if IsExactVersion(constraint) {
		for _, p := range idx {
			if strings.EqualFold(p.Name, name) && p.Version == constraint {
				return constraint
			}
		}
		return ""
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
		return ""
	}
	sort.Strings(candidates)
	return candidates[len(candidates)-1]
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
