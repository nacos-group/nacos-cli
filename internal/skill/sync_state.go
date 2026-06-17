package skill

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nacos-group/nacos-cli/internal/config"
)

const (
	// SyncStateFile is the legacy global sync state file name.
	SyncStateFile = "skill-sync-state.json"
	// SyncRootDir is the root directory for profile-scoped sync data.
	SyncRootDir = "skill-sync"
	// SyncProfilesDir is the directory containing per-profile sync data.
	SyncProfilesDir = "profiles"
	// SyncProfileStateFile is the per-profile sync state file name.
	SyncProfileStateFile = "state.json"
	// SyncAgentsFile stores globally shared agent directories.
	SyncAgentsFile = "agents.json"
	// SyncActiveProfileFile records the profile currently linked into agents.
	SyncActiveProfileFile = "active-profile"
	// SyncStateVersion is the current schema version.
	SyncStateVersion = 3
	// SyncDaemonPIDFile records the sync daemon process ID.
	SyncDaemonPIDFile = "skill-sync.pid"
	// SyncDaemonLogFile records the sync daemon log output.
	SyncDaemonLogFile = "skill-sync.log"
	// SyncBackupDir is the directory name for conflict backups inside agent dirs.
	SyncBackupDir = ".skill-sync-backup"
)

// SyncMode represents which kind of source the daemon synchronizes from.
type SyncMode string

const (
	// SyncModeUnset indicates the user has not chosen a mode yet.
	SyncModeUnset SyncMode = ""
	// SyncModeLocal pushes content from the local skill repo to agents.
	SyncModeLocal SyncMode = "local"
	// SyncModeNacos pulls content from a Nacos profile to the local repo and agents.
	SyncModeNacos SyncMode = "nacos"
)

// SyncStatus represents the synchronization state of a skill.
type SyncStatus string

const (
	SyncStatusSynced        SyncStatus = "synced"
	SyncStatusLocalChanges  SyncStatus = "local_changes"
	SyncStatusUploaded      SyncStatus = "uploaded"
	SyncStatusRemoteChanges SyncStatus = "remote_changes"
	SyncStatusConflict      SyncStatus = "conflict"
	SyncStatusLinked        SyncStatus = "linked"
	SyncStatusUploadBlocked SyncStatus = "upload_blocked"
	SyncStatusPending       SyncStatus = "pending"
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
	case SyncStatusLinked:
		return "Linked"
	case SyncStatusUploadBlocked:
		return "Upload blocked"
	case SyncStatusPending:
		return "Pending"
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
	// LastUploadedMd5 is the md5 returned by Nacos after our last upload.
	// Used to determine if a remote draft is still ours and safe to overwrite.
	LastUploadedMd5 string `json:"lastUploadedMd5,omitempty"`
	// LastUploadedAt records when this skill was last uploaded by us.
	LastUploadedAt string `json:"lastUploadedAt,omitempty"`
	// UploadedVersion records the draft version produced by the last auto/manual
	// upload. The daemon watches this exact version until it becomes online.
	UploadedVersion string `json:"uploadedVersion,omitempty"`
	// UploadedMd5 records the md5 of UploadedVersion at upload time, so the
	// daemon can verify that the published content is still the uploaded content.
	UploadedMd5 string `json:"uploadedMd5,omitempty"`
	// PendingChangeHash records the local hash that triggered debounce.
	// When two consecutive polls report the same hash, the upload fires.
	PendingChangeHash string `json:"pendingChangeHash,omitempty"`
	// AutoUploadDisabled overrides the global auto-upload setting for this skill.
	AutoUploadDisabled bool `json:"autoUploadDisabled,omitempty"`
	// BlockedDraftVersion records the Nacos editing draft that blocked the last
	// auto-upload attempt.
	BlockedDraftVersion string `json:"blockedDraftVersion,omitempty"`
	// BlockedReviewVersion records the Nacos reviewing version that blocked the
	// last auto-upload attempt.
	BlockedReviewVersion string `json:"blockedReviewVersion,omitempty"`
	// ConflictAgents lists agents whose real directory differs from the central
	// repo. The skill's Status is set to Conflict when this is non-empty.
	ConflictAgents []string `json:"conflictAgents,omitempty"`
	// ExcludedAgents is retained for backward compatibility with older state
	// files; the current CLI no longer writes to it.
	ExcludedAgents []string `json:"excludedAgents,omitempty"`
}

// SyncConfig holds global behavior switches for the sync daemon.
type SyncConfig struct {
	// AutoUpload enables daemon-driven upload when local changes are detected.
	AutoUpload bool `json:"autoUpload"`
}

// SyncState is the top-level per-profile sync state structure.
type SyncState struct {
	Version   int                       `json:"version"`
	Mode      SyncMode                  `json:"mode"`
	Profile   string                    `json:"profile,omitempty"`
	Repo      string                    `json:"repo,omitempty"`
	Label     string                    `json:"label"`
	Config    SyncConfig                `json:"config"`
	Agents    []AgentDir                `json:"agents"`
	Skills    map[string]SyncSkillEntry `json:"skills"`
	UpdatedAt string                    `json:"updatedAt"`
}

type syncAgentsFile struct {
	Version int        `json:"version"`
	Agents  []AgentDir `json:"agents"`
}

var currentSyncProfile string

// SetCurrentSyncProfile sets the profile used by profile-scoped sync paths.
func SetCurrentSyncProfile(profile string) {
	currentSyncProfile = config.NormalizeProfileName(profile)
}

// CurrentSyncProfile returns the active profile name for this process.
func CurrentSyncProfile() string {
	if currentSyncProfile != "" {
		return currentSyncProfile
	}
	profile, err := config.GetCurrentProfile()
	if err != nil || profile == "" {
		return config.DefaultProfile
	}
	return config.NormalizeProfileName(profile)
}

// GetSyncRootPath returns the root directory for skill-sync data.
func GetSyncRootPath() (string, error) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, SyncRootDir), nil
}

// GetSyncProfileDir returns the profile-scoped sync directory.
func GetSyncProfileDir(profile string) (string, error) {
	root, err := GetSyncRootPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, SyncProfilesDir, config.NormalizeProfileName(profile)), nil
}

// GetSyncStatePath returns the path to the current profile sync state file.
func GetSyncStatePath() (string, error) {
	return GetSyncStatePathForProfile(CurrentSyncProfile())
}

// GetSyncStatePathForProfile returns the path to a profile sync state file.
func GetSyncStatePathForProfile(profile string) (string, error) {
	profileDir, err := GetSyncProfileDir(profile)
	if err != nil {
		return "", err
	}
	return filepath.Join(profileDir, SyncProfileStateFile), nil
}

func getLegacySyncStatePath() (string, error) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, SyncStateFile), nil
}

// GetSyncAgentsPath returns the global agent config path.
func GetSyncAgentsPath() (string, error) {
	root, err := GetSyncRootPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, SyncAgentsFile), nil
}

// GetActiveSyncProfilePath returns the active profile marker path.
func GetActiveSyncProfilePath() (string, error) {
	root, err := GetSyncRootPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, SyncActiveProfileFile), nil
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

// LoadActiveSyncProfile reads the profile currently linked into agents.
func LoadActiveSyncProfile() (string, error) {
	path, err := GetActiveSyncProfilePath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// SaveActiveSyncProfile records the profile currently linked into agents.
func SaveActiveSyncProfile(profile string) error {
	path, err := GetActiveSyncProfilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(config.NormalizeProfileName(profile)+"\n"), 0644)
}

// LoadSyncAgents reads globally shared agent directories.
func LoadSyncAgents() ([]AgentDir, error) {
	path, err := GetSyncAgentsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var file syncAgentsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	if file.Agents == nil {
		return nil, nil
	}
	return file.Agents, nil
}

// SaveSyncAgents writes globally shared agent directories.
func SaveSyncAgents(agents []AgentDir) error {
	path, err := GetSyncAgentsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(syncAgentsFile{Version: SyncStateVersion, Agents: agents}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

// LoadSyncState reads and parses the current profile sync state file.
// Returns a default state if the file doesn't exist.
func LoadSyncState() (*SyncState, error) {
	return LoadSyncStateForProfile(CurrentSyncProfile())
}

// LoadSyncStateForProfile reads and parses a profile sync state file.
func LoadSyncStateForProfile(profile string) (*SyncState, error) {
	profile = config.NormalizeProfileName(profile)
	statePath, err := GetSyncStatePathForProfile(profile)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			state, loadErr := loadLegacySyncStateIfProfileMatches(profile)
			if loadErr != nil {
				return nil, loadErr
			}
			if state == nil {
				state = defaultSyncState()
				state.Profile = profile
			}
			if agents, agentsErr := LoadSyncAgents(); agentsErr == nil && len(agents) > 0 {
				state.Agents = agents
			}
			return state, nil
		}
		return nil, err
	}

	state, err := parseSyncState(data)
	if err != nil {
		return nil, err
	}
	state.Profile = profile
	if agents, agentsErr := LoadSyncAgents(); agentsErr == nil && len(agents) > 0 {
		state.Agents = agents
	}
	return state, nil
}

func parseSyncState(data []byte) (*SyncState, error) {
	var state SyncState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	normalizeSyncState(&state, data)
	return &state, nil
}

func normalizeSyncState(state *SyncState, data []byte) {
	if state.Skills == nil {
		state.Skills = make(map[string]SyncSkillEntry)
	}
	if state.Agents == nil {
		state.Agents = []AgentDir{}
	}
	if state.Label == "" {
		state.Label = "latest"
	}
	if !strings.Contains(string(data), "\"autoUpload\"") {
		state.Config.AutoUpload = true
	}
	if state.Version == 0 {
		state.Version = SyncStateVersion
	}
}

func loadLegacySyncStateIfProfileMatches(profile string) (*SyncState, error) {
	legacyPath, err := getLegacySyncStatePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(legacyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	state, err := parseSyncState(data)
	if err != nil {
		return nil, err
	}
	if state.Profile != "" && state.Profile != profile {
		return nil, nil
	}
	state.Profile = profile
	return state, nil
}

// defaultSyncState returns a freshly initialized state with safe defaults.
func defaultSyncState() *SyncState {
	return &SyncState{
		Version: SyncStateVersion,
		Mode:    SyncModeUnset,
		Label:   "latest",
		Config:  SyncConfig{AutoUpload: true},
		Agents:  []AgentDir{},
		Skills:  make(map[string]SyncSkillEntry),
	}
}

// SaveSyncState writes the sync state to disk.
func SaveSyncState(state *SyncState) error {
	profile := state.Profile
	if profile == "" {
		profile = CurrentSyncProfile()
	}
	return SaveSyncStateForProfile(profile, state)
}

// SaveSyncStateForProfile writes a profile sync state to disk.
func SaveSyncStateForProfile(profile string, state *SyncState) error {
	profile = config.NormalizeProfileName(profile)
	statePath, err := GetSyncStatePathForProfile(profile)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(statePath), 0755); err != nil {
		return err
	}

	state.Profile = profile
	state.Version = SyncStateVersion
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	if err := SaveSyncAgents(state.Agents); err != nil {
		return err
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(statePath, append(data, '\n'), 0644)
}

// AddSkill adds or updates a skill entry in the sync state.
func (s *SyncState) AddSkill(name, label, resolvedVersion, remoteMd5, localHash string) {
	now := time.Now().UTC().Format(time.RFC3339)
	status := SyncStatusSynced
	if s.Mode == SyncModeLocal {
		status = SyncStatusLinked
	}
	s.Skills[name] = SyncSkillEntry{
		Name:            name,
		Label:           label,
		ResolvedVersion: resolvedVersion,
		RemoteMd5:       remoteMd5,
		LocalHash:       localHash,
		SyncedHash:      localHash,
		Status:          status,
		UpdatedAt:       now,
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
