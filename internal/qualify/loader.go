package qualify

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed builtin
var builtinFS embed.FS

const expectedSuffix = ".expected.json"

// BuiltinCases materialises the embedded qualification suite into destDir and
// returns the cases (with Path pointing at the written files), ready to validate.
func BuiltinCases(destDir string) ([]Case, error) {
	var cases []Case
	err := fs.WalkDir(builtinFS, "builtin", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !isCaseFile(p) {
			return nil
		}
		data, err := builtinFS.ReadFile(p)
		if err != nil {
			return err
		}
		exp, err := readExpectedFS(builtinFS, expectedPath(p))
		if err != nil {
			return err
		}

		name := strings.TrimPrefix(p, "builtin/")
		dest := filepath.Join(destDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(dest), 0750); err != nil {
			return err
		}
		if err := os.WriteFile(dest, data, 0600); err != nil {
			return err
		}
		cases = append(cases, Case{Name: name, Path: dest, Expected: exp})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sortCases(cases)
	return cases, nil
}

// LoadSuite reads a custom qualification suite from dir: every *.json that is
// not itself an *.expected.json and has a companion *.expected.json file.
func LoadSuite(dir string) ([]Case, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}

	var cases []Case
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !isCaseFile(p) {
			return nil
		}
		expPath := expectedPath(p)
		if _, statErr := os.Stat(expPath); statErr != nil {
			// A FHIR file without a companion expectation is skipped, not an error.
			return nil
		}
		exp, err := readExpectedOS(expPath)
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(dir, p)
		if relErr != nil {
			rel = p
		}
		cases = append(cases, Case{Name: filepath.ToSlash(rel), Path: p, Expected: exp})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("no test cases found in %s (each case needs a companion %s file)", dir, expectedSuffix)
	}
	sortCases(cases)
	return cases, nil
}

// isCaseFile reports whether p is a FHIR case file (.json) and not an expectation.
func isCaseFile(p string) bool {
	return strings.HasSuffix(p, ".json") && !strings.HasSuffix(p, expectedSuffix)
}

// expectedPath returns the companion expectation path for a case file.
func expectedPath(caseFile string) string {
	return strings.TrimSuffix(caseFile, ".json") + expectedSuffix
}

func readExpectedFS(fsys fs.FS, p string) (Expected, error) {
	data, err := fs.ReadFile(fsys, p)
	if err != nil {
		return Expected{}, fmt.Errorf("missing expectation file %s: %w", p, err)
	}
	return parseExpected(data, p)
}

func readExpectedOS(p string) (Expected, error) {
	data, err := os.ReadFile(p) //nolint:gosec // user-supplied suite path
	if err != nil {
		return Expected{}, err
	}
	return parseExpected(data, p)
}

func parseExpected(data []byte, p string) (Expected, error) {
	var exp Expected
	if err := json.Unmarshal(data, &exp); err != nil {
		return Expected{}, fmt.Errorf("parsing %s: %w", p, err)
	}
	return exp, nil
}

func sortCases(cases []Case) {
	sort.Slice(cases, func(i, j int) bool { return cases[i].Name < cases[j].Name })
}
