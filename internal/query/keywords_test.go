package query

import (
	"reflect"
	"testing"
)

func TestExtractKeywords(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "where is psge located",
			input:    "where is psge located?",
			expected: []string{"psge"},
		},
		{
			name:     "psge location",
			input:    "psge location",
			expected: []string{"psge", "location"},
		},
		{
			name:     "git status in History_eXtended",
			input:    "git status in History_eXtended",
			expected: []string{"git", "status", "history_extended"},
		},
		{
			name:     "single token",
			input:    "make",
			expected: []string{"make"},
		},
		{
			name:     "punctuation stripped",
			input:    "hello, world!",
			expected: []string{"hello", "world"},
		},
		{
			name:     "empty",
			input:    "",
			expected: nil,
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: nil,
		},
		{
			name:     "all stopwords",
			input:    "where is the",
			expected: nil,
		},
		{
			name:     "mixed case",
			input:    "WHERE Is Psge LOCATED",
			expected: []string{"psge"},
		},
		{
			name:     "hyphenated",
			input:    "my-project build",
			expected: []string{"my-project", "build"},
		},
		{
			name:     "hyphenated multi-part",
			input:    "test-s3-integration",
			expected: []string{"test-s3-integration"},
		},
		{
			name:     "path-like",
			input:    "projects/psge",
			expected: []string{"projects", "psge"},
		},
		{
			name:     "config file",
			input:    "fix .golangci.yml",
			expected: []string{"fix", "golangci", "yml"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractKeywords(tt.input)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("ExtractKeywords(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestBuildFTSQuery(t *testing.T) {
	tests := []struct {
		name     string
		keywords []string
		expected string
	}{
		{
			name:     "single",
			keywords: []string{"psge"},
			expected: "psge",
		},
		{
			name:     "multiple OR",
			keywords: []string{"git", "status"},
			expected: "git OR status",
		},
		{
			name:     "three tokens",
			keywords: []string{"make", "build", "test"},
			expected: "make OR build OR test",
		},
		{
			name:     "empty",
			keywords: []string{},
			expected: "",
		},
		{
			name:     "nil",
			keywords: nil,
			expected: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildFTSQuery(tt.keywords)
			if got != tt.expected {
				t.Errorf("BuildFTSQuery(%v) = %q, want %q", tt.keywords, got, tt.expected)
			}
		})
	}
}
