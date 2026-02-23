package tui

import (
	"strings"
	"unicode"
)

// fuzzyMatch checks whether pattern is a fuzzy match for str and returns a
// score. A higher score indicates a better match. Returns (0, false) if the
// pattern does not match at all.
//
// The algorithm uses two strategies:
//  1. Subsequence match: every character in pattern appears in str in order.
//     This handles abbreviations like "cmpct" → "compact".
//  2. Edit distance: Damerau-Levenshtein distance (insertions, deletions,
//     substitutions, and transpositions). This handles typos like
//     "settigns" → "settings". Matches within a threshold based on the
//     string lengths are accepted.
//
// The better of the two strategies is used. All matching is case-insensitive.
func fuzzyMatch(pattern, str string) (int, bool) {
	pLower := strings.ToLower(pattern)
	sLower := strings.ToLower(str)

	pLen := len(pLower)
	sLen := len(sLower)

	if pLen == 0 {
		return 0, true
	}

	// Try subsequence match first.
	if subScore, ok := subsequenceScore(pLower, sLower, str); ok {
		return subScore, true
	}

	// Fall back to edit distance for typos (transpositions, substitutions, etc.).
	dist := damerauLevenshtein(pLower, sLower)

	// Allow up to ceil(max(pLen, sLen) / 3) edits, minimum 1, maximum 3.
	maxLen := pLen
	if sLen > maxLen {
		maxLen = sLen
	}
	threshold := (maxLen + 2) / 3 // ceil division
	if threshold < 1 {
		threshold = 1
	}
	if threshold > 3 {
		threshold = 3
	}

	if dist > threshold {
		return 0, false
	}

	// Score: higher is better. Base score from string length minus penalty for edits.
	score := maxLen*2 - dist*5

	// Bonus: prefix match (first char matches).
	if pLen > 0 && sLen > 0 && pLower[0] == sLower[0] {
		score += 6
	}

	// Bonus for shorter targets.
	if sLen > pLen {
		score -= (sLen - pLen)
	}

	// Exact match bonus.
	if pLower == sLower {
		score += 20
	}

	if score <= 0 {
		score = 1 // ensure positive score for accepted matches
	}

	return score, true
}

// subsequenceScore checks if pattern is a subsequence of str and returns a
// score. Returns (0, false) if pattern is not a subsequence.
func subsequenceScore(pLower, sLower, strOrig string) (int, bool) {
	pLen := len(pLower)
	sLen := len(sLower)

	if pLen > sLen {
		return 0, false
	}

	// Check: is pattern a subsequence of str?
	pi := 0
	for si := 0; si < sLen && pi < pLen; si++ {
		if sLower[si] == pLower[pi] {
			pi++
		}
	}
	if pi < pLen {
		return 0, false
	}

	// Score the match greedily.
	score := 0
	pi = 0
	prevMatchIdx := -1

	for si := 0; si < sLen && pi < pLen; si++ {
		if sLower[si] != pLower[pi] {
			continue
		}

		// Base point for a character match.
		score += 1

		// Bonus: consecutive match (characters are adjacent).
		if prevMatchIdx == si-1 {
			score += 4
		}

		// Bonus: match at the start of the string.
		if si == 0 {
			score += 8
		}

		// Bonus: match at a word boundary.
		if si > 0 && isBoundary(rune(strOrig[si-1]), rune(strOrig[si])) {
			score += 4
		}

		prevMatchIdx = si
		pi++
	}

	// Prefer shorter targets.
	if sLen > pLen {
		score -= (sLen - pLen)
	}

	// Exact match bonus.
	if pLower == sLower {
		score += 20
	}

	return score, true
}

// damerauLevenshtein computes the Damerau-Levenshtein distance between two
// strings. This accounts for insertions, deletions, substitutions, and
// adjacent transpositions.
func damerauLevenshtein(a, b string) int {
	la := len(a)
	lb := len(b)

	// Use the optimal string alignment variant (restricted edit distance).
	// This is sufficient for command typo correction.
	d := make([][]int, la+1)
	for i := range d {
		d[i] = make([]int, lb+1)
		d[i][0] = i
	}
	for j := 0; j <= lb; j++ {
		d[0][j] = j
	}

	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}

			d[i][j] = min3(
				d[i-1][j]+1,   // deletion
				d[i][j-1]+1,   // insertion
				d[i-1][j-1]+cost, // substitution
			)

			// Transposition.
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				if d[i-2][j-2]+cost < d[i][j] {
					d[i][j] = d[i-2][j-2] + cost
				}
			}
		}
	}

	return d[la][lb]
}

// min3 returns the minimum of three ints.
func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// isBoundary returns true if the transition from prev to cur represents a
// word boundary (e.g., underscore, hyphen, or lowercase→uppercase).
func isBoundary(prev, cur rune) bool {
	if prev == '_' || prev == '-' {
		return true
	}
	if unicode.IsLower(prev) && unicode.IsUpper(cur) {
		return true
	}
	return false
}

// fuzzyRank holds a match candidate and its score.
type fuzzyRank struct {
	Name  string
	Score int
}

// fuzzyRankCandidates scores pattern against each candidate and returns
// matches sorted by descending score. Only entries with a positive match
// are included.
func fuzzyRankCandidates(pattern string, candidates []string) []fuzzyRank {
	var results []fuzzyRank
	for _, c := range candidates {
		if score, ok := fuzzyMatch(pattern, c); ok && score > 0 {
			results = append(results, fuzzyRank{Name: c, Score: score})
		}
	}

	// Sort by descending score, then alphabetically for ties.
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score ||
				(results[j].Score == results[i].Score && results[j].Name < results[i].Name) {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	return results
}
