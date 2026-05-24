package stats

import "testing"

func TestCompute_CountsAndBuckets(t *testing.T) {
	r := Compute([]Resource{
		{Type: "Patient", Profiles: []string{"P1"}},
		{Type: "Patient"},
		{Type: "Observation", Profiles: []string{"P1", "P2"}},
		{Type: ""}, // unreadable → (unknown)
	})

	if r.TotalResources != 4 {
		t.Errorf("expected 4 resources, got %d", r.TotalResources)
	}

	types := map[string]int{}
	for _, tc := range r.ResourceTypes {
		types[tc.Type] = tc.Count
	}
	if types["Patient"] != 2 || types["Observation"] != 1 || types[unknownType] != 1 {
		t.Errorf("unexpected type counts: %+v", types)
	}

	profiles := map[string]int{}
	for _, pc := range r.Profiles {
		profiles[pc.Profile] = pc.Count
	}
	if profiles["P1"] != 2 || profiles["P2"] != 1 {
		t.Errorf("unexpected profile counts: %+v", profiles)
	}
	// One Patient and the (unknown) resource declare no profile → (none) = 2.
	if profiles[noneProfile] != 2 {
		t.Errorf("expected (none)=2, got %d", profiles[noneProfile])
	}
}

func TestCompute_TypesSortedByCountDesc(t *testing.T) {
	r := Compute([]Resource{
		{Type: "A"}, {Type: "B"}, {Type: "B"}, {Type: "C"}, {Type: "C"}, {Type: "C"},
	})
	want := []string{"C", "B", "A"}
	for i, w := range want {
		if r.ResourceTypes[i].Type != w {
			t.Errorf("position %d: expected %q, got %q", i, w, r.ResourceTypes[i].Type)
		}
	}
}

func TestCompute_NoneProfileSortsLast(t *testing.T) {
	r := Compute([]Resource{
		{Type: "X"},                          // none
		{Type: "Y", Profiles: []string{"Z"}}, // one declared profile
	})
	last := r.Profiles[len(r.Profiles)-1]
	if last.Profile != noneProfile {
		t.Errorf("expected (none) last, got %q", last.Profile)
	}
}

func TestPercent(t *testing.T) {
	cases := []struct {
		n, total int
		want     string
	}{{3, 4, "75%"}, {0, 0, "0%"}, {5, 5, "100%"}, {1, 3, "33%"}}
	for _, c := range cases {
		if got := percent(c.n, c.total); got != c.want {
			t.Errorf("percent(%d,%d)=%q, want %q", c.n, c.total, got, c.want)
		}
	}
}
