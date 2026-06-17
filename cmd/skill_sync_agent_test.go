package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nacos-group/nacos-cli/internal/skill"
)

func TestRunSkillSyncAgentAddLinksExistingSkills(t *testing.T) {
	withTempHome(t)

	repoPath, err := skill.EnsureSkillRepo()
	if err != nil {
		t.Fatal(err)
	}
	writeSkillFile(t, repoPath, "demo", "SKILL.md", "shared")

	hash, err := skill.ComputeDirectoryHash(filepath.Join(repoPath, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	state := &skill.SyncState{
		Version: skill.SyncStateVersion,
		Mode:    skill.SyncModeNacos,
		Repo:    repoPath,
		Label:   "latest",
		Skills: map[string]skill.SyncSkillEntry{
			"demo": {
				Name:       "demo",
				Label:      "latest",
				LocalHash:  hash,
				SyncedHash: hash,
				Status:     skill.SyncStatusSynced,
			},
		},
	}
	if err := skill.SaveSyncState(state); err != nil {
		t.Fatal(err)
	}

	agentPath := t.TempDir()
	var addErr error
	output := captureStdout(t, func() {
		addErr = runSkillSyncAgentAdd("agents", agentPath)
	})
	if addErr != nil {
		t.Fatal(addErr)
	}

	assertSkillSymlink(t, agentPath, "demo")
	assertFileContent(t, filepath.Join(agentPath, "demo", "SKILL.md"), "shared")
	for _, want := range []string{
		"Syncing 1 existing skill(s) to agents...",
		"agents\tlinked (new)",
		"Added agent: agents",
		"Synced 1 existing skill(s) to agents.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}

	state, err = skill.LoadSyncState()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Agents) != 1 || state.Agents[0].Name != "agents" {
		t.Fatalf("agents = %+v, want one agents entry", state.Agents)
	}
	if got := state.Skills["demo"].Status; got != skill.SyncStatusSynced {
		t.Fatalf("status = %s, want synced", got)
	}
}

func TestRunSkillSyncAgentAddRecordsSkillConflict(t *testing.T) {
	withTempHome(t)

	repoPath, err := skill.EnsureSkillRepo()
	if err != nil {
		t.Fatal(err)
	}
	writeSkillFile(t, repoPath, "demo", "SKILL.md", "shared")

	hash, err := skill.ComputeDirectoryHash(filepath.Join(repoPath, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	state := &skill.SyncState{
		Version: skill.SyncStateVersion,
		Mode:    skill.SyncModeNacos,
		Repo:    repoPath,
		Label:   "latest",
		Skills: map[string]skill.SyncSkillEntry{
			"demo": {
				Name:       "demo",
				Label:      "latest",
				LocalHash:  hash,
				SyncedHash: hash,
				Status:     skill.SyncStatusSynced,
			},
		},
	}
	if err := skill.SaveSyncState(state); err != nil {
		t.Fatal(err)
	}

	agentPath := t.TempDir()
	writeSkillFile(t, agentPath, "demo", "SKILL.md", "local")

	var addErr error
	output := captureStdout(t, func() {
		addErr = runSkillSyncAgentAdd("agents", agentPath)
	})
	if addErr != nil {
		t.Fatal(addErr)
	}

	info, err := os.Lstat(filepath.Join(agentPath, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("conflicting agent skill should not be overwritten with symlink")
	}
	assertFileContent(t, filepath.Join(agentPath, "demo", "SKILL.md"), "local")
	for _, want := range []string{
		"agents\tskipped (kept local)",
		"Conflicts: [demo]",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}

	state, err = skill.LoadSyncState()
	if err != nil {
		t.Fatal(err)
	}
	entry := state.Skills["demo"]
	if entry.Status != skill.SyncStatusConflict {
		t.Fatalf("status = %s, want conflict", entry.Status)
	}
	if got := strings.Join(entry.ConflictAgents, ","); got != "agents" {
		t.Fatalf("conflict agents = %q, want agents", got)
	}
}
