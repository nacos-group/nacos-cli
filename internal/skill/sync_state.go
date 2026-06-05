package skill

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/nacos-group/nacos-cli/internal/config"
)

const (
	// SyncStateFile is the name of the global sync state file.
	SyncStateFile = "skill-sync-state.json"
	// SyncStateVersion is the current schema version.
	SyncStateVersion = 1
	// SyncDaemonPIDFile records the sync daemon process ID.
	SyncDaemonPIDFile = "skill-sync.pid"
	// SyncDaemonLogFile records the sync daemon log output.
	SyncDaemonLogFile = "skill-sync.log"
	// SyncBackupDir is the directory name for conflict backups inside agent dirs.
	SyncBackupDir = ".skill-sync-backup"
)

// SyncStatus represents the synchronization state of a skill.
type SyncStatus string

const (
	SyncStatusSynced        SyncStatus = "synced"
	SyncStatusLocalChanges  SyncStatus = "local_changes"
	SyncStatusUploaded      SyncStatus = "uploaded"
	SyncStatusRemoteChanges SyncStatus = "remote_changes"
	SyncStatusConflict      SyncStatus = "conflict"
)

// DisplayString returns a human-friendly label for the sync status.
func (s SyncStatus) DisplayString() string {
	switch s {
	case SyncStatusSynced:
		return "Synced"
	case SyncStatusLocalChanges:
		return "Local changes"
	case SyncStatusUploaded:
		return "Uploaded"
	case SyncStatusRemoteChanges:
		return "Remote changes"
	case SyncStatusConflict:
		return "Conflict"
	default:
		return string(s)
	}
}

// AgentDir represents a registered agent skill directory.
type AgentDir struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	AutoFound bool   `json:"autoFound"`
}

// SyncSkillEntry tracks sync state for one skill.
type SyncSkillEntry struct {
	Name            string     `json:"name"`
	Label           string     `json:"label,omitempty"`
	ResolvedVersion string     `json:"resolvedVersion,omitempty"`
	RemoteMd5       string     `json:"remoteMd5,omitempty"`
	LocalHash       string     `json:"localHash,omitempty"`
	SyncedHash      string     `json:"syncedHash,omitempty"`
	Status          SyncStatus `json:"status"`
	UpdatedAt       string     `json:"updatedAt"`
}

// SyncState is the top-level global sync state structure.
type SyncState struct {
	Version   int                        `json:"version"`
	Label     string                     `json:"label"`
	Agents    []AgentDir                 `json:"agents"`
	Skills    map[string]SyncSkillEntry  `json:"skills"`
	UpdatedAt string                     `json:"updatedAt"`
}

// GetSyncStatePath returns the path to the global sync state file.
func GetSyncStatePath() (string, error) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, SyncStateFile), nil
}

// GetSyncDaemonPIDPath returns the path to the sync daemon PID file.
func GetSyncDaemonPIDPath() (string, error) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, SyncDaemonPIDFile), nil
}

// GetSyncDaemonLogPath returns the path to the sync daemon log file.
func GetSyncDaemonLogPath() (string, error) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, SyncDaemonLogFile), nil
}

// LoadSyncState reads and parses the global sync state file.
// Returns a default state if the file doesn't exist.
func LoadSyncState() (*SyncState, error) {
	statePath, err := GetSyncStatePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &SyncState{
				Version: SyncStateVersion,
				Label:   "latest",
				Agents:  []AgentDir{},
				Skills:  make(map[string]SyncSkillEntry),
			}, nil
		}
		return nil, err
	}

	var state SyncState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	if state.Skills == nil {
		state.Skills = make(map[string]SyncSkillEntry)
	}
	if state.Agents == nil {
		state.Agents = []AgentDir{}
	}
	if state.Label == "" {
		state.Label = "latest"
	}

	return &state, nil
}

// SaveSyncState writes the sync state to disk.
func SaveSyncState(state *SyncState) error {
	statePath, err := GetSyncStatePath()
	if err != nil {
		return err
	}

	if err := config.EnsureConfigDir(); err != nil {
		return err
	}

	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(statePath, append(data, '\n'), 0644)
}

// AddSkill adds or updates a skill entry in the sync state.
func (s *SyncState) AddSkill(name, label, resolvedVersion, remoteMd5, localHash string) {
	s.Skills[name] = SyncSkillEntry{
		Name:            name,
		Label:           label,
		ResolvedVersion: resolvedVersion,
		RemoteMd5:       remoteMd5,
		LocalHash:       localHash,
		SyncedHash:      localHash,
		Status:          SyncStatusSynced,
		UpdatedAt:       time.Now().UTC().Format(time.RFC3339),
	}
}

// RemoveSkill removes a skill from the sync state.
func (s *SyncState) RemoveSkill(name string) {
	delete(s.Skills, name)
}

// SetLabel updates the global tracking label.
func (s *SyncState) SetLabel(label string) {
	s.Label = label
}

// GetSubscribedSkillNames returns all skill names in the sync state.
func (s *SyncState) GetSubscribedSkillNames() []string {
	names := make([]string, 0, len(s.Skills))
	for name := range s.Skills {
		names = append(names, name)
	}
	return names
}

// DetermineStatus computes the sync status based on local and remote state.
func DetermineStatus(entry SyncSkillEntry, currentLocalHash, currentRemoteMd5 string) SyncStatus {
	localChanged := currentLocalHash != "" && entry.SyncedHash != "" && currentLocalHash != entry.SyncedHash
	remoteChanged := currentRemoteMd5 != "" && entry.RemoteMd5 != "" && currentRemoteMd5 != entry.RemoteMd5

	if localChanged && remoteChanged {
		return SyncStatusConflict
	}
	if entry.Status == SyncStatusUploaded && !remoteChanged {
		return SyncStatusUploaded
	}
	if localChanged {
		return SyncStatusLocalChanges
	}
	if remoteChanged {
		return SyncStatusRemoteChanges
	}
	return SyncStatusSynced
}
