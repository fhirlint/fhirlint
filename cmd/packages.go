package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fhirlint/fhirlint/internal/fhirpkg"
)

var (
	flagPackagesJSON bool
	flagPackagesSort string
	flagTreeDepth    int
)

var packagesCmd = &cobra.Command{
	Use:   "packages",
	Short: "Show the FHIR package cache",
	Long: `List the IG packages installed in the FHIR package cache (~/.fhir/packages).

This is the cache the validator JAR populates and the rest of the FHIR toolchain
shares, so fhirlint only reads it — nothing here modifies or removes anything.

Versions of the same package are grouped together, including packages published
under different capitalisations (KBV.Basis and kbv.basis are the same package
upstream, and both end up on disk).`,
	Example: `  # What is installed, largest first
  fhirlint packages --sort size

  # Machine-readable
  fhirlint packages --json`,
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, _ []string) error {
		return runPackages()
	},
}

var packagesTreeCmd = &cobra.Command{
	Use:   "tree <name#version>",
	Short: "Show a cached package's dependency tree",
	Long: `Resolve a package's dependencies from the manifests in the FHIR package cache.

Reads only what is already on disk, so it works offline and is the preflight for
--offline: it says up front which packages a run would need and which of them are
missing, instead of failing one package at a time during validation.

Dependencies declared as a range rather than an exact version (for example
"1.5.x") are marked, because the manifest alone does not determine which version
a run resolves to.`,
	Example:      `  fhirlint packages tree de.medizininformatikinitiative.kerndatensatz.diagnose#2025.0.1`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return runPackagesTree(args[0])
	},
}

func init() {
	packagesCmd.Flags().BoolVar(&flagPackagesJSON, "json", false, "Output as JSON")
	packagesCmd.Flags().StringVar(&flagPackagesSort, "sort", "name", "Sort order: name or size")
	packagesTreeCmd.Flags().BoolVar(&flagPackagesJSON, "json", false, "Output as JSON")
	packagesTreeCmd.Flags().IntVar(&flagTreeDepth, "depth", 0, "Maximum depth to show (0 = unlimited)")
	packagesCmd.AddCommand(packagesTreeCmd)
}

func runPackages() error {
	// Sizes mean walking 60-odd package directories; skip that work when the
	// output is not sorted by it and JSON was not asked for.
	withSizes := true
	pkgs, err := fhirpkg.List(withSizes)
	if err != nil {
		if errors.Is(err, fhirpkg.ErrNoCache) {
			fmt.Printf("No FHIR package cache yet.\n" +
				"It is created the first time a validation resolves an IG package.\n")
			return nil
		}
		return err
	}

	groups := fhirpkg.Grouped(pkgs)
	switch flagPackagesSort {
	case "size":
		sort.SliceStable(groups, func(i, j int) bool { return groups[i].Bytes > groups[j].Bytes })
	case "name":
		// fhirpkg.List already sorted by folded name.
	default:
		return fmt.Errorf("unknown --sort %q: use name or size", flagPackagesSort)
	}

	if flagPackagesJSON {
		return json.NewEncoder(os.Stdout).Encode(packagesJSON(groups))
	}
	printPackages(groups)
	return nil
}

type packageJSONEntry struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	ID           string   `json:"id"`
	Title        string   `json:"title,omitempty"`
	FHIRVersions []string `json:"fhirVersions,omitempty"`
	Canonical    string   `json:"canonical,omitempty"`
	Bytes        int64    `json:"bytes"`
	Dir          string   `json:"dir"`
}

func packagesJSON(groups []fhirpkg.Group) map[string]any {
	var out []packageJSONEntry
	var total int64
	for _, g := range groups {
		for _, p := range g.Versions {
			out = append(out, packageJSONEntry{
				Name: p.Name, Version: p.Version, ID: p.ID, Title: p.Title,
				FHIRVersions: p.FHIRVersions, Canonical: p.Canonical,
				Bytes: p.Bytes, Dir: p.Dir,
			})
			total += p.Bytes
		}
	}
	if out == nil {
		out = []packageJSONEntry{}
	}
	return map[string]any{"packages": out, "count": len(out), "bytes": total}
}

func printPackages(groups []fhirpkg.Group) {
	var total int64
	var count int
	for _, g := range groups {
		total += g.Bytes
		count += len(g.Versions)
	}
	fmt.Printf("FHIR package cache — %d package(s), %s\n\n", count, humanBytes(total))

	for _, g := range groups {
		note := ""
		if g.MixedCase {
			// Two spellings of one upstream package means two copies on disk.
			note = "  (installed under more than one spelling)"
		}
		fmt.Printf("%s%s\n", g.Name, note)
		for _, p := range g.Versions {
			fhir := strings.Join(p.FHIRVersions, ", ")
			if fhir == "" {
				fhir = "—"
			}
			line := fmt.Sprintf("  %-14s %9s  FHIR %-8s", p.Version, humanBytes(p.Bytes), fhir)
			if g.MixedCase {
				// Say which spelling this one is, or the note above is unactionable.
				line += "  " + p.Name
			}
			fmt.Println(strings.TrimRight(line, " "))
		}
	}
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%d MB", n/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
