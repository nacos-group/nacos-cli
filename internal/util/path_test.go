package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandTilde(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "tilde only",
			input:    "~",
			expected: homeDir,
			wantErr:  false,
		},
		{
			name:     "tilde slash path",
			input:    "~/test/path",
			expected: filepath.Join(homeDir, "test/path"),
			wantErr:  false,
		},
		{
			name:     "absolute path unchanged",
			input:    "/absolute/path",
			expected: "/absolute/path",
			wantErr:  false,
		},
		{
			name:     "relative path unchanged",
			input:    "relative/path",
			expected: "relative/path",
			wantErr:  false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpandTilde(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExpandTilde() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("ExpandTilde() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSeparatorLine(t *testing.T) {
	tests := []struct {
		name     string
		length   int
		ascii    bool
		expected string
	}{
		{
			name:     "unicode 10 chars",
			length:   10,
			ascii:    false,
			expected: "══════════",
		},
		{
			name:     "ascii 10 chars",
			length:   10,
			ascii:    true,
			expected: "----------",
		},
		{
			name:     "zero length",
			length:   0,
			ascii:    false,
			expected: "",
		},
		{
			name:     "negative length",
			length:   -5,
			ascii:    true,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SeparatorLine(tt.length, tt.ascii)
			if got != tt.expected {
				t.Errorf("SeparatorLine(%d, %v) = %q, want %q", tt.length, tt.ascii, got, tt.expected)
			}
		})
	}
}
