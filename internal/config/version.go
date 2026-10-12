package config

import (
	"strconv"
	"strings"
)

// CompareVersions compares two dotted version strings, returning -1, 0, or 1. A
// leading "v" is ignored. Numeric segments sort numerically, a numeric segment
// sorts before a non-numeric one, and otherwise segments compare lexically.
func CompareVersions(a, b string) int {
	as := splitVersion(a)
	bs := splitVersion(b)
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		av, bv := "0", "0"
		if i < len(as) && as[i] != "" {
			av = as[i]
		}
		if i < len(bs) && bs[i] != "" {
			bv = bs[i]
		}
		if c := compareSegment(av, bv); c != 0 {
			return c
		}
	}
	return 0
}

func splitVersion(v string) []string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	return strings.Split(v, ".")
}

func compareSegment(a, b string) int {
	an, aok := strconv.Atoi(a)
	bn, bok := strconv.Atoi(b)
	switch {
	case aok == nil && bok == nil:
		if an < bn {
			return -1
		}
		if an > bn {
			return 1
		}
		return 0
	case aok == nil && bok != nil:
		return -1
	case aok != nil && bok == nil:
		return 1
	default:
		return strings.Compare(a, b)
	}
}
