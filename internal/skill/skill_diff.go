package skill

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FileDiffStatus describes how a file differs between two skill directories.
type FileDiffStatus string

const (
	// FileDiffAdded means the file exists only in the source.
	FileDiffAdded FileDiffStatus = "added"
	// FileDiffRemoved means the file exists only in the target (agent local).
	FileDiffRemoved FileDiffStatus = "removed"
	// FileDiffModified means the file exists in both but content differs.
	FileDiffModified FileDiffStatus = "modified"
)

// FileDiff describes one file-level difference.
type FileDiff struct {
	RelPath string
	Status  FileDiffStatus
}

// CompareSkillDirs walks two skill directories and reports differences.
// The "source" side is treated as the authoritative version.
func CompareSkillDirs(sourceDir, targetDir string) ([]FileDiff, error) {
	srcFiles, err := listSkillFiles(sourceDir)
	if err != nil {
		return nil, err
	}
	tgtFiles, err := listSkillFiles(targetDir)
	if err != nil {
		return nil, err
	}

	var diffs []FileDiff
	srcSet := make(map[string]bool, len(srcFiles))
	for _, p := range srcFiles {
		srcSet[p] = true
	}
	tgtSet := make(map[string]bool, len(tgtFiles))
	for _, p := range tgtFiles {
		tgtSet[p] = true
	}

	allPaths := make(map[string]bool, len(srcFiles)+len(tgtFiles))
	for p := range srcSet {
		allPaths[p] = true
	}
	for p := range tgtSet {
		allPaths[p] = true
	}

	sortedPaths := make([]string, 0, len(allPaths))
	for p := range allPaths {
		sortedPaths = append(sortedPaths, p)
	}
	sort.Strings(sortedPaths)

	for _, p := range sortedPaths {
		switch {
		case srcSet[p] && !tgtSet[p]:
			diffs = append(diffs, FileDiff{RelPath: p, Status: FileDiffAdded})
		case !srcSet[p] && tgtSet[p]:
			diffs = append(diffs, FileDiff{RelPath: p, Status: FileDiffRemoved})
		default:
			same, err := filesEqual(filepath.Join(sourceDir, p), filepath.Join(targetDir, p))
			if err != nil {
				return nil, err
			}
			if !same {
				diffs = append(diffs, FileDiff{RelPath: p, Status: FileDiffModified})
			}
		}
	}
	return diffs, nil
}

// listSkillFiles walks a skill directory and returns sorted relative file paths,
// applying the same exclusion rules used by ComputeDirectoryHash.
func listSkillFiles(dir string) ([]string, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil
	}
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if hashExcludeDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if hashExcludeFiles[info.Name()] {
			return nil
		}
		if strings.HasPrefix(info.Name(), ".skill-sync") {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// filesEqual reports whether two files have identical bytes.
func filesEqual(a, b string) (bool, error) {
	dataA, err := os.ReadFile(a)
	if err != nil {
		return false, err
	}
	dataB, err := os.ReadFile(b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(dataA, dataB), nil
}

// PrintSkillDiff renders a human-readable summary of the differences between
// two skill directories. For modified files it shows a line-level unified diff.
func PrintSkillDiff(out io.Writer, sourceDir, targetDir string) error {
	diffs, err := CompareSkillDirs(sourceDir, targetDir)
	if err != nil {
		return err
	}
	if len(diffs) == 0 {
		fmt.Fprintln(out, "(no differences)")
		return nil
	}

	added, removed, modified := 0, 0, 0
	for _, d := range diffs {
		switch d.Status {
		case FileDiffAdded:
			added++
		case FileDiffRemoved:
			removed++
		case FileDiffModified:
			modified++
		}
	}
	fmt.Fprintf(out, "Source: %s\n", sourceDir)
	fmt.Fprintf(out, "Local:  %s\n", targetDir)
	fmt.Fprintf(out, "Summary: %d added, %d removed, %d modified\n\n", added, removed, modified)

	for _, d := range diffs {
		switch d.Status {
		case FileDiffAdded:
			fmt.Fprintf(out, "  + %s (in source only)\n", d.RelPath)
		case FileDiffRemoved:
			fmt.Fprintf(out, "  - %s (in local only)\n", d.RelPath)
		case FileDiffModified:
			fmt.Fprintf(out, "  ~ %s (modified)\n", d.RelPath)
		}
	}

	// Inline unified diff for modified files (small files only)
	for _, d := range diffs {
		if d.Status != FileDiffModified {
			continue
		}
		fmt.Fprintf(out, "\n--- %s (source)\n+++ %s (local)\n", d.RelPath, d.RelPath)
		if err := writeFileDiff(out, filepath.Join(sourceDir, d.RelPath), filepath.Join(targetDir, d.RelPath)); err != nil {
			fmt.Fprintf(out, "  (diff failed: %v)\n", err)
		}
	}
	return nil
}

// writeFileDiff produces a simple line-by-line diff. Files larger than 200 KB
// are summarized rather than fully diffed.
func writeFileDiff(out io.Writer, srcPath, tgtPath string) error {
	const maxSize = 200 * 1024

	srcStat, err := os.Stat(srcPath)
	if err != nil {
		return err
	}
	tgtStat, err := os.Stat(tgtPath)
	if err != nil {
		return err
	}
	if srcStat.Size() > maxSize || tgtStat.Size() > maxSize {
		fmt.Fprintf(out, "  (file too large to diff: source=%d bytes, local=%d bytes)\n",
			srcStat.Size(), tgtStat.Size())
		return nil
	}

	srcLines, err := readLines(srcPath)
	if err != nil {
		return err
	}
	tgtLines, err := readLines(tgtPath)
	if err != nil {
		return err
	}

	// Simple diff: print common prefix, then differences as -/+ lines.
	commonPrefix := 0
	for commonPrefix < len(srcLines) && commonPrefix < len(tgtLines) && srcLines[commonPrefix] == tgtLines[commonPrefix] {
		commonPrefix++
	}

	commonSuffix := 0
	for commonSuffix < len(srcLines)-commonPrefix && commonSuffix < len(tgtLines)-commonPrefix &&
		srcLines[len(srcLines)-1-commonSuffix] == tgtLines[len(tgtLines)-1-commonSuffix] {
		commonSuffix++
	}

	contextLines := 2
	startCtx := commonPrefix - contextLines
	if startCtx < 0 {
		startCtx = 0
	}

	for i := startCtx; i < commonPrefix; i++ {
		fmt.Fprintf(out, "  %s\n", srcLines[i])
	}
	for i := commonPrefix; i < len(srcLines)-commonSuffix; i++ {
		fmt.Fprintf(out, "- %s\n", srcLines[i])
	}
	for i := commonPrefix; i < len(tgtLines)-commonSuffix; i++ {
		fmt.Fprintf(out, "+ %s\n", tgtLines[i])
	}

	endCtx := len(srcLines) - commonSuffix + contextLines
	if endCtx > len(srcLines) {
		endCtx = len(srcLines)
	}
	for i := len(srcLines) - commonSuffix; i < endCtx; i++ {
		fmt.Fprintf(out, "  %s\n", srcLines[i])
	}
	return nil
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}
