package skill

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// SkillRepoDir is the directory name of the central skill repository
	// inside each profile sync directory.
	SkillRepoDir = "skill-repo"
)

// GetSkillRepoPath returns the absolute path of the current profile skill repository.
func GetSkillRepoPath() (string, error) {
	return GetSkillRepoPathForProfile(CurrentSyncProfile())
}

// GetSkillRepoPathForProfile returns the absolute path of a profile skill repository.
func GetSkillRepoPathForProfile(profile string) (string, error) {
	profileDir, err := GetSyncProfileDir(profile)
	if err != nil {
		return "", err
	}
	return filepath.Join(profileDir, SkillRepoDir), nil
}

// EnsureSkillRepo ensures the current profile skill repository exists.
func EnsureSkillRepo() (string, error) {
	repoPath, err := GetSkillRepoPath()
	if err != nil {
		return "", err
	}
	return ensureSkillRepoPath(repoPath)
}

// EnsureSkillRepoForProfile ensures a profile skill repository exists.
func EnsureSkillRepoForProfile(profile string) (string, error) {
	repoPath, err := GetSkillRepoPathForProfile(profile)
	if err != nil {
		return "", err
	}
	return ensureSkillRepoPath(repoPath)
}

func ensureSkillRepoPath(repoPath string) (string, error) {
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create skill repo: %w", err)
	}
	return repoPath, nil
}

// SkillExistsInRepo reports whether a skill exists in the central repository.
func SkillExistsInRepo(skillName string) (bool, error) {
	repoPath, err := GetSkillRepoPath()
	if err != nil {
		return false, err
	}
	skillPath := filepath.Join(repoPath, skillName)
	info, err := os.Stat(skillPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}

// ScanSkillRepo returns all skill names in the central repository.
// Only directories containing SKILL.md are considered valid skills.
func ScanSkillRepo() ([]string, error) {
	repoPath, err := GetSkillRepoPath()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(repoPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		skillMd := filepath.Join(repoPath, entry.Name(), "SKILL.md")
		if _, err := os.Stat(skillMd); err == nil {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// AgentSkillStatus describes how a skill currently exists in an agent directory.
type AgentSkillStatus string

const (
	// AgentSkillMissing means the skill does not exist in this agent directory.
	AgentSkillMissing AgentSkillStatus = "missing"
	// AgentSkillLinked means the skill is already a correct symlink to the central repo.
	AgentSkillLinked AgentSkillStatus = "linked"
	// AgentSkillSame means the skill exists as a real directory with the same content as the source.
	AgentSkillSame AgentSkillStatus = "same"
	// AgentSkillConflict means the skill exists with different content from the source.
	AgentSkillConflict AgentSkillStatus = "conflict"
	// AgentSkillBroken means the skill is a symlink whose target is missing.
	AgentSkillBroken AgentSkillStatus = "broken"
	// AgentSkillFound is used when scanning agents for skills not in the source.
	AgentSkillFound AgentSkillStatus = "found"
)

// InspectAgentSkill examines a skill within a single agent directory and reports
// its relationship to the central repository.
func InspectAgentSkill(agentPath, skillName, repoPath string) (AgentSkillStatus, error) {
	agentSkillPath := filepath.Join(agentPath, skillName)
	info, err := os.Lstat(agentSkillPath)
	if err != nil {
		if os.IsNotExist(err) {
			return AgentSkillMissing, nil
		}
		return "", err
	}

	repoSkillPath := filepath.Join(repoPath, skillName)
	repoExists := false
	if _, err := os.Stat(repoSkillPath); err == nil {
		repoExists = true
	}

	// Symlink case
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(agentSkillPath)
		if err != nil {
			return AgentSkillBroken, nil
		}
		// Resolve relative target against the agent dir
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(agentSkillPath), target)
		}
		// Check whether target exists at all
		if _, err := os.Stat(target); err != nil {
			return AgentSkillBroken, nil
		}
		absTarget, _ := filepath.Abs(target)
		absRepoSkill, _ := filepath.Abs(repoSkillPath)
		if absTarget == absRepoSkill {
			return AgentSkillLinked, nil
		}
		// Symlink to elsewhere; treat as conflict so we can decide
		return AgentSkillConflict, nil
	}

	// Real directory case
	if !info.IsDir() {
		return AgentSkillConflict, nil
	}

	if !repoExists {
		// Source missing but agent has it
		return AgentSkillFound, nil
	}

	// Compare hashes
	repoHash, err := ComputeDirectoryHash(repoSkillPath)
	if err != nil {
		return "", err
	}
	agentHash, err := ComputeDirectoryHash(agentSkillPath)
	if err != nil {
		return "", err
	}
	if repoHash == agentHash {
		return AgentSkillSame, nil
	}
	return AgentSkillConflict, nil
}

// LinkSkillToAgent creates a symlink from the agent dir to the central repo.
// It assumes any conflict has already been resolved (e.g. backup taken).
func LinkSkillToAgent(repoPath, skillName, agentPath string) error {
	if err := os.MkdirAll(agentPath, 0755); err != nil {
		return fmt.Errorf("create agent dir: %w", err)
	}
	target := filepath.Join(repoPath, skillName)
	link := filepath.Join(agentPath, skillName)

	// If link already exists and is the correct symlink, skip
	if info, err := os.Lstat(link); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			existing, _ := os.Readlink(link)
			if existing == target {
				return nil
			}
		}
		if err := os.RemoveAll(link); err != nil {
			return fmt.Errorf("remove existing %s: %w", link, err)
		}
	}

	if err := os.Symlink(target, link); err != nil {
		return fmt.Errorf("create symlink %s -> %s: %w", link, target, err)
	}
	return nil
}

// UnlinkSkillFromAgent removes a skill link or copy from a single agent directory.
// It only removes the entry inside agentPath/skillName; the central repo is preserved.
func UnlinkSkillFromAgent(skillName, agentPath string) error {
	link := filepath.Join(agentPath, skillName)
	if _, err := os.Lstat(link); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.RemoveAll(link)
}

// BackupAgentSkill moves an existing agent skill to the agent's backup dir
// before it is replaced by a symlink. Returns the backup path or empty if nothing to back up.
func BackupAgentSkill(agentPath, skillName string) (string, error) {
	src := filepath.Join(agentPath, skillName)
	info, err := os.Lstat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	// If it's already a symlink, no backup needed
	if info.Mode()&os.ModeSymlink != 0 {
		return "", nil
	}

	backupRoot := filepath.Join(agentPath, SyncBackupDir)
	if err := os.MkdirAll(backupRoot, 0755); err != nil {
		return "", err
	}
	timestamp := timeNowStamp()
	dest := filepath.Join(backupRoot, fmt.Sprintf("%s-%s", skillName, timestamp))
	if err := os.Rename(src, dest); err != nil {
		return "", fmt.Errorf("backup %s -> %s: %w", src, dest, err)
	}
	return dest, nil
}

// ImportAgentSkillToRepo copies an agent's skill directory into the central
// repository. It only works when the repo does not yet contain that skill.
func ImportAgentSkillToRepo(repoPath, agentSkillPath, skillName string) error {
	dst := filepath.Join(repoPath, skillName)
	if _, err := os.Stat(dst); err == nil {
		return errors.New("skill already exists in repo")
	}
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		return err
	}
	return copyDir(agentSkillPath, dst)
}

// ComputeFileMD5 computes the MD5 hash of a single file. Used when uploading
// to compare with the Nacos remote md5 fingerprint.
func ComputeFileMD5(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// timeNowStamp returns a backup-friendly timestamp.
func timeNowStamp() string {
	return time.Now().Format("20060102T150405")
}
