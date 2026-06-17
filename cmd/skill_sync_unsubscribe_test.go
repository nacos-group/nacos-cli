package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nacos-group/nacos-cli/internal/skill"
)

func TestRunSkillSyncRemoveKeepsAgentCopies(t *testing.T) {
	withTempHome(t)

	repoPath, err := skill.EnsureSkillRepo()
	if err != nil {
		t.Fatal(err)
	}
	writeSkillFile(t, repoPath, "demo", "SKILL.md", "repo content")

	agentPath := t.TempDir()
	if err := skill.LinkSkillToAgent(repoPath, "demo", agentPath); err != nil {
		t.Fatal(err)
	}

	hash, err := skill.ComputeDirectoryHash(filepath.Join(repoPath, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	state := &skill.SyncState{
		Version: skill.SyncStateVersion,
		Mode:    skill.SyncModeNacos,
		Repo:    repoPath,
		Label:   "latest",
		Agents: []skill.AgentDir{
			{Name: "codex", Path: agentPath},
		},
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

	var removeErr error
	output := captureStdout(t, func() {
		removeErr = runSkillSyncRemove([]string{"demo"}, removeOptions{})
	})
	if removeErr != nil {
		t.Fatal(removeErr)
	}

	for _, want := range []string{
		"Removing demo from skill-sync...",
		"codex\tdetached (copied)",
		"Removed: demo (agent copies preserved)",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}

	agentSkill := filepath.Join(agentPath, "demo")
	info, err := os.Lstat(agentSkill)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("%s should be a real directory after remove", agentSkill)
	}
	assertFileContent(t, filepath.Join(agentSkill, "SKILL.md"), "repo content")

	state, err = skill.LoadSyncState()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := state.Skills["demo"]; ok {
		t.Fatal("demo should be removed from sync state")
	}
}

func TestRunSkillSyncRemoveAllKeepsAgentCopies(t *testing.T) {
	withTempHome(t)

	repoPath, err := skill.EnsureSkillRepo()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alpha", "beta"} {
		writeSkillFile(t, repoPath, name, "SKILL.md", name+" content")
	}

	agentPath := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		if err := skill.LinkSkillToAgent(repoPath, name, agentPath); err != nil {
			t.Fatal(err)
		}
	}

	state := &skill.SyncState{
		Version: skill.SyncStateVersion,
		Mode:    skill.SyncModeNacos,
		Repo:    repoPath,
		Label:   "latest",
		Agents: []skill.AgentDir{
			{Name: "codex", Path: agentPath},
		},
		Skills: map[string]skill.SyncSkillEntry{},
	}
	for _, name := range []string{"alpha", "beta"} {
		hash, err := skill.ComputeDirectoryHash(filepath.Join(repoPath, name))
		if err != nil {
			t.Fatal(err)
		}
		state.Skills[name] = skill.SyncSkillEntry{
			Name:       name,
			Label:      "latest",
			LocalHash:  hash,
			SyncedHash: hash,
			Status:     skill.SyncStatusSynced,
		}
	}
	if err := skill.SaveSyncState(state); err != nil {
		t.Fatal(err)
	}

	var removeErr error
	output := captureStdout(t, func() {
		removeErr = runSkillSyncRemove(nil, removeOptions{all: true})
	})
	if removeErr != nil {
		t.Fatal(removeErr)
	}

	for _, name := range []string{"alpha", "beta"} {
		for _, want := range []string{
			"Removing " + name + " from skill-sync...",
			"Removed: " + name + " (agent copies preserved)",
		} {
			if !strings.Contains(output, want) {
				t.Fatalf("output missing %q:\n%s", want, output)
			}
		}
		agentSkill := filepath.Join(agentPath, name)
		info, err := os.Lstat(agentSkill)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("%s should be a real directory after remove --all", agentSkill)
		}
		assertFileContent(t, filepath.Join(agentSkill, "SKILL.md"), name+" content")
	}

	state, err = skill.LoadSyncState()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Skills) != 0 {
		t.Fatalf("all skills should be removed from sync state, got %v", state.Skills)
	}
}
