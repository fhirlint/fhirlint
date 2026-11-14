// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/validator"
)

func TestReportImplicitPackages(t *testing.T) {
	var w strings.Builder
	reportImplicitPackages(&w, []validator.PackageRef{
		{ID: "hl7.fhir.us.core", Version: "9.0.0"},
		{ID: "hl7.fhir.uv.extensions.r4", Version: "5.3.0-ballot-tc1", PreRelease: true},
	}, []string{"hl7.fhir.us.core#6.1.0", "./local-ig"})
	out := w.String()
	for _, want := range []string{
		"loaded 2 package(s)",
		"1 of them are pre-releases: hl7.fhir.uv.extensions.r4#5.3.0-ballot-tc1",
		"loaded hl7.fhir.us.core#9.0.0 although hl7.fhir.us.core#6.1.0 was requested",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}

	w.Reset()
	reportImplicitPackages(&w, nil, nil)
	if w.Len() != 0 {
		t.Errorf("no implicit packages printed %q", w.String())
	}
}
