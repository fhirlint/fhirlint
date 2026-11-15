// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build race

package scaling

// The race detector slows everything down unevenly; the scaling test skips.
// gobco type-checks every file regardless of build tags, so the flag is a
// var declared once in race_flag_test.go and set here (#617).
func init() { raceEnabled = true }
