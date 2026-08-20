package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fhirlint/fhirlint/internal/coverage"
	"github.com/fhirlint/fhirlint/internal/iglock"
	"github.com/fhirlint/fhirlint/internal/profiles"
	"github.com/spf13/cobra"
)

var (
	flagCoverageIG          []string
	flagCoverageProfile     []string
	flagCoverageFormat      string
	flagCoverageOutput      string
	flagCoverageMinCoverage float64
	flagCoverageVerbose     bool
	flagCoverageExclude     []string
	flagCoverageOffline     bool
)

// coveragePackageTimeout bounds fetching one IG package, metadata and archive
// together.
const coveragePackageTimeout = 5 * time.Minute

var coverageCmd = &cobra.Command{
	Use:   "coverage [path]",
	Short: "Report which mustSupport elements of a profile your resources actually populate",
	Long: `Measure how much of a profile a set of resources exercises.

The validator checks one resource at a time and only inspects what is there, so
it cannot answer the question this command exists for: across a whole example
set, which parts of the profile has nobody ever populated? An IG can ship thirty
green examples and still leave half its mustSupport elements untouched.

Profiles are read from IG packages in the local FHIR package cache
(~/.fhir/packages). A package that is not there yet is downloaded from the FHIR
package registry and verified against the checksum the registry publishes.
Use --offline to forbid that and fail instead.

Resources are attributed to a profile by their meta.profile. Naming profiles
with --profile also allows resources of the right resource type that declare no
profile at all to be measured against them.

  fhirlint coverage ./examples/ --ig de.gematik.isik-basismodul#4.0.3
  fhirlint coverage ./examples/ --ig kbv.basis#1.9.0 --profile KBV_PR_Base_Patient
  fhirlint coverage ./examples/ --ig isik#4.0.3 --min-coverage 80

With no path, the current directory is used.`,
	Args:         cobra.MaximumNArgs(1),
	RunE:         runCoverage,
	SilenceUsage: true,
}

func init() {
	coverageCmd.Flags().StringArrayVar(&flagCoverageIG, "ig", nil,
		"IG package to read profiles from, e.g. kbv.basis#1.9.0 (repeatable, required)")
	coverageCmd.Flags().StringArrayVarP(&flagCoverageProfile, "profile", "p", nil,
		"Limit to these profiles: canonical URL, id or name (repeatable)")
	coverageCmd.Flags().StringVarP(&flagCoverageFormat, "format", "f", "terminal",
		"Output format: terminal, json")
	coverageCmd.Flags().StringVarP(&flagCoverageOutput, "output", "o", "",
		"Output file (stdout if omitted)")
	coverageCmd.Flags().Float64Var(&flagCoverageMinCoverage, "min-coverage", -1,
		"Exit non-zero when any profile falls below this percentage (-1 disables)")
	coverageCmd.Flags().BoolVar(&flagCoverageVerbose, "verbose", false,
		"List every unpopulated element instead of the first few")
	coverageCmd.Flags().StringArrayVar(&flagCoverageExclude, "exclude", nil,
		"Glob pattern to exclude (repeatable)")
	coverageCmd.Flags().BoolVar(&flagCoverageOffline, "offline", false,
		"Never contact the package registry — fail if an IG package is not already cached")

	_ = coverageCmd.RegisterFlagCompletionFunc("format", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"terminal", "json"}, cobra.ShellCompDirectiveNoFileComp
	})
}

func runCoverage(_ *cobra.Command, args []string) error {
	if len(flagCoverageIG) == 0 {
		return fmt.Errorf("--ig is required: name the IG package holding the profiles, e.g. --ig kbv.basis#1.9.0")
	}

	root := "."
	if len(args) == 1 {
		root = args[0]
	}

	reg, requested, err := loadCoverageRegistry(flagCoverageIG)
	if err != nil {
		return err
	}

	selected := coverage.FilterProfiles(reg.ProfilesFrom(requested...), flagCoverageProfile)
	if len(selected) == 0 {
		if len(flagCoverageProfile) > 0 {
			return fmt.Errorf("none of the named profiles were found in %v — check the URL, id or name", flagCoverageIG)
		}
		return fmt.Errorf("no profiles found in %v", flagCoverageIG)
	}

	resources, skipped, err := coverage.LoadResources(root, flagCoverageExclude)
	if err != nil {
		return fmt.Errorf("reading %s: %w", root, err)
	}
	if len(resources) == 0 {
		return fmt.Errorf("no JSON or NDJSON resources found in %s", root)
	}

	rep := coverage.Run(reg, selected, resources, coverage.Options{
		// Naming profiles is a statement about what the data is meant to be, so
		// it licenses measuring resources that declare no profile of their own.
		AttributeByType: len(flagCoverageProfile) > 0,
	})
	rep.SkippedFiles = skipped

	if err := writeCoverage(rep); err != nil {
		return err
	}

	if flagCoverageMinCoverage >= 0 {
		if lowest := coverage.LowestPercent(rep); lowest < flagCoverageMinCoverage {
			return fmt.Errorf("coverage %.0f%% is below the required %.0f%%", lowest, flagCoverageMinCoverage)
		}
	}
	return nil
}

// loadCoverageRegistry loads the named packages plus everything else already in
// the cache.
//
// The extra packages are what makes slice resolution work in practice: German
// profiles delegate datatype constraints to other packages — a HumanName to
// humanname-de-basis, say — and the slice definitions live there. Named
// packages are loaded first so they win any canonical URL collision.
func loadCoverageRegistry(igs []string) (*coverage.Registry, []string, error) {
	cacheRoot, err := coverage.DefaultCacheRoot()
	if err != nil {
		return nil, nil, err
	}

	reg := coverage.NewRegistry()
	var requested []string
	for _, ig := range igs {
		for _, ref := range profiles.Resolve(ig) {
			name, version := iglock.ParseIGID(ref)
			if name == "" {
				return nil, nil, fmt.Errorf("--ig %q must be name#version, e.g. kbv.basis#1.9.0", ig)
			}
			label := name + "#" + version
			dir := coverage.PackageDir(cacheRoot, name, version)
			if _, err := os.Stat(dir); err != nil {
				if flagCoverageOffline {
					return nil, nil, &coverage.ErrPackageNotCached{ID: label, Dir: dir}
				}
				if err := downloadPackage(cacheRoot, name, version); err != nil {
					return nil, nil, err
				}
			}
			if _, err := reg.LoadPackage(dir, label); err != nil {
				return nil, nil, fmt.Errorf("reading package %s: %w", label, err)
			}
			requested = append(requested, label)
		}
	}

	// Supporting packages, best effort: a missing one only means some slices
	// end up unresolved, which the report states rather than hides.
	entries, err := os.ReadDir(cacheRoot)
	if err != nil {
		return reg, requested, nil //nolint:nilerr // the named packages loaded; the rest is a bonus
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		_, _ = reg.LoadPackage(filepath.Join(cacheRoot, e.Name(), "package"), e.Name())
	}
	return reg, requested, nil
}

// downloadPackage fetches a package the cache does not hold yet.
//
// Progress goes to stderr rather than stdout: the report is what stdout is for,
// and a pipeline redirecting it should not find a download notice in its JSON.
func downloadPackage(cacheRoot, name, version string) error {
	fmt.Fprintf(os.Stderr, "Downloading IG package %s#%s …\n", name, version)

	ctx, cancel := context.WithTimeout(context.Background(), coveragePackageTimeout)
	defer cancel()

	warnings, err := coverage.NewDownloader().Fetch(ctx, cacheRoot, name, version)
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}
	return err
}

func writeCoverage(rep coverage.Report) error {
	var body string
	switch flagCoverageFormat {
	case "json":
		data, err := coverage.JSON(rep)
		if err != nil {
			return err
		}
		body = string(data) + "\n"
	case "terminal", "":
		body = coverage.Terminal(rep, flagCoverageVerbose)
	default:
		return fmt.Errorf("unknown format %q — use: terminal, json", flagCoverageFormat)
	}

	if flagCoverageOutput == "" {
		fmt.Print(body)
		return nil
	}
	return os.WriteFile(flagCoverageOutput, []byte(body), 0600)
}
