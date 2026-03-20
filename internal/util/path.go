package util

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandTilde expands ~ and ~/ paths to absolute paths using the user's home directory.
// Supported patterns:
//   - "~"        → Home directory
//   - "~/path"   → Home directory joined with path
//   - "/path"    → Returned unchanged
//   - "path"     → Returned unchanged
//
// Returns the expanded path or the original path if expansion is not applicable.
func ExpandTilde(path string) (string, error) {
	if path == "~" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return path, err
		}
		return homeDir, nil
	}
	
	if strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return path, err
		}
		return filepath.Join(homeDir, path[2:]), nil
	}
	
	// No expansion needed
	return path, nil
}

// SeparatorLine returns a horizontal separator line.
// Uses Unicode double line characters by default.
// Pass ascii=true for environments that don't support Unicode.
// Returns empty string for non-positive lengths.
func SeparatorLine(length int, ascii bool) string {
	if length <= 0 {
		return ""
	}
	if ascii {
		return strings.Repeat("-", length)
	}
	return strings.Repeat("═", length)
}
