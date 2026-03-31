package main

import "testing"

func TestSanitizeTitle(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "HTML entity apostrophe with possessive",
			input:    "World&#39;s 1st Coding Monitor",
			expected: "World 1st Coding Monitor",
		},
		{
			name:     "HTML entity double quote",
			input:    "Hello &quot;World&quot;",
			expected: "Hello World",
		},
		{
			name:     "HTML entity ampersand",
			input:    "Tom &amp; Jerry",
			expected: "Tom & Jerry",
		},
		{
			name:     "Contraction don't",
			input:    "I don't know",
			expected: "I do not know",
		},
		{
			name:     "Contraction doesn't",
			input:    "It doesn't work",
			expected: "It does not work",
		},
		{
			name:     "Contraction can't",
			input:    "You can't do this",
			expected: "You cannot do this",
		},
		{
			name:     "Contraction won't",
			input:    "They won't come",
			expected: "They will not come",
		},
		{
			name:     "Contraction it's",
			input:    "it's great",
			expected: "it is great",
		},
		{
			name:     "Possessive 's dropped",
			input:    "World's best",
			expected: "World best",
		},
		{
			name:     "Smart quotes removed",
			input:    "Hello \u201cWorld\u201d and \u2018test\u2019",
			expected: "Hello World and test",
		},
		{
			name:     "Mixed HTML entities and contractions",
			input:    "World&#39;s 1st Monitor that doesn&#39;t break",
			expected: "World 1st Monitor that does not break",
		},
		{
			name:     "Plain title unchanged",
			input:    "A Simple Title",
			expected: "A Simple Title",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeTitle(tt.input)
			if got != tt.expected {
				t.Errorf("sanitizeTitle(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
