package skill

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// hashExcludeDirs lists directories that should be excluded from content hashing.
var hashExcludeDirs = map[string]bool{
	".git":               true,
	".skill-sync-backup": true,
	"node_modules":       true,
}

// hashExcludeFiles lists files that should be excluded from content hashing.
var hashExcludeFiles = map[string]bool{
	"skills-lock.json":   true,
	"skills-watcher.pid": true,
	"skills-watcher.log": true,
}

// ComputeDirectoryHash computes a deterministic SHA256 hash of a skill directory.
// The algorithm: walk directory (skip excluded dirs/files), sort file paths,
// then hash (relative_path + NULL + file_content + NULL) for each file in order.
// Returns empty string if directory doesn't exist or is empty.
func ComputeDirectoryHash(dir string) (string, error) {
	walkRoot, err := resolveDirectoryRoot(dir)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}

	// Collect all file paths with their relative paths
	type fileEntry struct {
		relPath string
		absPath string
	}
	var files []fileEntry

	err = filepath.Walk(walkRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(walkRoot, path)
		if err != nil {
			return err
		}

		// Skip excluded directories
		if info.IsDir() {
			if hashExcludeDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip excluded files
		if hashExcludeFiles[info.Name()] {
			return nil
		}

		// Skip hidden files starting with .skill-sync
		if strings.HasPrefix(info.Name(), ".skill-sync") {
			return nil
		}

		// Normalize path separators to forward slash
		normalizedPath := filepath.ToSlash(relPath)
		files = append(files, fileEntry{relPath: normalizedPath, absPath: path})
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to walk directory %s: %w", dir, err)
	}

	if len(files) == 0 {
		return "", nil
	}

	// Sort by relative path for deterministic ordering
	sort.Slice(files, func(i, j int) bool {
		return files[i].relPath < files[j].relPath
	})

	// Compute composite hash
	hasher := sha256.New()
	for _, f := range files {
		// Write relative path
		hasher.Write([]byte(f.relPath))
		hasher.Write([]byte{0}) // NULL separator

		// Write file content
		data, err := os.ReadFile(f.absPath)
		if err != nil {
			return "", fmt.Errorf("failed to read %s: %w", f.absPath, err)
		}
		if f.relPath == "SKILL.md" {
			data = stripSkillVersionFrontmatter(data)
		}
		hasher.Write(data)
		hasher.Write([]byte{0}) // NULL separator
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func resolveDirectoryRoot(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", path)
	}
	return resolved, nil
}

func stripSkillVersionFrontmatter(data []byte) []byte {
	lines := splitLinesKeepEnd(data)
	if len(lines) == 0 || strings.TrimSpace(string(lines[0])) != "---" {
		return data
	}

	var out []byte
	for i, line := range lines {
		trimmed := strings.TrimSpace(string(line))
		if i > 0 && trimmed == "---" {
			out = append(out, line...)
			for _, rest := range lines[i+1:] {
				out = append(out, rest...)
			}
			return out
		}
		if i > 0 && isSkillVersionFrontmatterLine(line) {
			continue
		}
		out = append(out, line...)
	}

	return data
}

func isSkillVersionFrontmatterLine(line []byte) bool {
	trimmed := strings.TrimSpace(string(line))
	key, _, ok := strings.Cut(trimmed, ":")
	return ok && strings.TrimSpace(key) == "version"
}

func splitLinesKeepEnd(data []byte) [][]byte {
	if len(data) == 0 {
		return nil
	}
	lines := make([][]byte, 0)
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i+1])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
