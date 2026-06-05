package skill

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// hashExcludeDirs lists directories that should be excluded from content hashing.
var hashExcludeDirs = map[string]bool{
	".git":              true,
	".skill-sync-backup": true,
	"node_modules":      true,
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
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return "", nil
	}

	// Collect all file paths with their relative paths
	type fileEntry struct {
		relPath string
		absPath string
	}
	var files []fileEntry

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(dir, path)
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
		file, err := os.Open(f.absPath)
		if err != nil {
			return "", fmt.Errorf("failed to open %s: %w", f.absPath, err)
		}
		if _, err := io.Copy(hasher, file); err != nil {
			file.Close()
			return "", fmt.Errorf("failed to read %s: %w", f.absPath, err)
		}
		file.Close()
		hasher.Write([]byte{0}) // NULL separator
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
