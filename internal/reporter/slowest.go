// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package reporter

import (
	"fmt"
	"io"
	"sort"

	"github.com/fhirlint/fhirlint/internal/termsafe"
	"github.com/fhirlint/fhirlint/internal/validator"
)

// Slowest lists the n files the validator took longest on, by the time it
// logged per file (#522). Files without a time (cached, crashed) are left out,
// and a note says how many, so a short list is not read as the whole run.
func Slowest(w io.Writer, results []*validator.Result, n int) {
	if n <= 0 {
		return
	}
	var timed []*validator.Result
	untimed := 0
	for _, r := range results {
		if r == nil {
			continue
		}
		if r.DurationMs > 0 {
			timed = append(timed, r)
		} else {
			untimed++
		}
	}
	if len(timed) == 0 {
		_, _ = fmt.Fprintln(w, dimStyle.Render("No validation times: the validator logged none for these files"))
		return
	}
	sort.SliceStable(timed, func(i, j int) bool { return timed[i].DurationMs > timed[j].DurationMs })
	if len(timed) > n {
		timed = timed[:n]
	}
	_, _ = fmt.Fprintf(w, "Slowest files (time the validator logged per file):\n")
	for _, r := range timed {
		_, _ = fmt.Fprintf(w, "  %8.3f s  %s\n", float64(r.DurationMs)/1000, termsafe.Line(label(r)))
	}
	if untimed > 0 {
		_, _ = fmt.Fprintln(w, dimStyle.Render(fmt.Sprintf("  %d file(s) without a time (cached or not validated) are not listed", untimed)))
	}
}
