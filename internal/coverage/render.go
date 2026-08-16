package coverage

import (
	"encoding/json"
	"fmt"
	"strings"
)

// maxListed caps how many missing elements are printed per profile. A profile
// with sixty untouched elements has a problem the first few already convey, and
// the full list is one flag away.
const maxListed = 10

// Terminal renders the report as human-readable text.
func Terminal(rep Report, verbose bool) string {
	var b strings.Builder
	w := &b

	if len(rep.Profiles) == 0 {
		fmt.Fprintln(w, "No profile with mustSupport elements matched the scanned resources.")
		writeAttribution(w, rep)
		writeSkipped(w, rep)
		return b.String()
	}

	for _, p := range rep.Profiles {
		fmt.Fprintf(w, "%s  (%s)\n", p.Name, p.Type)
		fmt.Fprintf(w, "  resources:      %d", p.Resources)
		if p.ByType > 0 {
			fmt.Fprintf(w, "  (%d matched by resource type, not by meta.profile)", p.ByType)
		}
		fmt.Fprintln(w)

		if p.Measurable() == 0 {
			fmt.Fprintf(w, "  must-support:   0/0 measurable of %d\n", len(p.Elements))
		} else {
			fmt.Fprintf(w, "  must-support:   %d/%d  (%.0f%%)\n", p.Covered(), p.Measurable(), p.Percent())
		}

		if missing := p.Missing(); len(missing) > 0 {
			fmt.Fprintln(w, "  never populated:")
			shown := missing
			if !verbose && len(shown) > maxListed {
				shown = shown[:maxListed]
			}
			for _, id := range shown {
				fmt.Fprintf(w, "    %s\n", id)
			}
			if len(shown) < len(missing) {
				fmt.Fprintf(w, "    … and %d more (use --verbose for the full list)\n", len(missing)-len(shown))
			}
		}

		if n := p.Unresolved(); n > 0 {
			reasons := unresolvedReasons(p)
			if len(reasons) == 1 {
				fmt.Fprintf(w, "  not measurable: %d element(s) — %s\n", n, reasons[0])
			} else {
				fmt.Fprintf(w, "  not measurable: %d element(s)\n", n)
				for _, r := range reasons {
					fmt.Fprintf(w, "      %s\n", r)
				}
			}
		}
		for _, warn := range p.Warnings {
			fmt.Fprintf(w, "  ! %s\n", warn)
		}
		fmt.Fprintln(w)
	}

	writeAttribution(w, rep)
	writeSkipped(w, rep)
	return b.String()
}

// unresolvedReasons returns the distinct reasons a profile's elements could not
// be measured, in the order they first appear.
//
// Collapsing them to one generic sentence was the first attempt and it threw
// away the useful half: on a real profile the reasons name different value
// sets, and "which value set" is what tells the reader whether the gap is
// inherent or a package they forgot to install.
func unresolvedReasons(p ProfileReport) []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range p.Elements {
		if !e.Unresolved || e.Reason == "" || seen[e.Reason] {
			continue
		}
		seen[e.Reason] = true
		out = append(out, e.Reason)
	}
	if len(out) == 0 {
		return []string{"slice membership could not be determined"}
	}
	return out
}

func writeAttribution(w *strings.Builder, rep Report) {
	fmt.Fprintf(w, "%d resource(s) scanned", rep.ResourcesScanned)
	if rep.Unattributed > 0 {
		fmt.Fprintf(w, ", %d not attributed to any profile", rep.Unattributed)
	}
	fmt.Fprintln(w)
	if rep.ProfilesWithoutResources > 0 {
		fmt.Fprintf(w, "%d profile(s) had no matching resource and were not measured\n",
			rep.ProfilesWithoutResources)
	}
	if rep.Unattributed > 0 && rep.Unattributed == rep.ResourcesScanned {
		fmt.Fprintln(w, "hint: resources declare no matching meta.profile — name the profile with --profile to measure them anyway")
	}
}

func writeSkipped(w *strings.Builder, rep Report) {
	if len(rep.SkippedFiles) == 0 {
		return
	}
	fmt.Fprintf(w, "%d file(s) skipped:\n", len(rep.SkippedFiles))
	for i, s := range rep.SkippedFiles {
		if i == maxListed {
			fmt.Fprintf(w, "  … and %d more\n", len(rep.SkippedFiles)-maxListed)
			break
		}
		fmt.Fprintf(w, "  %s — %s\n", s.Path, s.Reason)
	}
}

// jsonProfile adds the derived figures to the wire format, so a consumer does
// not have to recompute what the terminal output already states.
type jsonProfile struct {
	ProfileReport
	Covered    int     `json:"covered"`
	Measurable int     `json:"measurable"`
	Unresolved int     `json:"unresolvedCount"`
	Percent    float64 `json:"percent"`
}

type jsonReport struct {
	Profiles         []jsonProfile `json:"profiles"`
	ResourcesScanned int           `json:"resourcesScanned"`
	Unattributed     int           `json:"unattributed,omitempty"`
	SkippedFiles     []SkippedFile `json:"skippedFiles,omitempty"`
}

// JSON renders the machine-readable report.
func JSON(rep Report) ([]byte, error) {
	out := jsonReport{
		Profiles:         make([]jsonProfile, 0, len(rep.Profiles)),
		ResourcesScanned: rep.ResourcesScanned,
		Unattributed:     rep.Unattributed,
		SkippedFiles:     rep.SkippedFiles,
	}
	for _, p := range rep.Profiles {
		out.Profiles = append(out.Profiles, jsonProfile{
			ProfileReport: p,
			Covered:       p.Covered(),
			Measurable:    p.Measurable(),
			Unresolved:    p.Unresolved(),
			Percent:       round1(p.Percent()),
		})
	}
	return json.MarshalIndent(out, "", "  ")
}

func round1(f float64) float64 {
	return float64(int(f*10+0.5)) / 10
}

// LowestPercent returns the worst coverage in the report, used by
// --min-coverage. A report with no profiles returns 100: there is nothing to
// fail on, and failing a build over the absence of measurable profiles would
// punish the wrong thing.
func LowestPercent(rep Report) float64 {
	lowest := 100.0
	for _, p := range rep.Profiles {
		if p.Measurable() == 0 {
			continue
		}
		if p.Percent() < lowest {
			lowest = p.Percent()
		}
	}
	return lowest
}

// FilterProfiles keeps the definitions whose URL, id or name matches one of the
// given selectors. A selector may be a canonical URL, a profile id, or a name.
func FilterProfiles(sds []*StructureDefinition, selectors []string) []*StructureDefinition {
	if len(selectors) == 0 {
		return sds
	}
	want := make(map[string]bool, len(selectors))
	for _, s := range selectors {
		want[strings.ToLower(s)] = true
	}
	var out []*StructureDefinition
	for _, sd := range sds {
		if want[strings.ToLower(sd.URL)] || want[strings.ToLower(sd.ID)] || want[strings.ToLower(sd.Name)] {
			out = append(out, sd)
		}
	}
	return out
}
