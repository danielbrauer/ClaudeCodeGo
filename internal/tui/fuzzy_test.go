package tui

import (
	"testing"
)

func TestFuzzyMatch_ExactMatch(t *testing.T) {
	score, ok := fuzzyMatch("settings", "settings")
	if !ok {
		t.Fatal("exact match should succeed")
	}
	if score <= 0 {
		t.Errorf("exact match score should be positive, got %d", score)
	}
}

func TestFuzzyMatch_PrefixMatch(t *testing.T) {
	score, ok := fuzzyMatch("set", "settings")
	if !ok {
		t.Fatal("prefix match should succeed")
	}
	if score <= 0 {
		t.Errorf("prefix match score should be positive, got %d", score)
	}
}

func TestFuzzyMatch_Subsequence(t *testing.T) {
	// "stgs" is a subsequence of "settings" (s-e-t-t-i-n-g-s)
	score, ok := fuzzyMatch("stgs", "settings")
	if !ok {
		t.Fatal("subsequence match should succeed")
	}
	if score <= 0 {
		t.Errorf("subsequence match score should be positive, got %d", score)
	}
}

func TestFuzzyMatch_CaseInsensitive(t *testing.T) {
	score, ok := fuzzyMatch("HELP", "help")
	if !ok {
		t.Fatal("case-insensitive match should succeed")
	}
	if score <= 0 {
		t.Errorf("case-insensitive score should be positive, got %d", score)
	}
}

func TestFuzzyMatch_NoMatch(t *testing.T) {
	_, ok := fuzzyMatch("xyz", "help")
	if ok {
		t.Error("non-matching pattern should return false")
	}
}

func TestFuzzyMatch_EmptyPattern(t *testing.T) {
	_, ok := fuzzyMatch("", "anything")
	if !ok {
		t.Error("empty pattern should match everything")
	}
}

func TestFuzzyMatch_PatternLongerThanStr(t *testing.T) {
	_, ok := fuzzyMatch("longpattern", "short")
	if ok {
		t.Error("pattern longer than string should not match")
	}
}

func TestFuzzyMatch_Typo_SettingsSwap(t *testing.T) {
	// "settigns" is a common typo for "settings" — transposed letters.
	// The characters s,e,t,t,i,g,n,s are not all a subsequence of "settings"
	// because after matching s,e,t,t,i the next char 'g' comes before 'n' in
	// "settings" (settings has ...i,n,g,s). So "settigns" with g before n
	// would actually still match as a subsequence: s,e,t,t,i,g,s (skipping n).
	score, ok := fuzzyMatch("settigns", "settings")
	if !ok {
		t.Fatal("typo 'settigns' should still fuzzy-match 'settings'")
	}
	if score <= 0 {
		t.Errorf("typo match should have positive score, got %d", score)
	}
}

func TestFuzzyRankCandidates_Basic(t *testing.T) {
	candidates := []string{"compact", "config", "context", "continue", "cost"}
	results := fuzzyRankCandidates("cot", candidates)

	if len(results) == 0 {
		t.Fatal("expected at least one fuzzy match for 'cot'")
	}

	// All candidates that contain c, o, t in order should appear.
	found := make(map[string]bool)
	for _, r := range results {
		found[r.Name] = true
	}
	// "cost" should match (c-o-s-t has c,o,t in order)
	if !found["cost"] {
		t.Error("expected 'cost' to match 'cot'")
	}
}

func TestFuzzyRankCandidates_Ordering(t *testing.T) {
	candidates := []string{"compact", "config", "cost"}
	results := fuzzyRankCandidates("cos", candidates)

	if len(results) == 0 {
		t.Fatal("expected at least one match")
	}
	// "cost" should rank highest for "cos" since it's a prefix match.
	if results[0].Name != "cost" {
		t.Errorf("expected 'cost' first, got %q", results[0].Name)
	}
}

func TestFuzzyRankCandidates_NoMatch(t *testing.T) {
	candidates := []string{"help", "version"}
	results := fuzzyRankCandidates("xyz", candidates)
	if len(results) != 0 {
		t.Errorf("expected no matches, got %v", results)
	}
}

func TestSlashRegistry_FuzzyComplete(t *testing.T) {
	r := newSlashRegistry()

	tests := []struct {
		input    string
		wantFirst string // expected best match
	}{
		{"settigns", "settings"},  // transposed letters
		{"hlep", "help"},          // transposed letters
		{"cmpct", "compact"},      // missing vowels
		{"vrsn", "version"},       // missing vowels
		{"mdl", "model"},          // missing vowels
		{"dif", "diff"},           // prefix match (handled by prefix first)
		{"hel", "help"},           // prefix match
	}

	for _, tt := range tests {
		matches := r.fuzzyComplete(tt.input)
		if len(matches) == 0 {
			t.Errorf("fuzzyComplete(%q) returned no matches, want %q", tt.input, tt.wantFirst)
			continue
		}
		if matches[0] != tt.wantFirst {
			t.Errorf("fuzzyComplete(%q)[0] = %q, want %q (all: %v)", tt.input, matches[0], tt.wantFirst, matches)
		}
	}
}

func TestSlashRegistry_FuzzyBest(t *testing.T) {
	r := newSlashRegistry()

	// Exact match.
	best, ok := r.fuzzyBest("help")
	if !ok || best != "help" {
		t.Errorf("fuzzyBest(\"help\") = (%q, %v), want (\"help\", true)", best, ok)
	}

	// Fuzzy match.
	best, ok = r.fuzzyBest("settigns")
	if !ok {
		t.Error("fuzzyBest(\"settigns\") should find a match")
	}
	if best != "settings" {
		t.Errorf("fuzzyBest(\"settigns\") = %q, want \"settings\"", best)
	}

	// No match.
	_, ok = r.fuzzyBest("xyzabc")
	if ok {
		t.Error("fuzzyBest(\"xyzabc\") should not find a match")
	}
}

func TestFuzzyMatch_PrefixScoresHigherThanSubsequence(t *testing.T) {
	// "hel" as a prefix of "help" should score higher than as a subsequence
	// of a longer word.
	prefixScore, _ := fuzzyMatch("hel", "help")
	subseqScore, _ := fuzzyMatch("hel", "health")

	if prefixScore <= subseqScore {
		t.Errorf("prefix match (%d) should score higher than longer subsequence match (%d)",
			prefixScore, subseqScore)
	}
}

func TestFuzzyMatch_ShorterTargetPreferred(t *testing.T) {
	// For the same pattern, a shorter target that still matches should score
	// higher than a longer one.
	shortScore, _ := fuzzyMatch("co", "cost")
	longScore, _ := fuzzyMatch("co", "continue")

	if shortScore <= longScore {
		t.Errorf("shorter target score (%d for 'cost') should be higher than longer (%d for 'continue')",
			shortScore, longScore)
	}
}
