package main

import (
	"fmt"
	"strings"
)

// bech32Charset is the alphabet bech32 data characters are drawn from.
// A vanity pattern can only ever match using these characters.
const bech32Charset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

// Pattern describes what a candidate ZTS string must satisfy. Matching is
// always done against the part of the address after the "zts1" prefix,
// since the prefix itself never varies.
type Pattern struct {
	Prefix   string
	Suffix   string
	Contains string
}

func (p Pattern) Empty() bool {
	return p.Prefix == "" && p.Suffix == "" && p.Contains == ""
}

func (p Pattern) Match(zts string) bool {
	body := strings.TrimPrefix(zts, "zts1")
	if p.Prefix != "" && !strings.HasPrefix(body, p.Prefix) {
		return false
	}
	if p.Suffix != "" && !strings.HasSuffix(body, p.Suffix) {
		return false
	}
	if p.Contains != "" && !strings.Contains(body, p.Contains) {
		return false
	}
	return true
}

// ValidateBech32 reports an error if s contains a character bech32 data can
// never produce (the pattern could then never match anything).
func ValidateBech32(s string) error {
	for _, r := range s {
		if !strings.ContainsRune(bech32Charset, r) {
			return fmt.Errorf("character %q is not valid in a bech32 address (valid: %s)", r, bech32Charset)
		}
	}
	return nil
}

// EstimatedAttempts returns a rough estimate of how many candidates must be
// tried, on average, before a match is found.
func (p Pattern) EstimatedAttempts() float64 {
	n := len(p.Prefix) + len(p.Suffix)
	if n == 0 {
		n = len(p.Contains)
		if n == 0 {
			return 1
		}
		// "contains" can start anywhere in the ~16-char body, so it's cheaper
		// than an anchored prefix/suffix of the same length.
		const bodyLen = 16
		base := 1.0
		for i := 0; i < n; i++ {
			base *= 32
		}
		return base / float64(bodyLen)
	}
	base := 1.0
	for i := 0; i < n; i++ {
		base *= 32
	}
	return base
}
