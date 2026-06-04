package skill

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	// SkillsLockFile is the name of the lock file that tracks installed/subscribed skills.
	SkillsLockFile = "skills-lock.json"
	// SkillsLockVersion is the current schema version of the lock file.
	SkillsLockVersion = 1
)

// SkillLockEntry represents a single skill entry in the lock file.
type SkillLockEntry struct {
	Name            string `json:"name"`
	Version         string `json:"version,omitempty"`         // pinned version (empty = latest)
	Label           string `json:"label,omitempty"`           // route label (e.g. "latest")
	ResolvedVersion string `json:"resolvedVersion,omitempty"` // actual resolved version from server
	Md5             string `json:"md5"`                       // content fingerprint for conditional download
	Subscribed      bool   `json:"subscribed"`                // whether auto-update is active
	UpdatedAt       string `json:"updatedAt"`                 // ISO8601 timestamp of last update
}

// SkillsLock represents the lock file structure tracking all installed/subscribed skills.
type SkillsLock struct {
	Version int                       `json:"version"`
	Skills  map[string]SkillLockEntry `json:"skills"`
}

// LoadSkillsLock reads and parses the skills-lock.json from the given directory.
// Returns an empty lock structure if the file doesn't exist.
func LoadSkillsLock(dir string) (*SkillsLock, error) {
	lockPath := filepath.Join(dir, SkillsLockFile)

	data, err := os.ReadFile(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &SkillsLock{
				Version: SkillsLockVersion,
				Skills:  make(map[string]SkillLockEntry),
			}, nil
		}
		return nil, err
	}

	var lock SkillsLock
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, err
	}

	if lock.Skills == nil {
		lock.Skills = make(map[string]SkillLockEntry)
	}

	return &lock, nil
}

// SaveSkillsLock writes the lock structure to skills-lock.json in the given directory.
func SaveSkillsLock(dir string, lock *SkillsLock) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	lockPath := filepath.Join(dir, SkillsLockFile)
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(lockPath, append(data, '\n'), 0644)
}

// AddSubscription adds or updates a skill entry in the lock with subscribed=true.
func (l *SkillsLock) AddSubscription(name, version, label, md5, resolvedVersion string) {
	l.Skills[name] = SkillLockEntry{
		Name:            name,
		Version:         version,
		Label:           label,
		ResolvedVersion: resolvedVersion,
		Md5:             md5,
		Subscribed:      true,
		UpdatedAt:       time.Now().UTC().Format(time.RFC3339),
	}
}

// RemoveSubscription sets subscribed=false for the given skill.
func (l *SkillsLock) RemoveSubscription(name string) {
	if entry, ok := l.Skills[name]; ok {
		entry.Subscribed = false
		entry.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		l.Skills[name] = entry
	}
}

// RecordInstall adds or updates a skill entry with subscribed=false (one-shot install).
func (l *SkillsLock) RecordInstall(name, version, label, md5, resolvedVersion string) {
	existing, exists := l.Skills[name]
	subscribed := false
	if exists {
		subscribed = existing.Subscribed
	}

	l.Skills[name] = SkillLockEntry{
		Name:            name,
		Version:         version,
		Label:           label,
		ResolvedVersion: resolvedVersion,
		Md5:             md5,
		Subscribed:      subscribed,
		UpdatedAt:       time.Now().UTC().Format(time.RFC3339),
	}
}

// GetSubscribedSkills returns all entries where subscribed=true.
func (l *SkillsLock) GetSubscribedSkills() []SkillLockEntry {
	var result []SkillLockEntry
	for _, entry := range l.Skills {
		if entry.Subscribed {
			result = append(result, entry)
		}
	}
	return result
}
