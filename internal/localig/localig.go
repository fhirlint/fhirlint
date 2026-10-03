// Package localig creates a minimal temporary FHIR IG package directory from
// individual FHIR resource files (CodeSystems, ValueSets, etc.).
// The resulting directory can be passed to the HL7 FHIR Validator via -ig.
package localig

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fhirlint/fhirlint/internal/validator"
)

// PackageDir creates a temporary directory containing all provided FHIR resource
// files plus a minimal package.json, suitable for passing to the validator as -ig.
// The caller must invoke the returned cleanup function when the directory is no longer needed.
func PackageDir(paths []string, fhirVersion string) (dir string, cleanup func(), err error) {
	tmpDir, err := os.MkdirTemp("", "fhirlint-local-ig-*")
	if err != nil {
		return "", nil, fmt.Errorf("creating temp IG dir: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(tmpDir) }

	pkg := map[string]interface{}{
		"name":         "fhirlint.local",
		"version":      "0.0.1",
		"type":         "fhir.ig",
		"fhirVersions": []string{fhirVersion},
		"dependencies": map[string]string{corePackageName(fhirVersion): fhirVersion},
	}
	pkgData, _ := json.Marshal(pkg)
	if err := os.WriteFile(filepath.Join(tmpDir, "package.json"), pkgData, 0600); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("writing package.json: %w", err)
	}

	for i, src := range paths {
		dst := filepath.Join(tmpDir, tempName(src, i))
		if err := copyFile(src, dst); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("copying %s: %w", src, err)
		}
	}

	return tmpDir, cleanup, nil
}

// tempName is the name src gets inside the temp IG. The validator loads the
// directory as a plain folder and skips any file whose name ends in
// template.json or template.xml (see SkippedByValidator), and two sources with
// the same base name from different directories would overwrite each other.
// Inserting the position before the extension avoids both while keeping the
// original name readable in the validator's log.
func tempName(src string, i int) string {
	base := filepath.Base(src)
	ext := filepath.Ext(base)
	return fmt.Sprintf("%s.%d%s", strings.TrimSuffix(base, ext), i, ext)
}

// SkippedByValidator returns the files among local -ig entries that the
// validator JAR drops without an error a user would see.
//
// When it loads an IG from a folder or a single file (not a package), its
// IgLoader.loadResourceByVersion rejects every name ending in template.json or
// template.xml with "Unsupported format", and loadFileWithErrorChecking only
// logs that. A profile in such a file is never loaded, so instances claiming it
// pass (#444, hapifhir/org.hl7.fhir.core#2682).
//
// The checks follow IgLoader.loadIgSource: package ids, URLs and entries that do
// not exist are not local folders; a folder holding package.tgz, igpack.zip or
// validator.pack is read as that archive; otherwise only the folder's top level
// is scanned, because fhirlint never passes -recurse, and dot files are ignored.
func SkippedByValidator(igs []string) []string {
	var skipped []string
	for _, ig := range igs {
		info, err := os.Stat(ig)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			if rejectedName(filepath.Base(ig)) {
				skipped = append(skipped, ig)
			}
			continue
		}
		if holdsArchive(ig) {
			continue
		}
		entries, err := os.ReadDir(ig)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			if rejectedName(e.Name()) {
				skipped = append(skipped, filepath.Join(ig, e.Name()))
			}
		}
	}
	return skipped
}

// rejectedName reports whether the validator rejects a file of this name. It
// renames a scanned file's extension to the detected format's ("json"/"xml")
// before the check, so the extension's case does not matter but the stem's
// does: report-template.JSON is rejected, ReportTemplate.json is not.
func rejectedName(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if ext != ".json" && ext != ".xml" {
		return false
	}
	return strings.HasSuffix(strings.TrimSuffix(name, filepath.Ext(name)), "template")
}

func holdsArchive(dir string) bool {
	for _, name := range []string{"package.tgz", "igpack.zip", "validator.pack"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

// corePackageName returns the hl7.fhir.rX.core package name for the given FHIR
// version, falling back to R4 for a version fhirlint does not know — which is
// what this has always answered for anything it did not recognise.
func corePackageName(fhirVersion string) string {
	if v, ok := validator.LookupFHIRVersion(fhirVersion); ok {
		return v.CorePackage
	}
	return "hl7.fhir.r4.core"
}

func copyFile(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // intentional: copying user-specified resource file
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst) //nolint:gosec // intentional: writing to temp dir
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	_, err = io.Copy(out, in)
	return err
}
