// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

package validator

import (
	"regexp"
	"sort"
)

// intlEditionClash is the edition clash --snomed-edition resolves (#734): a
// ValueSet include without a version ('') made the validator assume the
// international edition, and the coding states another one. It is the text of
// VALUESET_VALUE_MISMATCH_CHANGED, which also arrives as the tail of
// Terminology_TX_Confirm_4a on a required binding. Matched on the text, not
// the id, for that reason. The template has no translation in 6.10.4 or
// 7.0.6, so the English text also stands in a run with --locale de.
//
// VALUESET_VALUE_MISMATCH (an include that pins the international edition)
// and VALUESET_VALUE_MISMATCH_DEFAULT do not match: -sct does not change
// either.
var intlEditionClash = regexp.MustCompile(`version 'http://snomed\.info/sct/900000000000207008' ` +
	`resulting from the version '' in the ValueSet include is different to the one in the value ` +
	`\('http://snomed\.info/sct/(\d+)(?:/version/[^']*)?'\)`)

const intlModule = "900000000000207008"

// snomedEditionByModule is snomedEditions the other way round. Where two
// names share a module (ca, ca-fr), the shorter wins, then the first in order.
var snomedEditionByModule = func() map[string]string {
	names := SnomedEditionNames()
	sort.SliceStable(names, func(i, j int) bool { return len(names[i]) < len(names[j]) })
	m := make(map[string]string, len(names))
	for _, n := range names {
		if _, ok := m[snomedEditions[n]]; !ok {
			m[snomedEditions[n]] = n
		}
	}
	return m
}()

// SnomedEditionAdvice is the --snomed-edition that would have avoided iss, as
// the value to pass: an edition name, or the module id when no name maps to
// it. Empty when iss is not the clash.
func SnomedEditionAdvice(iss Issue) string {
	m := intlEditionClash.FindStringSubmatch(iss.Message)
	if m == nil || m[1] == intlModule {
		return ""
	}
	if name, ok := snomedEditionByModule[m[1]]; ok {
		return name
	}
	return m[1]
}

// AnnotateSnomedEdition sets Advice on every issue that the edition clash
// caused. A caller that passed an edition of its own does not call it: the
// validator then assumed what it was told.
func AnnotateSnomedEdition(results []*Result) {
	for _, r := range results {
		if r == nil {
			continue
		}
		for i := range r.Issues {
			iss := &r.Issues[i]
			if iss.Redacted || iss.Advice != "" {
				continue
			}
			if edition := SnomedEditionAdvice(*iss); edition != "" {
				iss.Advice = "the validator assumed the international SNOMED CT edition, the coding states another: " +
					"add --snomed-edition " + edition
			}
		}
	}
}
