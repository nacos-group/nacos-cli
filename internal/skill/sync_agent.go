package skill

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// WellKnownAgents defines the default agent skill directories to auto-discover.
var WellKnownAgents = []struct {
	Name string
	Path string // relative to home dir
}{
	{Name: "codex", Path: ".codex/skills"},
	{Name: "claude", Path: ".claude/skills"},
	{Name: "qoder", Path: ".qoder/skills"},
	{Name: "default", Path: ".skills"},
}

// DiscoverAgents checks well-known agent directories and returns those that exist.
func DiscoverAgents() ([]AgentDir, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	var found []AgentDir
	for _, agent := range WellKnownAgents {
		fullPath := filepath.Join(homeDir, agent.Path)
		if info, err := os.Stat(fullPath); err == nil && info.IsDir() {
			found = append(found, AgentDir{
				Name:      agent.Name,
				Path:      fullPath,
				AutoFound: true,
			})
		}
	}
	return found, nil
}

// AddAgent adds a custom agent directory to the sync state.
// Returns error if the path doesn't exist or the name is already taken.
func (s *SyncState) AddAgent(name, path string) error {
	// Expand ~ in path
	if strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		path = filepath.Join(homeDir, path[2:])
	}

	// Validate path exists
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("path does not exist: %s", path)
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}

	// Check for duplicate name
	for _, agent := range s.Agents {
		if agent.Name == name {
			return fmt.Errorf("agent %q already exists (path: %s)", name, agent.Path)
		}
	}

	// Check for duplicate path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	for _, agent := range s.Agents {
		existingAbs, _ := filepath.Abs(agent.Path)
		if existingAbs == absPath {
			return fmt.Errorf("path %s already registered as agent %q", path, agent.Name)
		}
	}

	s.Agents = append(s.Agents, AgentDir{
		Name:      name,
		Path:      absPath,
		AutoFound: false,
	})
	return nil
}

// RemoveAgent removes an agent by name from the sync state.
func (s *SyncState) RemoveAgent(name string) error {
	for i, agent := range s.Agents {
		if agent.Name == name {
			s.Agents = append(s.Agents[:i], s.Agents[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("agent %q not found", name)
}

// EnsureSkillInAllAgents copies a skill directory from sourceDir to all agent directories.
// sourceDir should be the directory containing the skill (e.g., ~/.skills/pdf).
func EnsureSkillInAllAgents(skillName, sourceDir string, agents []AgentDir) error {
	for _, agent := range agents {
		destDir := filepath.Join(agent.Path, skillName)

		// Skip if source and dest are the same
		srcAbs, _ := filepath.Abs(sourceDir)
		dstAbs, _ := filepath.Abs(destDir)
		if srcAbs == dstAbs {
			continue
		}

		// Ensure agent dir exists
		if err := os.MkdirAll(agent.Path, 0755); err != nil {
			return fmt.Errorf("failed to create agent dir %s: %w", agent.Path, err)
		}

		// Remove existing skill dir and copy fresh
		if err := os.RemoveAll(destDir); err != nil {
			return fmt.Errorf("failed to remove existing %s: %w", destDir, err)
		}

		if err := copyDir(sourceDir, destDir); err != nil {
			return fmt.Errorf("failed to copy skill to %s: %w", agent.Path, err)
		}
	}
	return nil
}

// ReplaceSkillInAgents replaces the skill directory in every agent with the staged source.
// Unlike EnsureSkillInAllAgents, this also replaces the primary agent and removes files that
// no longer exist in the remote version.
func ReplaceSkillInAgents(skillName, sourceDir string, agents []AgentDir) error {
	for _, agent := range agents {
		if err := ReplaceSkillDir(agent.Path, skillName, sourceDir); err != nil {
			return fmt.Errorf("failed to replace skill in agent %s: %w", agent.Name, err)
		}
	}
	return nil
}

// ReplaceSkillDir atomically swaps one agent's skill directory with sourceDir.
func ReplaceSkillDir(agentPath, skillName, sourceDir string) error {
	if err := os.MkdirAll(agentPath, 0755); err != nil {
		return fmt.Errorf("failed to create agent dir %s: %w", agentPath, err)
	}

	destDir := filepath.Join(agentPath, skillName)
	srcAbs, _ := filepath.Abs(sourceDir)
	dstAbs, _ := filepath.Abs(destDir)
	if srcAbs == dstAbs {
		return nil
	}

	tempParent, err := os.MkdirTemp(agentPath, ".skill-sync-new-")
	if err != nil {
		return fmt.Errorf("failed to create temp dir in %s: %w", agentPath, err)
	}
	defer os.RemoveAll(tempParent)

	stagedDest := filepath.Join(tempParent, skillName)
	if err := copyDir(sourceDir, stagedDest); err != nil {
		return fmt.Errorf("failed to stage replacement: %w", err)
	}

	backupDir := filepath.Join(agentPath, fmt.Sprintf(".skill-sync-old-%s-%d", skillName, time.Now().UnixNano()))
	hadExisting := false
	if _, err := os.Stat(destDir); err == nil {
		hadExisting = true
		if err := os.Rename(destDir, backupDir); err != nil {
			return fmt.Errorf("failed to move existing skill dir: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to inspect existing skill dir: %w", err)
	}

	if err := os.Rename(stagedDest, destDir); err != nil {
		if hadExisting {
			_ = os.Rename(backupDir, destDir)
		}
		return fmt.Errorf("failed to install replacement: %w", err)
	}

	if hadExisting {
		if err := os.RemoveAll(backupDir); err != nil {
			return fmt.Errorf("failed to remove old skill dir: %w", err)
		}
	}

	return nil
}

// copyDir recursively copies a directory tree.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		return copyFile(path, destPath)
	})
}

// copyFile copies a single file.
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}
