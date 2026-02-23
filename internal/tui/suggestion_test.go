package tui

import "testing"

func TestIsValidSuggestion(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// Valid suggestions.
		{"run the tests", true},
		{"commit this", true},
		{"push it", true},
		{"yes", true},
		{"no", true},
		{"/help", true},
		{"fix the lint errors", true},
		{"try running it again", true},

		// Empty/meta.
		{"", false},
		{"done", false},
		{"nothing found", false},
		{"nothing found.", false},
		{"nothing to suggest here", false},
		{"no suggestion needed", false},

		// Error messages.
		{"api error: something broke", false},
		{"prompt is too long for this model", false},

		// Prefixed labels.
		{"Suggestion: try running tests", false},
		{"Next: commit the changes", false},

		// Single words (not in allowed list).
		{"hello", false},
		{"refactor", false},

		// Too many words.
		{"this is a very long suggestion that has way too many words in it for the prompt", false},

		// Too long (>= 100 chars).
		{"a very long string that is definitely going to be over one hundred characters when you count them all up in the suggestion", false},

		// Multiple sentences.
		{"Run the tests. Then commit the changes.", false},

		// Has formatting.
		{"run the **tests**", false},
		{"run the\ntests", false},

		// Evaluative phrases.
		{"looks good to me", false},
		{"thanks for doing that", false},
		{"that works perfectly", false},
		{"awesome job there", false},

		// Claude-voice.
		{"Let me check that for you", false},
		{"I'll run the tests now", false},
		{"Here's what I found", false},
		{"You should run the tests", false},
		{"certainly I can do that", false},
	}

	for _, tt := range tests {
		got := isValidSuggestion(tt.input)
		if got != tt.want {
			t.Errorf("isValidSuggestion(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
