package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DetectConflict determines if a skill is in conflict state.
// Conflict means both local hash and remote MD5 have changed from the synced baseline.
func DetectConflict(entry SyncSkillEntry, currentLocalHash, currentRemoteMd5 string) bool {
	localChanged := currentLocalHash != "" && entry.SyncedHash != "" && currentLocalHash != entry.SyncedHash
	remoteChanged := currentRemoteMd5 != "" && entry.RemoteMd5 != "" && currentRemoteMd5 != entry.RemoteMd5
	return localChanged && remoteChanged
}

// BackupSkillDir creates a backup of a skill directory before overwriting.
// Returns the backup path.
func BackupSkillDir(agentPath, skillName string) (string, error) {
	skillDir := filepath.Join(agentPath, skillName)
	if _, err := os.Stat(skillDir); os.IsNotExist(err) {
		return "", nil // Nothing to backup
	}

	backupBaseDir := filepath.Join(agentPath, SyncBackupDir)
	if err := os.MkdirAll(backupBaseDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}

	timestamp := time.Now().Format("20060102T150405")
	backupDir := filepath.Join(backupBaseDir, fmt.Sprintf("%s-%s", skillName, timestamp))

	if err := copyDir(skillDir, backupDir); err != nil {
		return "", fmt.Errorf("failed to backup skill directory: %w", err)
	}

	return backupDir, nil
}

// ResolveUseRemote resolves a conflict by accepting the remote version.
// Steps: backup local → re-download from server → update hashes → set status=Synced.
func ResolveUseRemote(state *SyncState, skillName string, skillService *SkillService, agents []AgentDir) error {
	entry, ok := state.Skills[skillName]
	if !ok {
		return fmt.Errorf("skill %q not found in sync state", skillName)
	}

	if len(agents) == 0 {
		return fmt.Errorf("no agent directories configured")
	}

	// Backup in all agent directories
	for _, agent := range agents {
		backupPath, err := BackupSkillDir(agent.Path, skillName)
		if err != nil {
			return fmt.Errorf("backup failed for agent %s: %w", agent.Name, err)
		}
		if backupPath != "" {
			fmt.Printf("  Backup: %s\n", backupPath)
		}
	}

	// Re-download to first agent directory (unconditional, no md5)
	primaryDir := agents[0].Path
	result, err := skillService.QuerySkill(skillName, primaryDir, "", state.Label, "")
	if err != nil {
		return fmt.Errorf("failed to download skill: %w", err)
	}
	if result.Deleted {
		return fmt.Errorf("skill %q not found on server", skillName)
	}

	// Copy to remaining agents
	sourceDir := filepath.Join(primaryDir, skillName)
	if len(agents) > 1 {
		if err := EnsureSkillInAllAgents(skillName, sourceDir, agents[1:]); err != nil {
			return fmt.Errorf("failed to sync to other agents: %w", err)
		}
	}

	// Recompute local hash
	localHash, err := ComputeDirectoryHash(sourceDir)
	if err != nil {
		return fmt.Errorf("failed to compute hash: %w", err)
	}

	// Update state
	entry.RemoteMd5 = result.Md5
	entry.ResolvedVersion = result.ResolvedVersion
	entry.LocalHash = localHash
	entry.SyncedHash = localHash
	entry.Status = SyncStatusSynced
	entry.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	state.Skills[skillName] = entry

	return nil
}

// ResolveUseLocal resolves a conflict by keeping local changes.
// Steps: acknowledge remote change, set status=LocalChanges (user should upload).
func ResolveUseLocal(state *SyncState, skillName string) error {
	entry, ok := state.Skills[skillName]
	if !ok {
		return fmt.Errorf("skill %q not found in sync state", skillName)
	}

	// Keep local as-is, update synced hash to acknowledge we've "seen" the remote
	// but chosen to keep local. Status becomes LocalChanges so user can upload.
	entry.Status = SyncStatusLocalChanges
	entry.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	state.Skills[skillName] = entry

	return nil
}
