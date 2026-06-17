package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ResolveUseRemote resolves a conflict by accepting the remote version.
// Steps: backup local, re-download from server into the central repo, link all
// agents, update hashes, and set status=Synced.
func ResolveUseRemote(state *SyncState, skillName string, skillService *SkillService, agents []AgentDir) error {
	entry, ok := state.Skills[skillName]
	if !ok {
		return fmt.Errorf("skill %q not found in sync state", skillName)
	}

	if len(agents) == 0 {
		return fmt.Errorf("no agent directories configured")
	}

	repoPath, err := EnsureSkillRepo()
	if err != nil {
		return err
	}

	// Re-download unconditionally.
	result, err := skillService.FetchSkill(skillName, "", state.Label, "")
	if err != nil {
		return fmt.Errorf("failed to download skill: %w", err)
	}
	if result.Deleted {
		return fmt.Errorf("skill %q not found on server", skillName)
	}

	stageRoot, err := os.MkdirTemp("", "nacos-skill-resolve-")
	if err != nil {
		return fmt.Errorf("failed to create staging dir: %w", err)
	}
	defer os.RemoveAll(stageRoot)

	if err := ExtractSkillZip(result.ZipBytes, stageRoot); err != nil {
		return fmt.Errorf("failed to extract remote skill: %w", err)
	}
	sourceDir := filepath.Join(stageRoot, skillName)
	if info, err := os.Stat(sourceDir); err != nil {
		return fmt.Errorf("remote skill directory not found: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("remote skill path is not a directory: %s", sourceDir)
	}

	repoSkillDir := filepath.Join(repoPath, skillName)
	if _, err := os.Stat(repoSkillDir); err == nil {
		backupRoot := filepath.Join(repoPath, "..", ".skill-sync-backup")
		if err := os.MkdirAll(backupRoot, 0755); err != nil {
			return err
		}
		backupDest := filepath.Join(backupRoot, fmt.Sprintf("repo-%s-%s", skillName, time.Now().Format("20060102T150405")))
		if err := os.Rename(repoSkillDir, backupDest); err != nil {
			return fmt.Errorf("backup repo skill: %w", err)
		}
	}

	if err := ImportAgentSkillToRepo(repoPath, sourceDir, skillName); err != nil {
		return fmt.Errorf("write remote skill to repo: %w", err)
	}
	if _, err := LinkSkillForce(repoPath, skillName, agents, os.Stdout); err != nil {
		return fmt.Errorf("failed to link remote skill: %w", err)
	}

	localHash, err := ComputeDirectoryHash(repoSkillDir)
	if err != nil {
		return fmt.Errorf("failed to compute hash: %w", err)
	}

	// Update state
	entry.RemoteMd5 = result.Md5
	entry.ResolvedVersion = result.ResolvedVersion
	entry.LocalHash = localHash
	entry.SyncedHash = localHash
	entry.ConflictAgents = nil
	entry.Status = SyncStatusSynced
	entry.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	state.Skills[skillName] = entry

	return nil
}

// ResolveAgentConflictUseRepo overwrites the diverging agents with the repo
// version, backing up the agent's prior content. The skill's ConflictAgents
// field is cleared and Status is set to Linked/Synced.
func ResolveAgentConflictUseRepo(state *SyncState, skillName string) error {
	entry, ok := state.Skills[skillName]
	if !ok {
		return fmt.Errorf("skill %q not found in sync state", skillName)
	}
	repoPath, err := GetSkillRepoPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(repoPath, skillName)); err != nil {
		return fmt.Errorf("repo skill missing: %w", err)
	}
	if _, err := LinkSkillForce(repoPath, skillName, state.Agents, os.Stdout); err != nil {
		return err
	}
	hash, _ := ComputeDirectoryHash(filepath.Join(repoPath, skillName))
	entry.LocalHash = hash
	entry.SyncedHash = hash
	entry.ConflictAgents = nil
	if state.Mode == SyncModeNacos {
		entry.Status = SyncStatusSynced
	} else {
		entry.Status = SyncStatusLinked
	}
	entry.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	state.Skills[skillName] = entry
	return nil
}

// ResolveAgentConflictUseAgent treats the named agent's content as the new
// authoritative source: it imports the agent's directory back into the repo,
// then re-links every other agent (overwriting + backing up the old conflicts).
func ResolveAgentConflictUseAgent(state *SyncState, skillName, agentName string) error {
	entry, ok := state.Skills[skillName]
	if !ok {
		return fmt.Errorf("skill %q not found in sync state", skillName)
	}
	repoPath, err := GetSkillRepoPath()
	if err != nil {
		return err
	}
	var source *AgentDir
	for i := range state.Agents {
		if state.Agents[i].Name == agentName {
			source = &state.Agents[i]
			break
		}
	}
	if source == nil {
		return fmt.Errorf("agent %q not configured", agentName)
	}
	agentSkillPath := filepath.Join(source.Path, skillName)
	info, err := os.Lstat(agentSkillPath)
	if err != nil {
		return fmt.Errorf("agent %q does not have skill %q: %w", agentName, skillName, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("agent %q's %q is a symlink; cannot use as source", agentName, skillName)
	}
	// Remove the old repo version so ImportAgentSkillToRepo can rewrite it.
	repoSkillDir := filepath.Join(repoPath, skillName)
	if _, err := os.Stat(repoSkillDir); err == nil {
		// Backup the old repo content before overwrite.
		backupRoot := filepath.Join(repoPath, "..", ".skill-sync-backup")
		if err := os.MkdirAll(backupRoot, 0755); err != nil {
			return err
		}
		backupDest := filepath.Join(backupRoot, fmt.Sprintf("repo-%s-%s", skillName, time.Now().Format("20060102T150405")))
		if err := os.Rename(repoSkillDir, backupDest); err != nil {
			return fmt.Errorf("backup repo skill: %w", err)
		}
	}
	if err := ImportAgentSkillToRepo(repoPath, agentSkillPath, skillName); err != nil {
		return fmt.Errorf("import from %s: %w", agentName, err)
	}
	if _, err := LinkSkillForce(repoPath, skillName, state.Agents, os.Stdout); err != nil {
		return err
	}
	hash, _ := ComputeDirectoryHash(filepath.Join(repoPath, skillName))
	entry.LocalHash = hash
	entry.SyncedHash = hash
	entry.ConflictAgents = nil
	if state.Mode == SyncModeNacos {
		// repo now diverges from Nacos; user should upload to publish.
		entry.Status = SyncStatusLocalChanges
	} else {
		entry.Status = SyncStatusLinked
	}
	entry.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	state.Skills[skillName] = entry
	return nil
}
