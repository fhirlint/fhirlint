package fhirpkg

import (
	"strconv"
	"strings"
)

// CompareVersions orders two package versions, returning -1, 0 or 1 along with
// whether the comparison could be made at all.
//
// FHIR IG versions are usually semver ("1.5.0", "2025.0.0", "1.00.000"), but
// the registry does not enforce it and publishers do deviate. Rather than
// guess, a version is only ordered when its core is at least two dot-separated
// integers, with an optional pre-release suffix. Everything else is reported as
// incomparable and the caller says "differs" instead of "outdated".
//
// The two-segment floor is what keeps a scheme change from being read as a
// version bump: "2025-Q1" would otherwise parse as core 2025 with pre-release
// Q1 and rank above "1.0.0", turning "this publisher started versioning by
// quarter" into a confident "you are 2024 releases behind". Single-integer
// schemes such as "20250101" are rejected for the same reason. Being told two
// versions differ is a weaker statement than being told which is newer, but it
// is never the wrong one.
//
// Pre-release handling follows semver only as far as it is unambiguous: a
// release outranks the same core version with a pre-release suffix. Two
// different pre-releases of the same core version are ordered lexically, which
// is an approximation and part of why this returns an ok flag at all.
func CompareVersions(a, b string) (int, bool) {
	aCore, aPre := splitPreRelease(a)
	bCore, bPre := splitPreRelease(b)

	aSeg, ok := numericSegments(aCore)
	if !ok {
		return 0, false
	}
	bSeg, ok := numericSegments(bCore)
	if !ok {
		return 0, false
	}

	for i := 0; i < len(aSeg) || i < len(bSeg); i++ {
		av, bv := 0, 0
		if i < len(aSeg) {
			av = aSeg[i]
		}
		if i < len(bSeg) {
			bv = bSeg[i]
		}
		if av != bv {
			return sign(av - bv), true
		}
	}

	switch {
	case aPre == "" && bPre == "":
		return 0, true
	case aPre == "":
		return 1, true // release beats pre-release
	case bPre == "":
		return -1, true
	case aPre == bPre:
		return 0, true
	case aPre < bPre:
		return -1, true
	default:
		return 1, true
	}
}

func splitPreRelease(v string) (core, pre string) {
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		return v[:i], v[i+1:]
	}
	return v, ""
}

// minVersionSegments is the two-segment floor described on CompareVersions.
const minVersionSegments = 2

func numericSegments(core string) ([]int, bool) {
	parts := strings.Split(core, ".")
	if len(parts) < minVersionSegments {
		return nil, false
	}
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

func sign(n int) int {
	if n < 0 {
		return -1
	}
	return 1
}
