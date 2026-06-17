package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
}

func mustWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadSyncState_Empty(t *testing.T) {
	// Point to a temp dir so it won't find a real state file
	origHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Ensure config dir exists
	configDir := filepath.Join(tmpDir, ".nacos-cli")
	mustMkdirAll(t, configDir)

	state, err := LoadSyncState()
	if err != nil {
		t.Fatalf("LoadSyncState() error = %v", err)
	}

	if state.Version != SyncStateVersion {
		t.Errorf("Version = %d, want %d", state.Version, SyncStateVersion)
	}
	if state.Label != "latest" {
		t.Errorf("Label = %q, want %q", state.Label, "latest")
	}
	if len(state.Skills) != 0 {
		t.Errorf("Skills = %v, want empty", state.Skills)
	}
	if len(state.Agents) != 0 {
		t.Errorf("Agents = %v, want empty", state.Agents)
	}
}

func TestSyncState_AddRemoveSkill(t *testing.T) {
	state := &SyncState{
		Version: SyncStateVersion,
		Label:   "latest",
		Skills:  make(map[string]SyncSkillEntry),
	}

	state.AddSkill("pdf", "latest", "v1.0.0", "abc123", "hash456")

	entry, ok := state.Skills["pdf"]
	if !ok {
		t.Fatal("AddSkill did not add entry")
	}
	if entry.Name != "pdf" {
		t.Errorf("Name = %q, want %q", entry.Name, "pdf")
	}
	if entry.Status != SyncStatusSynced {
		t.Errorf("Status = %q, want %q", entry.Status, SyncStatusSynced)
	}
	if entry.RemoteMd5 != "abc123" {
		t.Errorf("RemoteMd5 = %q, want %q", entry.RemoteMd5, "abc123")
	}
	if entry.LocalHash != "hash456" {
		t.Errorf("LocalHash = %q, want %q", entry.LocalHash, "hash456")
	}
	if entry.SyncedHash != "hash456" {
		t.Errorf("SyncedHash = %q, want %q", entry.SyncedHash, "hash456")
	}

	state.RemoveSkill("pdf")
	if _, ok := state.Skills["pdf"]; ok {
		t.Error("RemoveSkill did not remove entry")
	}
}

func TestSyncState_SetLabel(t *testing.T) {
	state := &SyncState{Label: "latest"}
	state.SetLabel("stable")
	if state.Label != "stable" {
		t.Errorf("Label = %q, want %q", state.Label, "stable")
	}
}

func TestDetermineStatus(t *testing.T) {
	tests := []struct {
		name           string
		entry          SyncSkillEntry
		localHash      string
		remoteMd5      string
		expectedStatus SyncStatus
	}{
		{
			name:           "synced - no changes",
			entry:          SyncSkillEntry{SyncedHash: "aaa", RemoteMd5: "bbb", Status: SyncStatusSynced},
			localHash:      "aaa",
			remoteMd5:      "bbb",
			expectedStatus: SyncStatusSynced,
		},
		{
			name:           "local changes",
			entry:          SyncSkillEntry{SyncedHash: "aaa", RemoteMd5: "bbb", Status: SyncStatusSynced},
			localHash:      "ccc",
			remoteMd5:      "bbb",
			expectedStatus: SyncStatusLocalChanges,
		},
		{
			name:           "remote changes",
			entry:          SyncSkillEntry{SyncedHash: "aaa", RemoteMd5: "bbb", Status: SyncStatusSynced},
			localHash:      "aaa",
			remoteMd5:      "ddd",
			expectedStatus: SyncStatusRemoteChanges,
		},
		{
			name:           "conflict - both changed",
			entry:          SyncSkillEntry{SyncedHash: "aaa", RemoteMd5: "bbb", Status: SyncStatusSynced},
			localHash:      "ccc",
			remoteMd5:      "ddd",
			expectedStatus: SyncStatusConflict,
		},
		{
			name:           "uploaded status preserved",
			entry:          SyncSkillEntry{SyncedHash: "aaa", RemoteMd5: "bbb", Status: SyncStatusUploaded},
			localHash:      "aaa",
			remoteMd5:      "bbb",
			expectedStatus: SyncStatusUploaded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetermineStatus(tt.entry, tt.localHash, tt.remoteMd5)
			if got != tt.expectedStatus {
				t.Errorf("DetermineStatus() = %q, want %q", got, tt.expectedStatus)
			}
		})
	}
}

func TestComputeDirectoryHash_Deterministic(t *testing.T) {
	dir := t.TempDir()

	// Create some files
	mustWriteFile(t, filepath.Join(dir, "SKILL.md"), []byte("# Test Skill"))
	mustMkdirAll(t, filepath.Join(dir, "assets"))
	mustWriteFile(t, filepath.Join(dir, "assets", "prompt.txt"), []byte("hello world"))

	hash1, err := ComputeDirectoryHash(dir)
	if err != nil {
		t.Fatalf("ComputeDirectoryHash() error = %v", err)
	}
	if hash1 == "" {
		t.Fatal("ComputeDirectoryHash() returned empty string")
	}

	// Same content should produce same hash
	hash2, err := ComputeDirectoryHash(dir)
	if err != nil {
		t.Fatalf("ComputeDirectoryHash() error = %v", err)
	}
	if hash1 != hash2 {
		t.Errorf("hash not deterministic: %q != %q", hash1, hash2)
	}
}

func TestComputeDirectoryHash_FollowsRootSymlinkDirectory(t *testing.T) {
	tmp := t.TempDir()
	realDir := filepath.Join(tmp, "real-skill")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "SKILL.md"), []byte("# Test Skill"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkDir := filepath.Join(tmp, "linked-skill")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	realHash, err := ComputeDirectoryHash(realDir)
	if err != nil {
		t.Fatalf("ComputeDirectoryHash(real) error = %v", err)
	}
	linkHash, err := ComputeDirectoryHash(linkDir)
	if err != nil {
		t.Fatalf("ComputeDirectoryHash(link) error = %v", err)
	}
	if linkHash != realHash {
		t.Fatalf("hash through symlink = %q, want %q", linkHash, realHash)
	}
}

func TestComputeDirectoryHash_DiffOnChange(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "SKILL.md"), []byte("# Original"))

	hash1, _ := ComputeDirectoryHash(dir)

	// Modify file
	mustWriteFile(t, filepath.Join(dir, "SKILL.md"), []byte("# Modified"))

	hash2, _ := ComputeDirectoryHash(dir)

	if hash1 == hash2 {
		t.Error("hash did not change after file modification")
	}
}

func TestComputeDirectoryHash_IgnoresSkillVersionFrontmatter(t *testing.T) {
	localDir := t.TempDir()
	remoteDir := t.TempDir()

	local := `---
name: demo
description: test skill
---
# Demo
`
	remote := `---
version: 0.0.1
name: demo
description: test skill
---
# Demo
`

	if err := os.WriteFile(filepath.Join(localDir, "SKILL.md"), []byte(local), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(remoteDir, "SKILL.md"), []byte(remote), 0644); err != nil {
		t.Fatal(err)
	}

	localHash, err := ComputeDirectoryHash(localDir)
	if err != nil {
		t.Fatal(err)
	}
	remoteHash, err := ComputeDirectoryHash(remoteDir)
	if err != nil {
		t.Fatal(err)
	}

	if localHash != remoteHash {
		t.Fatalf("hash differs for version-only frontmatter change: %s != %s", localHash, remoteHash)
	}
}

func TestComputeDirectoryHash_VersionOutsideFrontmatterStillCounts(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()

	a := `---
name: demo
---
version: local
`
	b := `---
name: demo
---
version: remote
`

	if err := os.WriteFile(filepath.Join(dirA, "SKILL.md"), []byte(a), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirB, "SKILL.md"), []byte(b), 0644); err != nil {
		t.Fatal(err)
	}

	hashA, err := ComputeDirectoryHash(dirA)
	if err != nil {
		t.Fatal(err)
	}
	hashB, err := ComputeDirectoryHash(dirB)
	if err != nil {
		t.Fatal(err)
	}

	if hashA == hashB {
		t.Fatal("hash should differ when version text changes outside frontmatter")
	}
}

func TestComputeDirectoryHash_ExcludesGit(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "SKILL.md"), []byte("# Test"))

	hash1, _ := ComputeDirectoryHash(dir)

	// Add .git directory (should be excluded)
	mustMkdirAll(t, filepath.Join(dir, ".git"))
	mustWriteFile(t, filepath.Join(dir, ".git", "config"), []byte("git stuff"))

	hash2, _ := ComputeDirectoryHash(dir)

	if hash1 != hash2 {
		t.Error("hash changed when .git was added (should be excluded)")
	}
}

func TestSyncState_SaveLoadRoundTrip(t *testing.T) {
	origHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	configDir := filepath.Join(tmpDir, ".nacos-cli")
	mustMkdirAll(t, configDir)

	state := &SyncState{
		Version: SyncStateVersion,
		Label:   "stable",
		Agents: []AgentDir{
			{Name: "codex", Path: "/home/user/.codex/skills", AutoFound: true},
		},
		Skills: map[string]SyncSkillEntry{
			"pdf": {
				Name:            "pdf",
				Label:           "stable",
				ResolvedVersion: "v1.0.0",
				RemoteMd5:       "abc123",
				LocalHash:       "def456",
				SyncedHash:      "def456",
				Status:          SyncStatusSynced,
				UpdatedAt:       "2026-01-01T00:00:00Z",
			},
		},
	}

	if err := SaveSyncState(state); err != nil {
		t.Fatalf("SaveSyncState() error = %v", err)
	}

	loaded, err := LoadSyncState()
	if err != nil {
		t.Fatalf("LoadSyncState() error = %v", err)
	}

	if loaded.Label != "stable" {
		t.Errorf("Label = %q, want %q", loaded.Label, "stable")
	}
	if len(loaded.Agents) != 1 {
		t.Fatalf("Agents len = %d, want 1", len(loaded.Agents))
	}
	if loaded.Agents[0].Name != "codex" {
		t.Errorf("Agent name = %q, want %q", loaded.Agents[0].Name, "codex")
	}

	entry, ok := loaded.Skills["pdf"]
	if !ok {
		t.Fatal("Skill 'pdf' not found after load")
	}
	if entry.Status != SyncStatusSynced {
		t.Errorf("Status = %q, want %q", entry.Status, SyncStatusSynced)
	}
	if entry.RemoteMd5 != "abc123" {
		t.Errorf("RemoteMd5 = %q, want %q", entry.RemoteMd5, "abc123")
	}
}
