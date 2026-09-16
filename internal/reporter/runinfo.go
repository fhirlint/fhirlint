package reporter

import (
	"strings"

	"github.com/fhirlint/fhirlint/internal/validator"
)

// RunInfo is the provenance a report carries: what produced the findings in
// it. A report is read long after the run, often by someone who did not make
// it, and the validator changes what it reports from release to release — a
// report that does not say which validator wrote it cannot be compared with
// another, or reproduced (#428).
//
// It is per run, not per file, and every format states it once.
type RunInfo struct {
	// Fhirlint is the fhirlint version.
	Fhirlint string `json:"fhirlint,omitempty"`
	// Validator is the version of the validator JAR, as fhirlint knows it from
	// the cache, the pin or the JAR manifest.
	Validator string `json:"validator,omitempty"`
	// ValidatorBuild is the JAR's own statement of what it is — version, Git
	// SHA and build date — which only validators from 6.10.5 on make. When
	// present it is the better answer, because it is the JAR's, not ours.
	ValidatorBuild string `json:"validatorBuild,omitempty"`
	// FHIRVersion is the FHIR release the run validated against.
	FHIRVersion string `json:"fhirVersion,omitempty"`
}

// WithResults fills ValidatorBuild from the results when the validator stated
// it. Every outcome of a run carries the same line, so the first is enough;
// a value already set (a caller that knows better) is kept.
func (info RunInfo) WithResults(results []*validator.Result) RunInfo {
	if info.ValidatorBuild != "" {
		return info
	}
	for _, r := range results {
		if r != nil && r.ValidatorBuild != "" {
			info.ValidatorBuild = r.ValidatorBuild
			break
		}
	}
	return info
}

// validatorLine is the one-line rendering for formats that have a single
// string slot: the JAR's own statement when it made one, else the version.
func (info RunInfo) validatorLine() string {
	if info.ValidatorBuild != "" {
		return info.ValidatorBuild
	}
	return info.Validator
}

// empty reports whether there is nothing to say.
func (info RunInfo) empty() bool {
	return info == RunInfo{}
}

// provenanceLine renders the run info as one sentence for prose formats,
// naming only what is known: "fhirlint 1.13.0 · validator 6.10.4 · FHIR 4.0.1".
func provenanceLine(info RunInfo) string {
	var parts []string
	if info.Fhirlint != "" {
		parts = append(parts, "fhirlint "+info.Fhirlint)
	}
	if v := info.validatorLine(); v != "" {
		parts = append(parts, "validator "+v)
	}
	if info.FHIRVersion != "" {
		parts = append(parts, "FHIR "+info.FHIRVersion)
	}
	return strings.Join(parts, " · ")
}
