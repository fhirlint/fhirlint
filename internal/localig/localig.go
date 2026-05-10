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

	for _, src := range paths {
		dst := filepath.Join(tmpDir, filepath.Base(src))
		if err := copyFile(src, dst); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("copying %s: %w", src, err)
		}
	}

	return tmpDir, cleanup, nil
}

// corePackageName returns the hl7.fhir.rX.core package name for the given FHIR version.
func corePackageName(fhirVersion string) string {
	switch fhirVersion {
	case "4.3.0":
		return "hl7.fhir.r4b.core"
	case "5.0.0":
		return "hl7.fhir.r5.core"
	default:
		return "hl7.fhir.r4.core"
	}
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
