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
		got, certain := ResolveInstalled(tc.name, tc.constraint, idx)
		if got != tc.want {
			t.Errorf("ResolveInstalled(%q, %q) = %q, want %q", tc.name, tc.constraint, got, tc.want)
		}
		if !certain {
			t.Errorf("ResolveInstalled(%q, %q) reported an ambiguous pick among orderable versions",
				tc.name, tc.constraint)
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

// The bug this file's ordering was fixed for: "1.5.10" sorts before "1.5.9" as
// a string, so the newest match was never last (#390).
func TestResolveInstalled_DoubleDigitSegments(t *testing.T) {
	idx := []Package{
		{Manifest: Manifest{Name: "p", Version: "1.5.2"}},
		{Manifest: Manifest{Name: "p", Version: "1.5.9"}},
		{Manifest: Manifest{Name: "p", Version: "1.5.10"}},
	}
	got, certain := ResolveInstalled("p", "1.5.x", idx)
	if got != "1.5.10" {
		t.Errorf("ResolveInstalled = %q, want 1.5.10 (string order would give 1.5.9)", got)
	}
	if !certain {
		t.Error("these versions order fine; the pick must not be reported as a guess")
	}
}

// MII modules version as YYYY.0.N and reach double digits in normal use.
func TestResolveInstalled_MIIVersionScheme(t *testing.T) {
	idx := []Package{
		{Manifest: Manifest{Name: "m", Version: "2025.0.9"}},
		{Manifest: Manifest{Name: "m", Version: "2025.0.11"}},
		{Manifest: Manifest{Name: "m", Version: "2025.0.2"}},
	}
	if got, _ := ResolveInstalled("m", "2025.0.x", idx); got != "2025.0.11" {
		t.Errorf("ResolveInstalled = %q, want 2025.0.11", got)
	}
}

// A publisher that changed scheme mid-prefix leaves versions with no order
// between them. Picking one is unavoidable; doing it silently is not.
func TestResolveInstalled_IncomparableCandidatesAreReported(t *testing.T) {
	idx := []Package{
		{Manifest: Manifest{Name: "p", Version: "1.5.2"}},
		{Manifest: Manifest{Name: "p", Version: "1.5.Q1"}}, // not two integers, so unorderable
	}
	got, certain := ResolveInstalled("p", "1.5.x", idx)
	if got == "" {
		t.Fatal("want some candidate to be chosen")
	}
	if certain {
		t.Error("a pick among unorderable versions must not be reported as certain")
	}
}

// An exact constraint is a lookup, never a guess.
func TestResolveInstalled_ExactIsAlwaysCertain(t *testing.T) {
	idx := []Package{{Manifest: Manifest{Name: "p", Version: "1.5.2"}}}
	if got, certain := ResolveInstalled("p", "1.5.2", idx); got != "1.5.2" || !certain {
		t.Errorf("got %q certain=%v, want 1.5.2 certain", got, certain)
	}
	if got, certain := ResolveInstalled("p", "9.9.9", idx); got != "" || !certain {
		t.Errorf("a missing exact version is a fact, not a guess: %q certain=%v", got, certain)
	}
}

// The tree has to carry the uncertainty out to whoever renders it.
func TestTree_MarksAnAmbiguousRange(t *testing.T) {
	root := fakeCache(t)
	install(t, root, "app#1.0.0", Manifest{Dependencies: map[string]string{"base": "1.5.x"}}, 0)
	install(t, root, "base#1.5.2", Manifest{}, 0)
	install(t, root, "base#1.5.Q1", Manifest{}, 0)

	n, err := Tree("app", "1.0.0", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Children) != 1 {
		t.Fatalf("want one child, got %d", len(n.Children))
	}
	if !n.Children[0].Ambiguous {
		t.Error("a range resolved across unorderable versions must be marked ambiguous")
	}
}

// And an ordinary range must not be marked, or the marker means nothing.
func TestTree_DoesNotMarkAnOrderableRange(t *testing.T) {
	root := fakeCache(t)
	install(t, root, "app#1.0.0", Manifest{Dependencies: map[string]string{"base": "1.5.x"}}, 0)
	install(t, root, "base#1.5.9", Manifest{}, 0)
	install(t, root, "base#1.5.10", Manifest{}, 0)

	n, err := Tree("app", "1.0.0", 0)
	if err != nil {
		t.Fatal(err)
	}
	if n.Children[0].Ambiguous {
		t.Error("orderable versions must not be marked ambiguous")
	}
	if n.Children[0].Version != "1.5.10" {
		t.Errorf("resolved to %q, want 1.5.10", n.Children[0].Version)
	}
}

// List sorts versions ascending, and Grouped depends on that to name a group.
func TestList_SortsVersionsNumerically(t *testing.T) {
	root := fakeCache(t)
	install(t, root, "p#1.5.9", Manifest{}, 0)
	install(t, root, "p#1.5.10", Manifest{}, 0)
	install(t, root, "p#1.5.2", Manifest{}, 0)

	pkgs, err := List(false)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range pkgs {
		got = append(got, p.Version)
	}
	want := []string{"1.5.2", "1.5.9", "1.5.10"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("List order = %v, want %v", got, want)
		}
	}
}

// Grouped takes its display name from the newest entry, so a mixed-case package
// whose newest version is double-digit must still name the newer spelling.
func TestGrouped_NamesTheNewestSpellingWithDoubleDigits(t *testing.T) {
	root := fakeCache(t)
	install(t, root, "KBV.Basis#1.5.9", Manifest{}, 0)
	install(t, root, "kbv.basis#1.5.10", Manifest{}, 0)

	pkgs, err := List(false)
	if err != nil {
		t.Fatal(err)
	}
	groups := Grouped(pkgs)
	if len(groups) != 1 {
		t.Fatalf("want one folded group, got %d", len(groups))
	}
	if groups[0].Name != "kbv.basis" {
		t.Errorf("group name = %q, want kbv.basis (the newer 1.5.10 spelling)", groups[0].Name)
	}
}
