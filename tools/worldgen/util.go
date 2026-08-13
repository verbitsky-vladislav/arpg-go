package main

// util.go — мелкие общие помощники.

import (
	"encoding/json"
	"sort"
	"strconv"
)

func jsonMarshalIndent(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	return string(b), err
}

func sortStrings(s []string) { sort.Strings(s) }

func itoa(i int) string { return strconv.Itoa(i) }

func atoi(s string) (int, bool) {
	v, err := strconv.Atoi(s)
	return v, err == nil
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
