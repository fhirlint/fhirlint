// Copyright The fhirlint Authors
// SPDX-License-Identifier: Apache-2.0

//go:build race

package scaling

// The race detector slows everything down unevenly; the scaling test skips.
const raceEnabled = true
