package fhirpkg

import (
	"strings"
	"testing"
)

func TestIsExactVersion(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"4.0.1", true},
		{"2025.0.1", true},
		{"1.5.x", false},
		{"2025.0.x", false},
		{"^1.0.0", false},
		{"~1.0.0", false},
		{">=1.0.0", false},
		{"1.0.0 || 2.0.0", false},
		{"", false},
	} {
		if got := IsExactVersion(tc.in); got != tc.want {
			t.Errorf("IsExactVersion(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestResolveInstalled(t *testing.T) {
	idx := []Package{
		{ID: "a#1.5.2", Manifest: Manifest{Name: "a", Version: "1.5.2"}},
		{ID: "a#1.5.4", Manifest: Manifest{Name: "a", Version: "1.5.4"}},
		{ID: "a#1.6.0", Manifest: Manifest{Name: "a", Version: "1.6.0"}},
		{ID: "B#2.0.0", Manifest: Manifest{Name: "B", Version: "2.0.0"}},
	}
	for _, tc := range []struct {
		name, constraint, want string
	}{
		{"a", "1.5.4", "1.5.4"},  // exact, installed
		{"a", "9.9.9", ""},       // exact, not installed
		{"a", "1.5.x", "1.5.4"},  // range picks the newest match
		{"a", "1.6.x", "1.6.0"},  // a different range
		{"a", "2.0.x", ""},       // range with no match
		{"b", "2.0.0", "2.0.0"},  // case-insensitive name match
		{"missing", "1.0.0", ""}, // unknown package
	} {
		if got := ResolveInstalled(tc.name, tc.constraint, idx); got != tc.want {
			t.Errorf("ResolveInstalled(%q, %q) = %q, want %q", tc.name, tc.constraint, got, tc.want)
		}
	}
}

func TestTree_NotInstalled(t *testing.T) {
	fakeCache(t)
	_, err := Tree("a.pkg", "1.0.0", 0)
	if err == nil {
		t.Fatal("want an error for a package that is not cached")
	}
	if !strings.Contains(err.Error(), "not in the FHIR package cache") {
		t.Errorf("unhelpful error: %v", err)
	}
}

func TestTree_ResolvesRangesAndMarksThem(t *testing.T) {
	root := fakeCache(t)
	install(t, root, "app#1.0.0", Manifest{Dependencies: map[string]string{
		"core": "4.0.1", "base": "1.5.x",
	}}, 0)
	install(t, root, "core#4.0.1", Manifest{}, 0)
	install(t, root, "base#1.5.2", Manifest{}, 0)
	install(t, root, "base#1.5.4", Manifest{Dependencies: map[string]string{"core": "4.0.1"}}, 0)

	n, err := Tree("app", "1.0.0", 0)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]*Node{}
	for _, c := range n.Children {
		byName[c.Name] = c
	}
	if b := byName["base"]; b == nil || b.Version != "1.5.4" {
		t.Fatalf("base resolved to %+v, want the newest 1.5.x (1.5.4)", b)
	}
	if byName["base"].Exact {
		t.Error("a 1.5.x constraint must be marked inexact")
	}
	if !byName["core"].Exact {
		t.Error("an exact constraint must be marked exact")
	}
	missing, inexact, truncated := Counts(n)
	if missing != 0 || inexact != 1 || truncated {
		t.Errorf("Counts() = %d,%d,%v want 0,1,false", missing, inexact, truncated)
	}
}

func TestTree_MissingDependency(t *testing.T) {
	root := fakeCache(t)
	install(t, root, "app#1.0.0", Manifest{Dependencies: map[string]string{"gone": "1.0.0"}}, 0)

	n, err := Tree("app", "1.0.0", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Children) != 1 || n.Children[0].Installed {
		t.Fatalf("want one uninstalled child, got %+v", n.Children)
	}
	if missing, _, _ := Counts(n); missing != 1 {
		t.Errorf("missing = %d, want 1", missing)
	}
}

// The core package is a dependency of nearly everything; expanding it once is enough.
func TestTree_RepeatedSubtreeIsMarkedNotReExpanded(t *testing.T) {
	root := fakeCache(t)
	install(t, root, "app#1.0.0", Manifest{Dependencies: map[string]string{
		"x": "1.0.0", "y": "1.0.0",
	}}, 0)
	install(t, root, "x#1.0.0", Manifest{Dependencies: map[string]string{"core": "4.0.1"}}, 0)
	install(t, root, "y#1.0.0", Manifest{Dependencies: map[string]string{"core": "4.0.1"}}, 0)
	install(t, root, "core#4.0.1", Manifest{}, 0)

	n, err := Tree("app", "1.0.0", 0)
	if err != nil {
		t.Fatal(err)
	}
	var repeats int
	var walk func(*Node)
	walk = func(n *Node) {
		if n.Repeated {
			repeats++
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(n)
	if repeats != 1 {
		t.Errorf("got %d repeated markers, want exactly 1 (core expanded once, flagged once)", repeats)
	}
}

// A cycle must terminate rather than hang, even though published packages
// should not contain one.
func TestTree_CycleTerminates(t *testing.T) {
	root := fakeCache(t)
	install(t, root, "a#1.0.0", Manifest{Dependencies: map[string]string{"b": "1.0.0"}}, 0)
	install(t, root, "b#1.0.0", Manifest{Dependencies: map[string]string{"a": "1.0.0"}}, 0)

	n, err := Tree("a", "1.0.0", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Children) != 1 || len(n.Children[0].Children) != 1 {
		t.Fatalf("unexpected shape: %+v", n)
	}
	if !n.Children[0].Children[0].Repeated {
		t.Error("the cycle's closing edge must be marked repeated, not expanded again")
	}
}

func TestTree_DepthLimit(t *testing.T) {
	root := fakeCache(t)
	install(t, root, "a#1.0.0", Manifest{Dependencies: map[string]string{"b": "1.0.0"}}, 0)
	install(t, root, "b#1.0.0", Manifest{Dependencies: map[string]string{"c": "1.0.0"}}, 0)
	install(t, root, "c#1.0.0", Manifest{}, 0)

	// depth 1 = direct dependencies only
	n, err := Tree("a", "1.0.0", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Children) != 1 {
		t.Fatalf("want the direct dependency at depth 1, got %d children", len(n.Children))
	}
	if len(n.Children[0].Children) != 0 {
		t.Error("depth 1 must not expand the second level")
	}
	if _, _, truncated := Counts(n); !truncated {
		t.Error("a cut-off node must report as truncated")
	}

	// A leaf at the depth boundary has nothing below it and is not truncated.
	full, err := Tree("a", "1.0.0", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, truncated := Counts(full); truncated {
		t.Error("an unlimited walk must not report truncation")
	}
}
