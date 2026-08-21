//go:build registry

// This file checks the built-in alias table against the live FHIR package
// registry. It is behind a build tag because the rest of the suite is offline
// by design — `go test ./...` must not depend on a community-run registry
// being up. Run it by hand when touching a pin:
//
//	go test -tags registry -v ./internal/profiles/
//
// CI runs it weekly (.github/workflows/alias-monitor.yml), which is what turns
// a rotting pin into an issue instead of into a user's bug report (#336).
package profiles_test

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/fhirlint/fhirlint/internal/igaudit"
	"github.com/fhirlint/fhirlint/internal/profiles"
)

// auditTimeout bounds the whole sweep. igaudit gives each request five seconds
// and runs four at a time, so this is generous for a table of this size.
const auditTimeout = 60 * time.Second

// aliasIndex returns every distinct package reference in the alias table, and a
// reverse index from reference to the aliases naming it. Findings are reported
// per alias: "kbv.basis#1.5.0 is outdated" is only actionable once you know
// which alias to edit, and two aliases can share one package.
func aliasIndex() (ids []string, byID map[string][]string) {
	byID = map[string][]string{}
	for alias, pkgs := range profiles.Aliases {
		for _, pkg := range pkgs {
			byID[pkg] = append(byID[pkg], alias)
		}
	}
	for id, aliases := range byID {
		sort.Strings(aliases)
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, byID
}

// auditAliases runs the registry audit once for a test. A registry that cannot
// be reached at all skips rather than fails: an outage is not evidence about
// any pin, and a monitor that cries wolf during downtime gets muted.
func auditAliases(t *testing.T) ([]igaudit.PackageReport, map[string][]string) {
	t.Helper()

	ids, byID := aliasIndex()
	ctx, cancel := context.WithTimeout(context.Background(), auditTimeout)
	defer cancel()

	report := igaudit.Audit(ctx, igaudit.NewClient(), ids)
	if report.Errors() == len(report.Packages) {
		t.Skipf("registry unreachable: none of the %d alias packages could be checked", len(ids))
	}
	for _, p := range report.Packages {
		if p.Error != "" {
			t.Logf("could not check %s (%s): %s", p.ID, named(byID, p.ID), p.Error)
		}
	}
	return report.Packages, byID
}

func named(byID map[string][]string, id string) string {
	return strings.Join(byID[id], ", ")
}

// TestAliasesResolve is the one that matters: a pin the registry does not serve
// cannot work for anyone with a cold package cache, whatever else is true of
// it. Deprecation counts here too — it is upstream saying the package is not
// the thing to point new users at.
func TestAliasesResolve(t *testing.T) {
	packages, byID := auditAliases(t)

	for _, p := range packages {
		switch {
		case p.NotFound:
			t.Errorf("alias %s → %s: the registry has no such package", named(byID, p.ID), p.ID)
		case p.VersionMissing:
			t.Errorf("alias %s → %s: the package exists, but the registry has no such version", named(byID, p.ID), p.ID)
		case p.Deprecated:
			note := p.DeprecationNote
			if note == "" {
				note = "no reason given"
			}
			t.Errorf("alias %s → %s: deprecated upstream (%s)", named(byID, p.ID), p.ID, note)
		}
	}
}

// TestAliasesUpToDate is the softer signal, kept separate so the monitor can
// tell "nobody can use this alias" apart from "there is a newer release". A
// stale pin is a decision to revisit, not a breakage: IG pins are deliberate.
func TestAliasesUpToDate(t *testing.T) {
	packages, byID := auditAliases(t)

	for _, p := range packages {
		switch {
		case p.Outdated:
			t.Errorf("alias %s → %s: registry latest is %s", named(byID, p.ID), p.ID, p.Latest)
		case p.Differs:
			// FHIR IG versions are not reliably semver, so igaudit reports the
			// ones it could not order rather than guessing which is newer.
			t.Errorf("alias %s → %s: registry latest is %s, and the two could not be ordered — check by hand",
				named(byID, p.ID), p.ID, p.Latest)
		case p.Ahead:
			t.Logf("alias %s → %s is ahead of the registry's latest (%s), which is normal for a pre-release pin",
				named(byID, p.ID), p.ID, p.Latest)
		}
	}
}
