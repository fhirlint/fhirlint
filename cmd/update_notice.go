package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/spf13/viper"
	"golang.org/x/mod/semver"

	"github.com/fhirlint/fhirlint/internal/updatecheck"
	"github.com/fhirlint/fhirlint/internal/validator"
)

// fhirlintRepo is where fhirlint's own releases live.
const fhirlintRepo = "fhirlint/fhirlint"

// latestFhirlintRelease is a var so tests can drive the comparison without a
// network call or a cache file.
var latestFhirlintRelease = updatecheck.Latest

// noUpdateNotifierEnvVar switches the notice off entirely, for users who have
// deliberately pinned a version and do not want to hear about it again.
const noUpdateNotifierEnvVar = "FHIRLINT_NO_UPDATE_NOTIFIER"

// pendingUpdate is one thing that has a newer version available.
type pendingUpdate struct {
	name    string // "fhirlint" / "validator"
	current string
	latest  string
	action  string // what to do about it
}

// printUpdateNotice reports everything that is out of date, as a single block.
//
// One block, not one per component: a user behind on both used to get two
// separate paragraphs after every run, which is twice the noise for the same
// information (#360).
func printUpdateNotice() {
	if updateNoticeSuppressed() {
		return
	}

	var pending []pendingUpdate
	if current, latest, ok := fhirlintUpdate(); ok {
		pending = append(pending, pendingUpdate{
			name:    "fhirlint",
			current: current,
			latest:  latest,
			// Which command is right depends on how fhirlint was installed —
			// brew, go install, a container tag, an archive. Detecting that is
			// #361; until then the releases page is the honest answer.
			action: "https://github.com/" + fhirlintRepo + "/releases",
		})
	}
	if latest := validatorUpdate(); latest != "" {
		pending = append(pending, pendingUpdate{
			name:    "validator",
			current: validatorCurrentVersion(),
			latest:  latest,
			action:  validatorAction(latest),
		})
	}
	if len(pending) == 0 {
		return
	}

	// Align the arrows so two rows read as a table rather than as prose.
	nameWidth, versionWidth := 0, 0
	for _, p := range pending {
		nameWidth = max(nameWidth, len(p.name))
		versionWidth = max(versionWidth, len(p.current+" → "+p.latest))
	}

	var b strings.Builder
	b.WriteString("\nUpdates available:\n")
	for _, p := range pending {
		versions := p.current + " → " + p.latest
		fmt.Fprintf(&b, "  %-*s  %-*s  %s\n", nameWidth, p.name, versionWidth, versions, p.action)
	}
	fmt.Fprint(os.Stderr, b.String())
}

// updateNoticeSuppressed reports whether the notice should be withheld.
//
// Every case here is one where the user either cannot act on the notice or has
// said they do not want it. A courtesy that cannot be acted on is just noise.
func updateNoticeSuppressed() bool {
	// An explicit opt-out, the way gh has GH_NO_UPDATE_NOTIFIER.
	if os.Getenv(noUpdateNotifierEnvVar) != "" {
		return true
	}
	// --offline, or offline: true in the config, said no network.
	if viper.GetBool("offline") {
		return true
	}
	// A pipeline runs a version somebody pinned on purpose and cannot upgrade
	// itself. Most CI systems set CI=true; those that do not are usually caught
	// by the terminal check below.
	if os.Getenv("CI") != "" {
		return true
	}
	// The notice goes to stderr, so that is what has to be a terminal — not
	// stdout. Redirecting a report to a file while watching the run should
	// still show the notice.
	return !isatty.IsTerminal(os.Stderr.Fd()) && !isatty.IsCygwinTerminal(os.Stderr.Fd())
}

// fhirlintUpdate reports whether a newer fhirlint release exists.
//
// It answers only for release builds. fhirlintVersion() falls back to build
// info when the GoReleaser ldflag is absent, yielding "dev", "dev-abc1234" or a
// module pseudo-version — none of which order against a release tag, so there
// is no honest comparison to make and no notice to give.
func fhirlintUpdate() (current, latest string, ok bool) {
	if version == "" {
		return "", "", false
	}
	current = version
	latest = latestFhirlintRelease(fhirlintRepo)
	if latest == "" {
		return "", "", false
	}
	// Compare as versions, not as strings: during a release the newest tag can
	// briefly be ahead of the published release, and telling someone to
	// "upgrade" to an older version would be worse than staying quiet.
	if !semver.IsValid(current) || !semver.IsValid(latest) {
		return "", "", false
	}
	if semver.Compare(latest, current) <= 0 {
		return "", "", false
	}
	return current, latest, true
}

// validatorUpdate reports a newer validator JAR release, or "".
func validatorUpdate() string { return validator.CheckForUpdate() }

// validatorCurrentVersion is the JAR version the notice compares against.
func validatorCurrentVersion() string { return validator.ValidatorVersion() }

// validatorAction is what to do about a pending validator release.
//
// A release that was downloaded and refused here still gets a row: someone
// stuck on an older JAR while a newer one exists should be able to see why,
// rather than have the tool go quiet for reasons it will not explain. What the
// row must stop doing is recommending an action that fails (#377).
func validatorAction(latest string) string {
	reason, when, rejected := validator.JARRejection(latest)
	if !rejected {
		return "run: fhirlint update"
	}
	return fmt.Sprintf("%s here on %s; retry with: fhirlint update",
		reason, when.Format("2006-01-02"))
}
