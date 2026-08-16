package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	yaml "go.yaml.in/yaml/v3"
)

// TestGoReleaserChangelogFilters checks the release-notes exclusion patterns
// against the commit subjects this repo actually writes.
//
// It exists because the failure mode is invisible until release day: a pattern
// that misses turns into a housekeeping commit sitting in notes people read to
// decide whether to upgrade, and nobody finds out until the tag is already
// pushed. v1.8.0 shipped with "ci(#310): …" in its changelog for exactly that
// reason — "^ci:" does not match a scoped commit, and this repo scopes nearly
// every commit with an issue number.
func TestGoReleaserChangelogFilters(t *testing.T) {
	patterns := loadChangelogExcludes(t)

	cases := []struct {
		subject string
		exclude bool
	}{
		// Housekeeping, unscoped and scoped. Both spellings occur in history.
		{"chore: remove Reddit post draft from the repo (#323)", true},
		{"chore(#42): bump version references to 1.9.0", true},
		{"docs: describe the coverage command", true},
		{"docs(#12): fix a broken anchor", true},
		{"ci(#310): pin the validator for push runs", true},
		{"ci: run tests on windows", true},
		{"test: cover the slice matcher", true},
		{"test(#7): add a regression case", true},
		{"Merge pull request #1 from fhirlint/branch", true},
		{"Merge branch 'main' into feature", true},

		// Everything a user upgrading would want to see must survive.
		{"feat(#324): add fhirlint coverage for must-support element coverage", false},
		{"feat: add --group", false},
		{"fix(#321): bump Go to 1.25.13 and builder image to 1.26.6", false},
		{"fix: report cache write failures", false},
		{"refactor(#306): derive FHIR version list from one table", false},
		{"perf: skip unchanged files faster", false},
		{"build(deps): bump actions/attest-build-provenance", false},

		// Near misses that must not be swallowed: the prefix has to be the
		// whole word, not the start of one.
		{"choreography: something entirely different", false},
		{"citation: not a ci commit", false},
		{"testing-tools: add a helper", false},
	}

	for _, tc := range cases {
		got := anyMatch(t, patterns, tc.subject)
		if got != tc.exclude {
			verb := "should have been excluded"
			if !tc.exclude {
				verb = "should have stayed in the notes"
			}
			t.Errorf("%q %s", tc.subject, verb)
		}
	}
}

func anyMatch(t *testing.T, patterns []string, subject string) bool {
	t.Helper()
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			t.Fatalf("pattern %q does not compile: %v", p, err)
		}
		if re.MatchString(subject) {
			return true
		}
	}
	return false
}

// loadChangelogExcludes reads the exclusion patterns straight out of
// .goreleaser.yml, so the test cannot drift from the file it is meant to guard.
func loadChangelogExcludes(t *testing.T) []string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", ".goreleaser.yml"))
	if err != nil {
		t.Fatalf("reading .goreleaser.yml: %v", err)
	}

	var cfg struct {
		Changelog struct {
			Filters struct {
				Exclude []string `yaml:"exclude"`
			} `yaml:"filters"`
		} `yaml:"changelog"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parsing .goreleaser.yml: %v", err)
	}

	if len(cfg.Changelog.Filters.Exclude) == 0 {
		t.Fatal("no changelog exclusion patterns found — the config shape changed")
	}
	return cfg.Changelog.Filters.Exclude
}
